# Parity with the AIgentFlow reference validator and the JavaScript port

This document defines how this Go implementation stays in step with two things:

1. the **AIgentFlow Go engine's static checks** — the ultimate reference, closed-source; and
2. **[`aigentflow-flow-validator-js`](https://github.com/itsatony/aigentflow-flow-validator-js)** —
   the MIT-licensed JavaScript port this library was ported *from*, and whose `PARITY.md` maps every
   rule back to the reference.

**Tracks AIgentFlow flow schema: `v2.738.0`** (`specVersion` in
[`spec/aigentflow-spec.json`](./spec/aigentflow-spec.json)).

> **v0.3.0 (2026-09-25) — parity refresh, v2.485.0 → v2.738.0.** The spec and all 48 conformance
> fixtures are byte-identical to the JS port's `main` (package 0.12.0), and every rule that port
> added in that window is ported here:
>
> - **Refused keys (`unknown_yaml_key`, error):** flow-root `budget:` and `max_retries:`, a
>   `max_retries:` on a step or loop sub-step (v2.721.0), and `campaign.budget_max_per_child`
>   (v2.728.0). On key presence, so `budget: 0` and `budget: null` are refused too.
> - **Executor URL shape (error):** the reference's one URL regex, vendored in the spec. Templated
>   URLs are skipped. `openai:///gpt-4`, `ai://openai`, `http://api.example.com/v1` and
>   `ai://openai/gpt-4.1` are now refused.
> - **Expression-function catalog (errors):** `package:` is refused, a `function:` outside the fixed
>   20-entry catalog is refused, and a template that calls an `fn_` name must name a catalog entry
>   the flow declares (v2.642.0).
> - **Loop body (v2.648.0, v2.672.0):** sub-step `next:` targets (`loop_substep_next_target_not_found`,
>   `_sentinel`, `_parallel`, errors); `loop_sub_step_id_reserved` (warning); templates and
>   processing operations inside a loop body are checked, always as warnings.
> - **Processing-operation shape (warnings, v2.647.0):** `unknown_processing_operation` and
>   `unknown_processing_config_key`, with the loop-only `loop.set` / `loop.break` partition.
> - **Warnings:** `unreachable_error_goto` (v2.651.0), `loop_substep_error_goto_ignored` (v2.652.0),
>   `response_expectation_unread` (DC-FORGE-145), `step_max_duration_ignored` (DC-FORGE-147).
> - **Orchestrator / campaign (errors):** `orchestrator_human_question_timeout_invalid` (v2.695.0;
>   unparseable or non-positive), `campaign.max_credits_per_child` must decode as an integer and be
>   `>= 0` (v2.728.0).
> - **Registries:** seven orchestrator tools (`aif_memory_recall`, `aif_memory_reflect`, five
>   `aif_e2b_*`), the registered executor-scheme set (43, `web` added, nine unregistered legacy
>   names removed), and template functions `mod`, `atoi`, `int`, `addf`, `subf`, `mulf`, `divf`,
>   `float64` plus the 20 catalog names. The template list was checked against the reference's
>   registry: identical.
> - **Behaviour changes:** a condition's yield `goto: orchestrator` now counts as the owner-mode
>   yield edge (v0.2.0 still read `goto_step` there), and an unknown template function is now a
>   **warning** by default instead of silent (error under `StrictRegistries`, as divergence #4
>   in the JS port always said).
>
> **Measured, not assumed.** A throwaway program (outside both repositories, with its own `go.mod`
> and a local `replace`) ran every YAML file under the reference's `example_flows/` (216 files)
> through the reference's save door — `NewStrictFlowParser().ParseFromYAMLBytes` then
> `ValidateFlowWithDetails` — and through this library:
>
> | 216 bundled flows | reference | v0.2.0 | v0.3.0 |
> | --- | --- | --- | --- |
> | valid, default options | 206 | 210 | 210 |
> | valid, `StrictRegistries` | 206 | 197 | **206** |
> | same verdict, default | — | 212 | 212 |
> | same verdict, `StrictRegistries` | — | 207 | **216** |
> | `unreachable_step` / `potential_infinite_loop` | 25 / 1 | 25 / 1 | 25 / 1 (same `(file, field)` pairs) |
>
> The four default-mode differences are the same four files both times: the reference refuses a
> template calling an unknown function (`{{ query.x }}`), and this library warns by default
> (divergence #7). With `StrictRegistries` every verdict matches. On the conformance fixtures
> (normalised for divergences #8 and #9) the verdict matches on 48 of 48, and on every fixture the
> reference parses the error and warning `(code, field)` sets are identical, apart from the one
> intended `unknown_executor_scheme` warning.

> **v0.4.0 (2026-09-25) — the JS port caught up, and one false positive here.** The five
> reference save-door refusals this library carried first (below) are ported in the JS port's
> **0.13.0** ([PR #18](https://github.com/itsatony/aigentflow-flow-validator-js/pull/18)), with ten
> new conformance fixtures, copied here byte-for-byte. The spec is unchanged (`v2.738.0`). Until
> that PR merges, the parity harness must run against its branch
> (`parity/go-only-save-door-rules`), because `main` there does not yet carry the fixtures.
>
> | Code | Reference rule |
> | --- | --- |
> | `reserved_step_id_orchestrator` | a top-level step may not be called `orchestrator` (v2.484.0) |
> | `tool_discovery_invalid` | `tool_discovery` on the flow, the orchestrator or a step `query` must be `eager`, `lazy` or `off` (empty and templated values are skipped) |
> | `mock_delay_invalid` | a `mock_scenarios` step `delay` must be a Go duration (`100` is refused, `100ms` is not; see divergence #15) |
> | `output_param_empty` | an `output:` entry may not be the empty string (a YAML null entry saves) |
> | `campaign_no_child_flows`, `campaign_child_flow_no_id` | `campaign.child_flows` must be non-empty and each entry needs `flow_id` or `flow_name` |
>
> **Fixed here: null `child_flows` entries.** The reference decodes `child_flows` into a typed
> slice, and yaml.v3 drops null list entries while doing so. This library decodes into untyped
> values, which keep the nil, and v0.3.0 judged a null entry as a non-mapping. Measured on the
> reference's strict save parser (`NewStrictFlowParser().ParseFromYAMLBytes`, then
> `ValidateFlowWithDetails`):
>
> | `campaign.child_flows` | reference | v0.3.0 | v0.4.0 |
> | --- | --- | --- | --- |
> | `[{flow_name: c}, ~]` (either order) | saves, 1 entry | `invalid_type` | valid |
> | `[~]`, `[~, ~]`, a lone bare `-` | `campaign_no_child_flows` | `invalid_type` | `campaign_no_child_flows` |
> | `[{flow_name: c}, foo]` | refused (cannot decode `foo`) | `invalid_type` | `invalid_type` |
>
> A finding's index is the entry's position in the YAML list, as in the JS port. The reference's
> own message counts only the non-null entries; messages are not part of the contract.
>
> **`max_credits_per_child: 1.5` — both ports now agree.** The reference decodes it into an `int64`
> field, which truncates, and saves the flow. This library has always accepted it; the JS port
> refused any non-integer as `invalid_type` and stops doing so in 0.13.0. The new shared fixture
> `valid-campaign-decoded-shapes.yaml` pins it on both sides.

## The comparison contract

**Error `code` + the `valid` verdict.** Never message wording. `Message`, `Context`, and
`Suggestion` are for humans and may differ freely between implementations; `Code` is what a consumer
branches on, so it is the thing held stable.

`Valid` is false ⇔ `Errors` is non-empty. Warnings never affect it, in either implementation.

## How the two are kept in step

Three mechanisms, in increasing strength. All three live in `parity_test.go` and skip cleanly when
the JS checkout is absent, so CI on a bare clone passes — but they fail loudly when it is present and
the two disagree, which is the only way a one-sided change gets caught.

1. **`TestParitySpecIsByteIdenticalUpstream`** — `spec/aigentflow-spec.json` must be byte-identical
   to the JS repo's `src/spec/aigentflow-spec.json`. This is why the enum surface is **embedded via
   `go:embed` and never transcribed into Go constants**: an upstream enum bump stays a one-file copy
   that the conformance suite immediately re-verifies, instead of 51 hand-edited string literals
   nobody re-reads.
2. **`TestParityFixtureSetIsIdentical`** — the conformance corpus (`testdata/conformance/`) must
   match the JS repo's `test/conformance/fixtures/` by name *and* by bytes. A fixture edited on one
   side only would let the two suites be self-consistently green about different documents.
3. **`TestParityVerdictsMatchTheJSImplementation`** — runs the JS CLI over every shared fixture and
   diffs the code sets. Error codes must match **exactly**; JS warnings must all be present here
   (not vice versa — see the intended divergences below).

Mechanism 3 depends on a **fresh** JS build. The harness reads the built CLI's own reported schema
version and skips with an explanatory message when it trails this library's, because a stale
`dist/` would otherwise surface as parity drift. `make parity-check` rebuilds first for that reason.

## Intended divergences

These are deliberate and are *not* parity failures. Each states its direction, because the
conformance harness's subset assertions only tolerate one direction.

### 1. Template syntax: this implementation is exact where JS approximates

The JS port hand-rolls a lexer (`src/template/gotmpl-syntax.ts`) and says so plainly: "NOT a renderer
and NOT a full parser… deliberate bias against false positives". JavaScript has no Go template
parser, so it cannot do better.

Go does: this library calls `text/template.Parse` — the *same parser the AIgentFlow engine uses* —
with every one of the allow-listed template functions registered as a no-op stub. Delimiters,
quoting, comments, `if`/`range`/`with`/`block`/`define` nesting, `else` placement, and empty actions
are therefore judged exactly rather than approximately.

**Direction: Go ⊇ JS on `template_syntax_error`.** Go may reject a pipeline the JS lexer waves
through; it will not accept one JS rejects. The verdict harness asserts error-code *equality* on the
shared fixtures (which both implementations agree on today) and warning *containment*; a future
fixture where Go is legitimately stricter must be added to both repos, with the JS side's expectation
recorded as a known gap in *its* `PARITY.md`.

Unknown function names are discovered by registering a stub and re-parsing in a bounded loop, so an
unknown function never masks the rest of a document's syntax errors — a single-pass parse would stop
at the first unrecognised name.

### 2. `pattern` compilation: RE2, not the JS regex engine

`input_schema[].pattern` is compiled with Go's `regexp` (RE2), which is what the engine uses. The JS
port compiles with the JS regex engine. A pattern valid in one and not the other diverges; **this
side is authoritative**, since it matches the runtime.

### 3. Durations: `time.ParseDuration`, not a hand-rolled scanner

The JS port reimplements Go's duration grammar (including the `µs`/`μs` variants). This library calls
`time.ParseDuration` directly, so the check is exact. No known behavioural difference; recorded
because it is a different mechanism reaching the same rule.

### 4. Finding ORDER differs

Go map iteration is randomised, so every validator here walks steps and mappings in **sorted key
order**; the JS port walks YAML insertion order. Order is not part of the comparison contract (code
*sets* are), and determinism is worth more than insertion order to a consumer that diffs or caches
verdicts — `TestDeterministicFindingOrder` locks it.

### 5. Source positions are an addition

The JS port positions parse errors only. This library builds a path→position index from the
`yaml.Node` tree and resolves **every** finding to a line and column, falling back to the nearest
existing ancestor for a path that by definition does not exist (a `missing_required_field` names a
key that is absent — landing an author on the enclosing step is right, landing them on line 1 is
useless). Purely additive: no verdict depends on it.

### 6. Not ported: the JS CLI

`src/cli.ts` has no counterpart. The Go consumer is a library.

### 7. Unknown template functions: a warning by default (shared with JS, divergence #4 there)

The reference refuses a template that calls a function its registry does not have, and reports it
as `template_syntax_error`. This library reports it as `template_function_unknown`: a **warning** by
default and an **error** under `StrictRegistries`. The vendored list can lag the live registry (a
real new function must not block a publish), which is the same reason unknown orchestrator tools and
executor schemes warn. At v0.3.0 the vendored list equals the reference's registry exactly.

**Direction: looser than the reference by default.** Measured: 4 of the reference's 216 bundled
flows are refused by the reference for this alone and are valid here without `StrictRegistries`.
v0.2.0 said nothing at all in that case; v0.3.0 at least warns.

### 8. `end` as a next target (shared with JS, divergence #2 there)

Both ports treat `null`, `end` and `orchestrator` as terminal markers. The reference's
connectivity check exempts `end`, but its save-door parser (`validateNextLogic`) does **not**: it
refuses `next: { default: end }` because no step is called `end`. Measured at v2.738.0: 23 of the
48 shared fixtures route to `end`, and the reference's save door refuses 16 of them with "step_id
(end) … not found" (14 are fixtures both ports call valid; the rest fail earlier on another rule).

**Direction: looser than the reference.** Kept because the shared fixtures and the JS port depend
on it, and changing it is a cross-repository decision. **Owed:** decide it in both ports together.

### 9. Unknown keys are not rejected in general (shared with JS)

The reference's save door parses with `KnownFields(true)`, so any key its types do not declare is
refused. This library reports unknown keys only where the reference names them: the retired keys
above and a condition's `goto_step`. For example the shared fixture `valid-branching.yaml` carries a
flow-root `constraints:` block, which both ports accept and the reference refuses. Porting the rule
means a full field inventory of every flow, step, loop and orchestrator type. **Owed, as in JS.**

### 10. The loop body is walked for every step (shared with JS)

The reference walks the loop body only for steps reachable from `start`. This library walks every
step. All loop-body findings are warnings, so no verdict changes.

### 11. The `fn_` usage scan walks the document (shared with JS, divergence #10 there)

The reference re-serialises its typed `Flow` and scans that, so it sees only declared fields. This
library walks every string leaf of the parsed document. The two agree on every flow the reference
would accept, because anything else is refused by divergence #9's rule.

### 12. A malformed processing-operation entry gets no shape verdict (shared with JS, #11 there)

The reference's unmarshaller refuses an entry that is not a one-key map. This library has no typed
unmarshal, so such an entry produces neither `unknown_processing_operation` nor
`unknown_processing_config_key`.

### 13. `human_question_timeout`: the refusal is ported, the log line is not (shared with JS, #12 there)

The reference also logs (not a validation warning) when the timeout exceeds its orchestrator
mission clock. That threshold is a deployment constant this package cannot observe.

### 14. Warnings the reference does not emit

- `unknown_executor_scheme`: the reference never warns on a scheme; it fails at dispatch. The
  bundled `voiceagent://` flow warns here for that reason.
- `unknown_data_type` on a top-level `query` type such as `bool` (shared with JS). Nine bundled
  flows carry it.
- `step_max_duration_ignored` on a boolean `max_duration` (`true`): the reference warns too (it
  decodes the boolean as the text "true"); the JS port ignores booleans.
- The reference's runtime template field-resolution warnings (`template_missing_field`,
  `condition_not_boolean`) and `compliance_catalog_missing` are not produced here (scope).

### 15. A mock `delay` written as a number is judged by its parsed value (shared with JS, #13 there)

yaml.v3 fills the reference's `string` field with the scalar's **source text**; this library decodes
into untyped values and only has the parsed number. They agree on every number except those that
parse to zero without being written `0`, `+0` or `-0`: `delay: 0.0`, `00` and `0x0` are refused by
the reference (no unit) and accepted here. Measured on the reference's strict save parser: `0`,
`+0`, `-0` save; `0.0`, `00`, `0x0`, `100` are refused. Every other number is refused on both sides,
because no number's text carries a unit.

**Direction: looser than the reference.** The fix an author needs is the one the refusal already
gives: write a unit. The same limit applies to `tool_discovery` and to a campaign `flow_id` /
`flow_name`, where it cannot change a verdict (no number or boolean spelling is a valid mode, an
empty value, or a template). Recovering the spelling would need the `yaml.Node` tree this library
already builds for positions; **owed**, and best decided in both ports together.

## Not ported (owed)

- `ValidateExecutorConfigEnvScopes` (reference v2.597.0): `executor_config` may expand only the
  environment variables of the provider it is written under. Needs four vendored scope tables. Owed
  in both ports.
- General unknown-key rejection (divergence #9).
- The `end` decision (divergence #8).
- The numeric-spelling gap for mock `delay` (divergence #15).

## Enum surfaces carried but not yet consumed

The vendored spec includes values no static rule reads yet: `parallelResolutions`,
`inputSchema.datePattern`, `inputSchema.maxInputKeyCount`, `inputSchema.maxStringInputLength`, and
`evalJudgeUrl`. (`templateActionOpen` is mirrored by the `templateOpenDelim` constant.) They are unused in the JS implementation too — the reference validates them at
runtime, not statically. They stay in the embedded document so it remains byte-identical upstream;
**do not prune them**, that would break mechanism 1.

## Discipline for a schema bump

When AIgentFlow's flow schema changes:

1. Land it upstream in the JS repo first (its `PARITY.md` §"Discipline" governs that side), including
   the new enum values and any new conformance fixture.
2. `cp <js-repo>/src/spec/aigentflow-spec.json spec/aigentflow-spec.json` — never hand-edit either copy.
3. `cp <js-repo>/test/conformance/fixtures/*.yaml testdata/conformance/` and add the matching case to
   `conformanceCases`. `TestConformanceFixturesAreAllCovered` fails if you forget the case, so a copied
   fixture cannot sit unasserted.
4. Port the new rule, with its `code` constant added to `keys.go`.
5. `make ci && make parity-check`.
6. Note the version and the ported rule at the top of this file's tracked-version line.

## Consumers that BLOCK on a verdict

If you are using this as an admission gate (aigentverse does), two obligations follow from the
pinned-schema model:

- **Surface `Result.SpecVersion`** on every rejection. See README.
- **Keep `StrictRegistries` off.** The registry-lag warnings exist precisely so a real-but-newer
  executor scheme, template function, or orchestrator tool cannot block a publish.
