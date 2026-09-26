# go-aigentflow-validator

Static, offline validator for [AIgentFlow](https://aigentflow.dev.ai.vaud.one) workflow YAML — in **Go**.

It catches the structural and semantic problems in a flow definition *before* it reaches a server:
missing required fields, dangling step references, malformed executor URIs, broken Go-template
syntax, invalid `input_schema` fields, unbalanced query/response schemas, and more. It reproduces
the **static** checks of the AIgentFlow Go engine, so a flow that passes here passes the server's
structural validation.

This is the Go counterpart of
[`aigentflow-flow-validator-js`](https://github.com/itsatony/aigentflow-flow-validator-js). The two
share one vendored enum surface and one conformance corpus, and agree on error `code` — see
[PARITY.md](./PARITY.md).

- **Zero-config, offline, deterministic.** No network, no filesystem, no credentials, no server.
- **One dependency.** `gopkg.in/yaml.v3` and nothing else.
- **WASM-safe.** Builds for `GOOS=js GOARCH=wasm`, so the same rules run in a server, a CLI, and a browser.
- **Positioned findings.** Every finding resolves to a line and column in the source.
- **Parity-tracked.** Mirrors a pinned AIgentFlow flow-schema version, with a test that diffs against the JS implementation.

> **Scope:** static checks only. It does **not** check credentials, the model-compliance catalogue,
> or runtime template field-resolution — those need a live server. See
> [What it does not check](#what-it-does-not-check).

---

## Install

```bash
go get github.com/itsatony/go-aigentflow-validator
```

Requires Go 1.25+.

## Usage

```go
import aifvalidate "github.com/itsatony/go-aigentflow-validator"

result := aifvalidate.ValidateFlow(yamlSource, aifvalidate.Options{})
if !result.Valid {
    for _, e := range result.Errors {
        fmt.Printf("%s [%s] line %d: %s\n", e.Code, e.Field, e.Line, e.Message)
    }
}
for _, w := range result.Warnings {
    fmt.Printf("warning: %s [%s]: %s\n", w.Code, w.Field, w.Message)
}
```

Already have a decoded document? Use `ValidateFlowObject(map[string]any, Options)`. It gives the
same verdict, but cannot set `Line`/`Column` — there is no source to locate findings in.

### Errors vs warnings

`Valid` is `false` if and only if `Errors` is non-empty. **Warnings never lower the verdict**, and a
consumer that treats them as blocking will reject legitimate flows: the vendored allow-lists
(executor schemes, template functions, orchestrator tools) can lag the live AIgentFlow registries,
so an unrecognised-but-real name is reported as a warning on purpose.

Set `Options{StrictRegistries: true}` to promote unrecognised-name findings to errors. That is
right for an authoring-time lint and wrong for admission control.

Know the cost of leaving it off: AIgentFlow itself refuses a template that calls a function it does
not have. Without `StrictRegistries` this library only warns (`template_function_unknown`), so such
a flow passes here and is refused at AIgentFlow's save door. At spec v2.753.0 the vendored function
list equals AIgentFlow's registry, so the warning is a real problem unless your AIgentFlow is newer.

`Errors` and `Warnings` are always non-nil, so they encode as `[]` and never `null`.

### Reporting which schema version judged a flow

`Result.SpecVersion` (and `SpecVersion()`) names the AIgentFlow flow-schema version whose rules
produced the verdict. **Surface it wherever you block on an error.** A pinned validator can
legitimately reject a flow written against a newer schema, and an author who cannot see which
version rejected them has no way to understand it.

## What it checks

| Area | Examples |
| --- | --- |
| Basic structure | required top-level fields, step-map shape, per-step executor, reserved `.` in step IDs, reserved step ID `orchestrator` |
| Unknown keys and value kinds | any key a flow type does not declare, at every level (`unknown_yaml_key`); a value of the wrong kind, e.g. `next: end` (`invalid_type`) |
| Retired keys | flow-root `budget:` and `max_retries:`, step and loop sub-step `max_retries:`, `campaign.budget_max_per_child` (all refused as `unknown_yaml_key`) |
| Executors | the reference's executor-URL shape, `scheme://authority/path` (error; templated URLs skipped), unknown scheme (warning) |
| Connectivity | `next.default` / `next.conditions[].goto` existence (error; only `null` and `orchestrator` need no step, so `end` must name one), unreachable steps + cycles (warning) |
| `next.parallel` | rendezvous + member existence, non-empty fan-out, `next: orchestrator` needs an orchestrator |
| `error_strategy` | action enum, `goto_step` existence, Go durations, `backoff_multiplier > 0`, `retry_on` categories, a `goto_step` no action can take (warning) |
| `query` schema | param types, nested `properties`, array `items` types, `min_items`/`max_items` |
| `response_expectation` | field data types, array `items`, `required` as bool-or-template, an expectation nothing reads (warning) |
| `for_each` / `loop` / `throttle` | mutual exclusions, `max_iterations` bounds, sub-step ids, loop sub-step `next:` targets, throttle ceilings |
| Loop body | templates and processing operations inside `loop.steps` (warnings), `goto_step` in a sub-step (warning), reserved sub-step ids (warning) |
| Processing operations | undispatchable operation type, config keys the handler never reads (warnings) |
| Step `max_duration` | unparseable, or on a loop step (warning) |
| Credential bindings | `credential` vs `credentials` exclusivity, `stored/{provider}/{name}` form, `inject_as` |
| `input_schema` / `output_schema` | version, field names, types, constraint/type compatibility, `visible_when`, RE2 patterns |
| `quality_gate` | rubric, threshold range, `on_fail` enum, self-goto, composite/parallel-member scope |
| Orchestrator / campaign | `exons` presence, `mode` enum + owner-needs-yield, triggers, tools, `human_question_timeout`, `child_flows`, `max_credits_per_child`, campaign handoff |
| Templates | Go `text/template` syntax across `query`, `pre_processing`, `post_processing`, `conditions[].if` |
| `expression_functions` | exactly one of `package` / `function`; `package:` refused; `function:` must be in the fixed catalog; a template calling an `fn_` name must declare it |
| Other save-door rules | `tool_discovery` vocabulary, mock-scenario `delay` durations, empty `output:` entries |

## What it does not check

- **Credentials.** Only the reference *form* is validated. Nothing is read, resolved, or transported.
- **The model-compliance catalogue.** Whether a named model is permitted is a server question.
- **Runtime template field resolution.** Whether `.data.fetch.title` will exist at run time needs the
  live state graph. The companion `unresolvable_data_path` rule is therefore out of scope.
- **The `orchestrator.exons` body.** Its presence is required; its contents belong to the go-exons engine.

## Development

```bash
make ci             # fmt + vet + lint + test + wasm-check
make parity-check   # diff verdicts against the sibling JS implementation
```

`make parity-check` needs the JS checkout (`JS_VALIDATOR_REPO`, default
`~/code/aigentflow-flow-validator-js`) and rebuilds it first — the harness compares against the
*built* CLI and skips loudly if that build trails this library's pinned schema version, so a stale
build can never masquerade as parity drift.

## Licence

MIT — see [LICENSE](./LICENSE).
