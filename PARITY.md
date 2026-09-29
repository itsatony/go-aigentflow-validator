# Parity with the AIgentFlow reference validator and the JavaScript port

This document defines how this Go implementation stays in step with two things:

1. the **AIgentFlow Go engine's static checks** — the ultimate reference, closed-source; and
2. **[`aigentflow-flow-validator-js`](https://github.com/itsatony/aigentflow-flow-validator-js)** —
   the MIT-licensed JavaScript port this library was ported *from*, and whose `PARITY.md` maps every
   rule back to the reference.

**Tracks AIgentFlow flow schema: `v2.788.0`** (`specVersion` in
[`spec/aigentflow-spec.json`](./spec/aigentflow-spec.json)), plus `server_owned_query_key`
(v0.6.1, below) from the AIgentFlow release after v2.790.0, and `credential_endpoint_unpaired`
(v0.6.2, below) from AIgentFlow v2.793.0 and the release after it.

> **v0.6.2 (2026-09-29) — an endpoint a server-supplied credential will never be sent to.** One
> reference WARNING, ported to both ports together (JS port 0.15.2), with the same spec and
> fixtures.
>
> | Rule (reference) | Code | Severity | File |
> | --- | --- | --- | --- |
> | `validateCredentialEndpointPairing` (AIgentFlow DC-FORGE-231, v2.793.0) and `validateFamilyCredentialEndpointPairing` (DC-FORGE-233) | `credential_endpoint_unpaired` | warning | `validate.credentialendpoint.go`, spec `credentialEndpointPairing` |
>
> - **Why.** AIgentFlow sends a credential the SERVER supplied (an org's or the platform's stored
>   key, or a value of one of the deployment's environment variables) only to the endpoint that
>   came with it: the default, the stored credential's own base URL, or a deployment-configured
>   one. An endpoint the flow names is honoured only with a credential the flow supplies. That
>   run-time refusal is the control; this warning names, at save, the shapes that are statically
>   certain to meet it. A warning, never an error: a run may still bring its own key, and a stored
>   flow must stay saveable.
> - **ai:// (DC-FORGE-231).** A step's (or loop sub-step's) `<provider>_base_url`, a non-empty
>   YAML string, with no key of the flow's own: neither the step query's `api_key` /
>   `<provider>_api_key` (non-empty strings, literal or templated) nor a literal
>   `executor_config.<provider>.api_key` (an `${ENV}` reference is the server's key). And a literal
>   `executor_config.<provider>.base_url` for an AI provider with no literal key beside it, unless
>   at least one step uses the provider and every such step brings its own query key. `ollama` and
>   `vllm` are exempt. Fields: `steps.<step>.query.<key>`,
>   `steps.<step>.loop.steps.<sub-step id>.query.<key>`, `executor_config.<provider>.base_url`.
> - **The executor families (DC-FORGE-233).** The reference's per-protocol table is spec data
>   (`credentialEndpointPairing.families`, keyed `protocol` or `protocol/driver`), each row's
>   server-variable set evaluated, so a new family is a spec change. A step warns when its secret
>   (the step query first, then the family's `executor_config` block: `api_key` where the family
>   copies it into a secret, else the secret under `extra`) is exactly `${NAME}` for one of the
>   row's server variables on a row whose plugin expands references, and its endpoint (same order)
>   is certainly the author's: not templated, not a `${…}` reference, and not of the same origin as
>   the row's default (scheme, host and port, lower-cased, the scheme's default port filled). A
>   family whose client carries an implicit service credential (`getmd://`, the trove drivers) is
>   not judged; `nexus://` and its alias `aigentchat://` get the ai:// shape (an endpoint with no
>   key of its own and no `credentials:` mapping).
> - **Measured** against the reference at its DC-FORGE-233 commit (strict parse, then its full
>   save-door verdict). On the 403-file differential corpus (the 216 bundled flows including the 10
>   starter templates, the flow-create examples, all 111 conformance fixtures and the 49-shape
>   CFX-02 probe matrix) the reference emits **15** warnings of this code and this library emits
>   the same 15, `(code, field)` for `(code, field)`, all on the new fixtures: **no bundled flow
>   trips it**. On 56 further probe shapes (one per boundary: numeric, empty, null, binary and
>   timestamp values, a null `executor_config` block, userinfo, path, case and space around a
>   default, an unparseable URL, a bracketed host, a missing scheme, an empty port, another
>   family's variable, the `extra` block, a numeric loop sub-step id, an unused provider block, the
>   nexus own-key shapes) the pairs are identical except on two files: `credentials:` with a
>   non-string key (divergence #19, looser), and `ai://openai` with no operation, which the
>   reference refuses at its parse (only the verdict is compared there). The CFX-02 differential
>   stays at 0 divergences. 18 mutants (each exemption, each arm, each read order, the origin
>   reduction, the wiring): all killed.

> **v0.6.1 (2026-09-29) — a step query may not declare a server-owned parameter.** One new
> reference rule, ported to both ports together (JS port 0.15.1), with the same spec and fixtures.
>
> | Rule (reference) | Code | Severity | File |
> | --- | --- | --- | --- |
> | `validateNoServerOwnedStepQueryKeys` / `IsServerOwnedParamKey` (AIgentFlow CFX-05) | `server_owned_query_key` | error | `validate.serverownedquerykeys.go`, spec `serverOwnedQueryKeys` |
>
> - **Why.** The `aiv://` executor sends the running org's aigentverse credential, or a signed
>   token naming the running person, to the base URL in its parameters, and the reference used to
>   lay a step's `query:` over the credential resolver's output. A shared flow declaring
>   `aiv_base_url: https://attacker.example` therefore chose where that credential went. The
>   reference now discards such a value at run time and refuses it by name at save.
> - **Exactly as the reference.** The key set is data (`serverOwnedQueryKeys.keys`: `aiv_api_key`,
>   `aiv_base_url`, `aiv_delegation`), so a new server-owned key is a spec change. Matching is exact
>   and case-sensitive. The scanned surfaces are exactly the reference's two: a top-level step's
>   `query:` (field `steps.<step>.query.<key>`) and a loop sub-step's `query:` (field
>   `steps.<step>.loop.steps.<sub-step id>.query.<key>`, `StepID` the loop step). A parallel branch
>   or a `for_each` body is a top-level step. Any value is refused, `null` and templates included;
>   a key nested inside a query value, the name as a value, and a flow input parameter of that name
>   are not.
> - **Measured** with the reference at the rule's commit (strict parse, then
>   `ValidateFlowWithDetails`) over 345 files (the 216 bundled flows and every conformance
>   fixture): the `(code, field)` pairs for this rule are identical, and no bundled flow trips it.
>   Four more shapes (a null value, a numeric sub-step id `07`, an upper-case key, a nested key)
>   agree too. A loop sub-step without an id is refused at the reference's parse, so it never
>   reaches this rule there; here it is reported with the empty id beside `loop_step_id_required`.
>   Six mutants (a key dropped from the spec, either surface unscanned, the rule unwired, a folded
>   case, the sub-step addressed by index): all killed.

> **v0.6.0 (2026-09-28) — every save-door rule AIgentFlow added after v2.753.0, measured against
> the reference's own save-door verdict.** Since AIgentFlow v2.788.0 its save door and
> `/flows/validate` are one verdict (strict parse, then `ValidateFlowWithDetails`), so "refuses what
> AIgentFlow's save refuses" is now one target. **This release is Go-first:** the spec, the new
> fixtures and one changed fixture are not yet in the JS port, so `make parity-check` reports them
> as drift until it follows.
>
> | Rule (reference) | Code | Severity | File |
> | --- | --- | --- | --- |
> | `end` is an ordinary step id to the reachability and cycle walks too (v2.760.0, DC-FORGE-189) | `unreachable_step` / `potential_infinite_loop` stop treating `end` as terminal | warning | `validate.connectivity.go`, spec `reachabilityTerminalMarkers` |
> | `ValidateExecutorConfigEnvScopes` (v2.597.0, DC-FORGE-29) | `executor_config_env_scope` | error | `validate.executorconfigenv.go`, spec `executorConfigEnvScopes` |
> | inline `.exons` step intake (v2.777.0, DC-FORGE-214) | `exons_attributes` | error, engine only | `validate.exons.go` |
> | orchestrator `.exons` parse, spec presence, provider (the reference's `validateOrchestrator`) | `orchestrator_exons_parse_failed`, `orchestrator_exons_no_provider` | error | `validate.exons.go` |
> | orchestrator `.exons` intake (v2.777.0) | `exons_attributes` | error, engine only | `validate.exons.go` |
> | orchestrator `requirements.resources` (v2.767.0, DC-FORGE-205) | `exons_resources_unhonoured` | error | `validate.exons.go` |
> | a tool one of `orchestrator.tools` / `tools.allow` withholds (v2.760.0, DC-FORGE-190) | `orchestrator_tool_withheld` | warning | `validate.exons.go`, spec `orchestratorToolAllow` |
>
> - **Codes.** The reference refuses all of these at its strict parse, where a refusal carries one
>   generic code (`flow_structure_invalid`, or `yaml_parse_error` for the env-scope refusal). Each
>   code here is the reference's own name for the rule where it has one (its retroactive-rule id
>   `executor_config_env_scope` / `exons_attributes`, its warning code `orchestrator_tool_withheld`)
>   and names the message otherwise. The reference stops at the first refusal; this library reports
>   each. The verdict is the same.
> - **The `.exons` engine is optional (divergence #16).** Judging whether a document parses and
>   whether its tags render needs go-exons. `Options.Exons` takes an `ExonsInspector`; the companion
>   module `github.com/itsatony/go-aigentflow-validator/exonsinspect` implements it with go-exons,
>   built exactly as the reference builds it (`exons.New(exons.WithEnvDisabled())`, `Parse`, and
>   `Validate(...).Errors()`), at the go-exons version the reference pins (v0.37.0). Without it the
>   built-in reader reads the frontmatter only, following the engine's extraction rules (a
>   15-shape matrix in `exonsinspect` pins that the two agree on every frontmatter shape).
> - **The env-scope table is the reference's, evaluated.** `executorConfigEnvScopes.scopes` is the
>   reference's `executorConfigEnvScope` evaluated for every key with a non-empty scope (38 keys);
>   any other key refuses every reference. It is not a prefix rule and it is not hand-written.
>
> **Measured** with the differential (below) on a 371-file corpus: the 216 bundled flows (including
> the 10 starter templates), 12 authoring-guide examples, all 94 conformance fixtures and a 49-shape
> probe matrix (one or more shapes per rule, each side of each rule). The reference refuses 62.
>
> | | verdict agrees | error codes agree | warning codes agree |
> | --- | --- | --- | --- |
> | v0.5.1 | 342 / 371 | 342 | — (misses every `orchestrator_tool_withheld`; `unreachable_step` on a real `end`) |
> | v0.6.0 with `exonsinspect` | **371 / 371** | **371** | **371** (outside divergences #14 and #17) |
> | v0.6.0, built-in reader | 359 / 371 | 359 | 359; the 12 are all divergence #16, all looser |
>
> On the 216 bundled flows alone the verdicts were already identical at v0.5.1: none of them trips
> a new rule. 16 mutants of the new rules (each rule removed, each guard inverted, the scope table
> emptied, the `end` marker restored, the signal exemption dropped, the templated-document skip
> removed): all killed by the conformance fixtures and unit tests.

> **v0.5.1 (2026-09-28) — a number where the reference expects text is that text, in every
> Go-`string` field.** The scalar-VALUE counterpart of v0.5.0's numeric step KEYS. The reference
> decodes a step reference, an enum, a name, a template and a duration into a Go `string`, and
> yaml.v3 fills a `string` from any scalar by its **source text**: `next: { default: 2 }` names the
> step `"2"`, `action: 1` is the invalid action `"1"`, `name: 123` is the name `"123"`. This library
> read most of those fields with a string-only assertion, so a number was either **invisible** (an
> existence or enum check skipped — looser) or **absent** (a present value reported missing —
> stricter). The spec is unchanged (`v2.753.0`); measured against the reference at
> aigentflow v2.786.0.
>
> | Shape | Reference (measured) | v0.5.0 | v0.5.1 |
> | --- | --- | --- | --- |
> | `next.default: 2`, a condition's `goto: 7`, `parallel.steps: [5]`, `rendezvous: 9`, `error_strategy.goto_step: 9` — no such step | refused, step not found | valid (default, goto); `goto_step_missing` / `missing_required_field` (goto_step, rendezvous) | `step_not_found` |
> | the same with the step present | saves, step reachable | `unreachable_step`; `step_not_found` / `goto_step_missing` (parallel member, goto_step) | valid, reachable |
> | `default: 1e3` with a step `1000` | refused (`1e3` ≠ `1000`) | valid | `step_not_found` |
> | step keys `1e3:`, `0x1F:`, `True:`, `2024-01-01:` named by the same spelling | saves | `unreachable_step` (keys read `1000`, `31`, `true`, a `time.Time`) | valid |
> | `action: 1`, `on_fail: 1`, `for_each.resolution: 1`, `orchestrator.mode: 1` | refused, invalid value | valid | the rule's own code |
> | `name: 123`, `aigentflow_version: 2.0`, `start: 1`, `rubric: 5`, `for_each.items: 5`, `inject_as: 5`, a `query` param/property `type: 1` | saves | `missing_required_field` / rule-specific "missing" | valid (`unknown_data_type` warning for the type) |
> | loop sub-steps `id: 3` / `id: 4`, `next: { default: 4 }` | saves | `loop_step_id_required`, `invalid_type` | valid |
> | `executor: 42` | refused, unusable executor URL | `invalid_type` | `invalid_executor_url` |
> | `expression_functions: [{ function: 5 }]` | refused, not in the catalog | `invalid_expression_function` | `expression_function_unknown` |
>
> - **One reader.** Every Go-`string` field is read through `scalarTextAt` (as `issues.stringOf` /
>   `issues.stringAt`), which returns a string as itself and a number, boolean or timestamp by its
>   source spelling. Only a mapping, a list or null is "not a string". Sites that compare a value
>   only against a literal no scalar spelling can equal (`orchestrator`, an `fn_` name, a template
>   `{{`, an empty `output:` entry) are left as they are; a step's `query:` is a Go `any` in the
>   reference and is not a string field.
> - **Keys too.** `ParseFlow` now retags every number, boolean and timestamp mapping KEY as a string
>   before decoding, so `1e3:` is the step `"1e3"` (v0.5.0 rendered keys with `fmt.Sprint`, which
>   gave `1000`, `31` for `0x1F`, and `2024-01-01 00:00:00 +0000 UTC` for a date). Keys and
>   references now agree on every spelling, and so do the source paths `collectScalarSources` keys
>   by. `TestYAMLStringFieldReceivesSourceText` pins the premise against yaml.v3 itself for 15
>   spellings.
> - **Measured.** Ten new conformance fixtures (`*-numeric-*`): the reference's verdict matches this
>   library on all 75 fixtures (v0.5.0: 70). A 26-shape probe matrix, one per rule: v0.5.0 agreed on
>   9, v0.5.1 on 24; the two left are unrelated and predate this change (an unknown orchestrator tool
>   is a warning by default, divergence #7's family; a flow-level `input_schema` field name the
>   reference saves is refused here whether written `5` or `'5'`). The 216 bundled flows: zero
>   findings changed.
> - **The JS port has the same defect** (its readers test `typeof === 'string'`), and the ten
>   fixtures are **Go-first**: until they are ported there, `make parity-check` reports them as
>   drift.

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

> **v0.5.0 (2026-09-26) — the three gaps that were looser than the reference in both ports are
> closed, together with the JS port's 0.14.0.** The spec (now `v2.753.0`: the reference's grammar
> sources have a zero diff from `v2.738.0`) and all 65 conformance fixtures are byte-identical to
> that branch. Every verdict was measured on the reference's strict save parser
> (`NewStrictFlowParser().ParseFromYAMLBytes`, then `ValidateFlowWithDetails`) over a 133-shape
> probe matrix; both ports match it on all 133 shapes and all 65 fixtures.
>
> | Gap | Reference verdict (measured) | Before | Now |
> | --- | --- | --- | --- |
> | `next: { default: end }`, a condition's `goto: end`, no step `end` | refused, "step_id (end) … not found" | valid | `step_not_found` |
> | the same, with a step named `end` | saves; `end` step `unreachable_step` | valid | valid, same warning |
> | `next: end` (scalar), `next: [a]`, `tags: x`, `output: {a: 1}` | refused, cannot decode | valid | `invalid_type` |
> | an unknown key at any struct level (root, step, `next`, `error_strategy`, loop sub-step, mock step, query definition, trigger, …) | refused, "field X not found in type T" | valid | `unknown_yaml_key` |
> | the same key with a null value | refused | valid | `unknown_yaml_key` |
> | a mock `delay` of `0.0`, `00`, `0x0`, `.0`, `-0.0`, `0.`, `0o0`, `0_0` | refused (no unit) | valid | `mock_delay_invalid` |
> | a mock `delay` of `0`, `+0`, `-0`, `"0"` | saves | valid | valid |
> | `throttle.delay` / `batch_delay` / `error_strategy.max_delay` of `100`, `1.5`, `0.0` | refused | valid (numbers skipped) | `invalid_duration` |
> | a timer trigger `interval: 0` | saves | `orchestrator_timer_no_interval` | valid |
> | a step id written as a number (`1:`) | saves | `invalid_type` | valid |
>
> - **`end`:** the existence check now uses the save door's sentinels (spec `nextMarkers`: `null`,
>   `orchestrator`); reachability and cycles keep `end` as terminal (spec
>   `reachabilityTerminalMarkers`), as the reference's walks do. Divergence #8 is closed. It was
>   hiding a real problem: 15 shared fixtures called valid routed to `end`, and the reference refused
>   all 15. They now say `'null'`.
> - **Unknown keys and value kinds:** `validate.unknownkeys.go`, driven by `knownKeys` in the spec,
>   which is derived by reflection over the reference's types (see the JS port's `PARITY.md`,
>   "Unknown keys and value kinds", for the derivation and its 771-check verification). Divergence #9
>   is closed. It fires zero times on the 216 bundled flows.
> - **Source spelling:** `ValidateFlow` records the source text of every number and boolean scalar
>   (`collectScalarSources`) and judges a Go-`string` duration field by it. Divergence #15 is
>   narrowed to `ValidateFlowObject`.
> - **Fixed here: a mapping with a non-string key.** yaml.v3 decodes `steps: {1: …}` into
>   `map[any]any`, which every validator read as "not a mapping", so a numeric step id was refused.
>   `ParseFlow` and `ValidateFlowObject` now normalise such keys to strings, as the reference's typed
>   decode does (without mutating a caller's document).
>
> Bundled corpus (216 files): with `StrictRegistries` the verdict matches the reference on all 216;
> `unreachable_step` 25 = 25 and `potential_infinite_loop` 1 = 1.

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

### 8. `end` as a next target — CLOSED in v0.5.0 (shared with JS, divergence #2 there)

Both ports used to treat `null`, `end` and `orchestrator` as terminal markers everywhere. The
reference's connectivity check exempts `end`, but its save-door parser (`validateNextLogic`) does
**not**: it refuses `next: { default: end }` because no step is called `end`. Both ports now
follow each half: the existence check uses the save door's set, and reachability and cycles keep
`end` as terminal. The shared fixtures that routed to `end` now route to `'null'`.

**Since v0.6.0 (reference v2.760.0) the second half is gone too:** no reference walk treats `end`
as terminal, so a real step named `end` is reachable and a cycle through it is a cycle. Spec
`reachabilityTerminalMarkers` is now `["null", "orchestrator"]`, the same values as `nextMarkers`;
the two keys stay separate so a future split is a one-value change.

### 9. Unknown keys — CLOSED in v0.5.0 (shared with JS)

The reference's save door parses with `KnownFields(true)`, so any key its types do not declare is
refused, at every depth. `validate.unknownkeys.go` now refuses the same keys (`unknown_yaml_key`)
and the same wrong value kinds (`invalid_type`), from the derived `knownKeys` inventory in the
spec. What stays open is what the reference leaves open: Go maps (author-chosen keys), interfaces
(`any`: a step's `query`, `data`, a mock's `content`), and a processing-operation entry, which has
its own unmarshaller (divergence #12). Scalars are judged for kind only.

### 10. The loop body is walked for every step (shared with JS)

The reference walks the loop body only for steps reachable from `start`. This library walks every
step. All loop-body findings are warnings, so no verdict changes.

### 11. The `fn_` usage scan walks the document (shared with JS, divergence #10 there)

The reference re-serialises its typed `Flow` and scans that, so it sees only declared fields. This
library walks every string leaf of the parsed document. The two agree on every flow the reference
would accept, because anything else is refused by the reference's unknown-key rule, which this
library now reproduces too.

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
- Plugin pre-validation: the reference's save door also runs a check each executor plugin may
  register (today only `script://`, which compiles its script). It depends on the plugin registry
  and is out of scope; no bundled flow trips it.

### 15. A number in a duration field without source text is judged by its parsed value (shared with JS, #13 there) — NARROWED in v0.5.0 and v0.5.1

**Since v0.5.0 this applies to `ValidateFlowObject` and to a value reached through a YAML alias
only.** `ValidateFlow` records the source spelling of every number, boolean and (since v0.5.1)
timestamp scalar and judges the mock `delay`, `throttle.delay`, `throttle.batch_delay`,
`error_strategy.max_delay`, a timer `interval` and `tool_discovery` by it, exactly as the
reference's `string` fields receive them. **Since v0.5.1 the same holds for every Go-`string`
field** — step references, enums, names — and for mapping keys. The rest of this section describes
the remaining case; under `ValidateFlowObject` a step reference and a numeric key are both rendered
from the decoded value, so they still agree for every ordinary spelling (`2`, `true`), and differ
from the reference only for spellings whose text the decode lost (`1e3`, `0x1F`).

yaml.v3 fills the reference's `string` field with the scalar's **source text**; this library decodes
into untyped values and only has the parsed number. They agree on every number except those that
parse to zero without being written `0`, `+0` or `-0`: `delay: 0.0`, `00` and `0x0` are refused by
the reference (no unit) and accepted here. Measured on the reference's strict save parser: `0`,
`+0`, `-0` save; `0.0`, `00`, `0x0`, `100` are refused. Every other number is refused on both sides,
because no number's text carries a unit.

**Direction: looser than the reference.** The fix an author needs is the one the refusal already
gives: write a unit. A campaign `flow_id` / `flow_name` also uses the parsed value, where it cannot
change a verdict (no number spelling is empty or a template).

### 16. `.exons` documents: the engine's judgement needs the engine (v0.6.0)

The reference judges an inline `.exons` document with go-exons: whether it parses (grammar,
frontmatter decode and the spec's own validation), and whether every tag can render. This module
has no `.exons` template engine and must not grow one, because a second implementation of that
grammar is a second opinion about it.

- **With `Options.Exons: exonsinspect.New()`** the verdict is the reference's: 371 of 371 on the
  differential corpus.
- **With the built-in reader** (`Options.Exons` nil) the frontmatter is read the way the engine
  extracts and decodes it, so `orchestrator_exons_parse_failed` (no spec, an unclosed frontmatter,
  the retired config block), `orchestrator_exons_no_provider`, `exons_resources_unhonoured` and
  `orchestrator_tool_withheld` are the reference's. What it cannot see: a document that does not
  parse for any other reason (including a spec the engine's own validation refuses, such as a
  missing `description`), a tag that cannot render (`exons_attributes`, on steps and on the
  orchestrator), and a frontmatter containing a tag (the engine executes it before decoding, so it
  is not read at all).

**Direction: looser than the reference without the engine; identical with it.** Never stricter:
the built-in reader refuses only where the engine refuses as well (pinned by the 15-shape matrix in
`exonsinspect`). The JS port shares the gap (its divergence "exons body not parsed").

### 17. `input_schema_file_after_parametric` is spelt `INPUT_SCHEMA_FILE_AFTER_PARAMETRIC` by the reference

The reference's save door merges its `input_schema` ordering lint into the verdict with its own
code, which is upper case, at field `doc`. Both ports have always reported it as
`input_schema_file_after_parametric` at the offending field (`input_schema.fields[N]`). A warning
only; renaming a published code would break every consumer that branches on it, so it stays.

### 18. `unresolvable_data_path`: not ported (reference-only error)

The reference refuses a `.data.<step>.<field>` template reference to a field that `<step>`'s
`output_schema` does not declare (`unresolvable_data_path`). Judging it means resolving template
field references against the step graph, the same runtime field-resolution this validator
deliberately does not model for templates, so a document carrying only this defect is **valid here
and refused by the reference**. Looser only, never stricter: a consumer that admits on this
package's verdict may accept a flow the reference will refuse at save. `validate.outputschema.go`
names this entry.

### 19. `credential_endpoint_unpaired`: what the static rule cannot see (v0.6.2, shared with JS)

The reference's save-time rule is itself static, and it is ported exactly. What it predicts is a
**run-time** refusal that reads things no document carries: the VALUES of the server's environment
variables (whether `${AIGENTFLOW_…}` is set, and whether a literal endpoint happens to equal the
deployment's configured one), the credential resolver's output (which stored credential a step is
given, and that credential's own base URL), the service configuration an executor fills, and
whether a client attaches the deployment's service-to-service credential. Neither the reference's
save-time rule nor this library reads any of them; the run time is the control.

- **Direction: identical to the reference's warning.** The warning is a subset of the run-time
  refusals (a flow that saves without it can still be refused at run time, e.g. an org holding no
  key for a literal endpoint the run supplies no key for).
- **The default endpoints of the two rows the rule never reads are not carried.** `getmd://`
  (implicit credential, never judged) and `nexus://` (the ai:// shape, which consults no default)
  have deployment-internal defaults in the reference; their rows carry `defaultEndpoints: []`.
  Verdict-neutral: no static path reads them.
- **A `credentials:` mapping with a non-string key (looser).** The reference decodes a step query
  into Go `any`, where yaml.v3 gives a mapping with any non-string key the type `map[any]any`, and
  its nexus own-key check asserts `map[string]any`, so `credentials: {1: x}` is no key there and
  the endpoint warns. This library stringifies every mapping key at parse (v0.5.0), so the same
  mapping is a key of the step's own and nothing warns. Looser only, on a shape no credentials map
  AIgentFlow resolves can take.
- **A document the reference refuses at its parse** reports this warning here beside the
  refusing error (as every rule does, PARITY.md "The differential"); the reference stops first.

## The differential

The reference is closed source, so the differential runs in two halves. On the reference's side a
program runs its save-door verdict (the strict parse, then `ValidateFlowWithDetails`, with the
plugin pre-validation hook registered) over a corpus and writes one JSON file:
`{absolute path: {valid, parse_refusal, message, errors: [{code, field}], warnings: [...]}}`. Here,
`make differential AIF_REFERENCE_VERDICTS=/path/to/that.json` runs
`exonsinspect/differential_test.go`, which validates each file twice (with `exonsinspect` and with
the built-in reader, both under `StrictRegistries`) and fails on any divergence outside #14, #16,
#17 and #18. Where the reference refused at its parse (`parse_refusal`), only the verdict is compared,
because such a refusal carries no rule code. The test skips when the variable is unset, and fails on
a one-sided corpus.

## Not ported (owed)

- The numeric-spelling gap for `ValidateFlowObject` (divergence #15), which has no source text to
  recover.

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
4. Port the new rule, with its `code` constant added to `keys.go`. A new or renamed field in any
   flow type arrives as a `knownKeys` change in the copied spec; nothing to port for it here.
5. `make ci && make parity-check`, and `make differential` against a fresh reference verdict file.
6. Note the version and the ported rule at the top of this file's tracked-version line.

## Consumers that BLOCK on a verdict

If you are using this as an admission gate (aigentverse does), two obligations follow from the
pinned-schema model:

- **Surface `Result.SpecVersion`** on every rejection. See README.
- **Keep `StrictRegistries` off.** The registry-lag warnings exist precisely so a real-but-newer
  executor scheme, template function, or orchestrator tool cannot block a publish.
- **Pass `Options.Exons: exonsinspect.New()`** when the gate predicts what AIgentFlow will save.
  Without it a flow whose `.exons` document AIgentFlow's engine refuses passes the gate
  (divergence #16).
