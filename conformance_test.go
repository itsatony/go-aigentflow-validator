package aifvalidate

import (
	"os"
	"path/filepath"
	"testing"
)

// The conformance suite. Each fixture is validated and compared against an
// expected verdict.
//
// This is the SAME fixture set and the SAME case table as
// aigentflow-flow-validator-js's test/conformance/conformance.test.ts — the
// fixtures under testdata/conformance/ are copied verbatim from that repo. It is
// the regression net that catches drift between the two implementations and from
// the AIgentFlow reference.
//
// Comparison is STRUCTURAL: the `valid` flag exactly, plus the expected
// error/warning codes as a SUBSET of the actual. Never exact message text —
// wording is explicitly not part of the parity contract. The subset (rather than
// equality) rule is also what tolerates this implementation being strictly
// stronger on template_syntax_error, since it uses Go's real text/template
// parser rather than the JS approximation. See PARITY.md.
//
// To extend: drop a new .yaml under testdata/conformance/ and add a case here —
// and add the same fixture and case upstream in the JS repo.

type conformanceCase struct {
	file string
	// valid is the expected verdict, asserted exactly.
	valid bool
	// wantErrorCodes must ALL be present among the actual error codes.
	wantErrorCodes []string
	// wantWarningCodes must ALL be present among the actual warning codes.
	wantWarningCodes []string
	// forbidWarningCodes must NOT be present. A rule whose whole risk is a false
	// positive needs a fixture that goes red when it fires, and `valid: true`
	// cannot say that: a warning never changes the verdict.
	forbidWarningCodes []string
	// forbidErrorCodes must NOT be present. `valid: true` already fails on any
	// error; this NAMES the rule under test so a regression reads clearly.
	forbidErrorCodes []string
	// exonsEngineOnly marks a fixture whose verdict needs an .exons ENGINE
	// (PARITY.md, divergence #16). The case states the reference's verdict;
	// this suite, which runs the built-in reader, asserts the documented
	// looser one instead — valid, with none of wantErrorCodes — and the
	// exonsinspect module asserts the reference's.
	exonsEngineOnly bool
}

// conformanceCases mirrors CASES in the JS repo's conformance.test.ts, case for
// case, and TestConformanceFixturesAreAllCovered keeps the table complete.
var conformanceCases = []conformanceCase{
	{file: "valid-minimal.yaml", valid: true},
	{
		// v2.648.0 (DC-FORGE-78): the loop body is walked. Every template here is
		// unparseable and the flow must still be VALID: loop-body findings are
		// warnings.
		file: "valid-loop-body-templates-warn.yaml", valid: true,
		wantWarningCodes: []string{codeTemplateSyntax},
	},
	{
		// The control: nothing in a clean loop body may warn.
		file: "valid-loop-body-clean.yaml", valid: true,
		forbidWarningCodes: []string{codeTemplateSyntax, codeTemplateFuncUnkn,
			codeUnknownProcessingOp, codeUnknownProcessingConfigKey},
	},
	{
		// The partition: loop.set is dispatchable ONLY on a loop sub-step.
		file: "valid-loop-only-op-at-top-level-warns.yaml", valid: true,
		wantWarningCodes: []string{codeUnknownProcessingOp},
	},
	{
		// v2.651.0 (DC-FORGE-81): goto_step beside a non-goto action.
		file: "warn-unreachable-error-goto.yaml", valid: true,
		wantWarningCodes: []string{codeUnreachableErrorGoto},
	},
	{
		file: "valid-reachable-error-goto.yaml", valid: true,
		forbidWarningCodes: []string{codeUnreachableErrorGoto},
	},
	{
		// v2.652.0 (DC-FORGE-82): goto_step inside a loop body is read by nothing.
		file: "warn-loop-substep-error-goto.yaml", valid: true,
		wantWarningCodes:   []string{codeLoopSubstepErrGotoIgnore},
		forbidWarningCodes: []string{codeUnreachableErrorGoto},
	},
	{
		// The counter-fixture: the warning's own remedy must not warn.
		file: "valid-loop-substep-error-continue.yaml", valid: true,
		forbidWarningCodes: []string{codeLoopSubstepErrGotoIgnore, codeUnreachableErrorGoto},
	},
	{
		// v2.672.0 (DC-FORGE-102): forward jump, backward jump, empty target.
		file: "valid-loop-substep-next.yaml", valid: true,
		forbidErrorCodes: []string{codeLoopSubstepNextNotFound, codeLoopSubstepNextSentinel,
			codeLoopSubstepNextParallel},
	},
	{
		file: "invalid-loop-substep-next-unknown-target.yaml", valid: false,
		wantErrorCodes: []string{codeLoopSubstepNextNotFound},
	},
	{
		file: "invalid-loop-substep-next-sentinel.yaml", valid: false,
		wantErrorCodes: []string{codeLoopSubstepNextSentinel},
	},
	{
		file: "invalid-loop-substep-next-parallel.yaml", valid: false,
		wantErrorCodes: []string{codeLoopSubstepNextParallel},
	},
	{
		// DC-FORGE-145: response_expectation is read only with response_evaluation.
		file: "warn-response-expectation-unread.yaml", valid: true,
		wantWarningCodes: []string{codeRespExpUnread},
	},
	{
		file: "valid-response-expectation-raw-text.yaml", valid: true,
		forbidWarningCodes: []string{codeRespExpUnread},
	},
	{
		file: "valid-response-expectation-async.yaml", valid: true,
		forbidWarningCodes: []string{codeRespExpUnread},
	},
	{
		file: "valid-response-expectation-absent.yaml", valid: true,
		forbidWarningCodes: []string{codeRespExpUnread},
	},
	{
		// A condition's target key is `goto`; `goto_step` there is refused.
		file: "invalid-condition-goto-step-misspelling.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		// v2.608.0: executor URLs are parsed with the reference's ONE parser.
		file: "invalid-executor-url-shapes.yaml", valid: false,
		wantErrorCodes: []string{codeInvalidExecutorURL},
	},
	{
		// The `{{` exception is load-bearing.
		file: "valid-templated-executor-url.yaml", valid: true,
	},
	{file: "valid-branching.yaml", valid: true},
	{
		// CLEANER POWER Phase 2: wait:// + eval:// schemes, output_schema, quality_gate.
		file: "valid-wait-eval-schema-gate.yaml", valid: true,
	},
	{
		file:  "invalid-output-schema-and-quality-gate.yaml",
		valid: false,
		wantErrorCodes: []string{
			codeISInvalidVersion,
			codeISInvalidFieldName,
			codeQGMissingRubric,
			codeQGThresholdRange,
			codeQGOnFailUnsupported,
		},
	},
	{
		// Skope retired in v2.435.0 → skope:// is an unknown scheme: warns, does
		// not reject. The guard against re-hardening scheme checks.
		file: "valid-retired-skope-scheme-warns.yaml", valid: true,
		wantWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		file:           "invalid-missing-fields.yaml",
		valid:          false,
		wantErrorCodes: []string{codeMissingField, codeStepNotFound},
	},
	{
		// v2.642.0 (DC-FORGE-72): the expression-function catalog.
		file: "invalid-expression-function-package.yaml", valid: false,
		wantErrorCodes: []string{codeExprFnPackageUnsupported},
	},
	{
		file: "invalid-expression-function-unknown-name.yaml", valid: false,
		wantErrorCodes: []string{codeExprFnUnknown},
	},
	{
		file: "invalid-expression-function-undeclared-use.yaml", valid: false,
		wantErrorCodes: []string{codeExprFnUndeclaredUse, codeExprFnUnknownUse},
	},
	{file: "valid-expression-functions.yaml", valid: true},
	{file: "valid-expression-function-prose-mention.yaml", valid: true},
	{
		// A FIELD, a data KEY or a STEP whose name begins with fn_ is not a call.
		file: "valid-expression-function-field-lookalikes.yaml", valid: true,
	},
	{
		// v2.647.0: an operation type the standard handler cannot dispatch.
		file: "valid-processing-operation-unknown-type-warns.yaml", valid: true,
		wantWarningCodes:   []string{codeUnknownProcessingOp},
		forbidWarningCodes: []string{codeUnknownProcessingConfigKey},
	},
	{
		// asset_id under binary.transform: real for binary.get, wrong here.
		file: "valid-processing-config-key-wrong-half-warns.yaml", valid: true,
		wantWarningCodes: []string{codeUnknownProcessingConfigKey},
	},
	{
		file: "valid-processing-config-keys-accepted.yaml", valid: true,
		forbidWarningCodes: []string{codeUnknownProcessingOp, codeUnknownProcessingConfigKey},
	},
	{
		// v2.648.0: a sub-step id that is also a loop-result summary field.
		file: "warn-loop-sub-step-id-reserved.yaml", valid: true,
		wantWarningCodes: []string{codeLoopSubStepIDReserved},
	},
	{
		// v2.721.0 (DC-FORGE-150): budget and max_retries were deleted.
		file: "invalid-retired-flow-budget.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		file: "invalid-retired-flow-max-retries.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		file: "invalid-retired-step-max-retries.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		// The working keys one level down must not be refused.
		file: "valid-limit-keys-one-level-down.yaml", valid: true,
	},
	{
		file:  "invalid-references-and-templates.yaml",
		valid: false,
		wantErrorCodes: []string{codeInvalidExecutorURL, codeTemplateSyntax,
			codeStepNotFound, codeInvalidErrStrategy},
	},
	{
		// DC-FORGE-147: a max_duration the engine cannot apply.
		file: "warn-step-max-duration-ignored.yaml", valid: true,
		wantWarningCodes: []string{codeStepMaxDurationIgnored},
	},
	{
		file: "valid-step-max-duration-applied.yaml", valid: true,
		forbidWarningCodes: []string{codeStepMaxDurationIgnored},
	},
	// Reference save-door refusals this library carried first; the JS port
	// ported them in its 0.13.0 and added these fixtures. Each invalid fixture
	// was checked against the reference's strict save parser, and a copy with
	// only the offending value corrected was checked to SAVE.
	{
		file: "invalid-reserved-step-id-orchestrator.yaml", valid: false,
		wantErrorCodes: []string{codeReservedStepIDOrch},
	},
	{
		file: "invalid-mock-scenario-delay.yaml", valid: false,
		wantErrorCodes: []string{codeMockDelayInvalid},
	},
	{
		file: "valid-mock-scenario-delay.yaml", valid: true,
		forbidErrorCodes: []string{codeMockDelayInvalid},
	},
	{
		file: "invalid-output-param-empty.yaml", valid: false,
		wantErrorCodes: []string{codeOutputParamEmpty},
	},
	{
		// yaml.v3 drops a null list entry, so the reference saves this.
		file: "valid-output-null-entry.yaml", valid: true,
		forbidErrorCodes: []string{codeOutputParamEmpty},
	},
	{
		// v0.5.0: `end` is looked up as a STEP by the reference's save door, in a
		// default and in a condition's goto alike, and no step is `end` here.
		file: "invalid-next-end-without-step.yaml", valid: false,
		wantErrorCodes: []string{codeStepNotFound},
	},
	{
		// A real step named `end` saves and is REACHABLE: since v2.760.0
		// (DC-FORGE-189) no reference walk treats `end` as a terminal.
		file: "valid-next-end-names-a-step.yaml", valid: true,
		forbidWarningCodes: []string{codeUnreachableStep},
		forbidErrorCodes:   []string{codeStepNotFound},
	},
	{
		// KnownFields(true) at every depth: seven keys, seven findings.
		file: "invalid-unknown-keys.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		// `next: end` is a scalar where a mapping is required; `tags:` a list.
		file: "invalid-value-kind.yaml", valid: false,
		wantErrorCodes: []string{codeInvalidType},
	},
	{
		// The false-positive guard for the unknown-key rule.
		file: "valid-open-key-sets.yaml", valid: true,
		forbidErrorCodes: []string{codeUnknownYAMLKey, codeInvalidType},
		// `default: 1` reaches the step `1`: a number in a step reference is
		// the step id of its text (v0.5.1).
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		// A number in a Go `string` duration field is its SOURCE text.
		file: "invalid-duration-numeric-spellings.yaml", valid: false,
		wantErrorCodes: []string{codeMockDelayInvalid, codeInvalidDuration},
	},
	{
		// 0, +0, -0 save.
		file: "valid-duration-numeric-zero.yaml", valid: true,
		forbidErrorCodes: []string{codeMockDelayInvalid, codeInvalidDuration},
	},
	// v0.5.1: a step REFERENCE written as a number is the step id of its source
	// text, because the reference decodes every reference into a Go `string`.
	// Each pair below is one reference kind, refused and accepted; every verdict
	// was measured on the reference's strict save parser. These ten fixtures are
	// GO-FIRST: the JS port has the same defect and does not carry them yet.
	{
		file: "invalid-numeric-next-default-unknown.yaml", valid: false,
		wantErrorCodes: []string{codeStepNotFound},
	},
	{
		file: "valid-numeric-next-default.yaml", valid: true,
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		file: "invalid-numeric-condition-goto-unknown.yaml", valid: false,
		wantErrorCodes: []string{codeStepNotFound},
	},
	{
		file: "valid-numeric-condition-goto.yaml", valid: true,
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		file: "invalid-numeric-parallel-member-unknown.yaml", valid: false,
		wantErrorCodes: []string{codeStepNotFound},
	},
	{
		// A false step_not_found on the member and a false missing rendezvous.
		file: "valid-numeric-parallel-member.yaml", valid: true,
		forbidErrorCodes:   []string{codeStepNotFound, codeMissingField},
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		// Refused as a MISSING STEP — the goto_step is present.
		file: "invalid-numeric-error-goto-unknown.yaml", valid: false,
		wantErrorCodes:   []string{codeStepNotFound},
		forbidErrorCodes: []string{codeGotoStepMissing},
	},
	{
		file: "valid-numeric-error-goto.yaml", valid: true,
		forbidErrorCodes:   []string{codeGotoStepMissing},
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		// `1e3`, `0x1F` and `True`, as keys and as references, are their text.
		file: "valid-numeric-reference-spellings.yaml", valid: true,
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	{
		// `1e3` is not the step `1000`, although both are the number 1000.
		file: "invalid-numeric-reference-spelling-mismatch.yaml", valid: false,
		wantErrorCodes: []string{codeStepNotFound},
	},

	// v0.6.0 — the save-door rules AIgentFlow added after v2.753.0.
	{
		// v2.597.0 (DC-FORGE-29): five out-of-scope references, one per shape.
		file: "invalid-executor-config-env-scope.yaml", valid: false,
		wantErrorCodes: []string{codeExecutorConfigEnvScope},
	},
	{
		file: "valid-executor-config-env-scope.yaml", valid: true,
		forbidErrorCodes: []string{codeExecutorConfigEnvScope},
	},
	// v0.17.0 (AIgentFlow v2.833.0): the agent surfaces AIgentFlow removed.
	{file: "invalid-executor-removed-exons.yaml", valid: false,
		wantErrorCodes: []string{codeExecutorRemoved}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "invalid-executor-removed-agent.yaml", valid: false,
		wantErrorCodes: []string{codeExecutorRemoved}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "invalid-executor-removed-scheme-case-whitespace.yaml", valid: false,
		wantErrorCodes: []string{codeExecutorRemoved}, forbidErrorCodes: []string{codeInvalidExecutorURL}},
	{file: "invalid-executor-removed-loop-substep.yaml", valid: false, wantErrorCodes: []string{codeExecutorRemoved}},
	{file: "invalid-executor-removed-exons-uploads.yaml", valid: false,
		wantErrorCodes: []string{codeExecutorRemoved}, forbidErrorCodes: []string{codeUnknownYAMLKey}},
	{file: "invalid-agentic-param-removed.yaml", valid: false, wantErrorCodes: []string{codeAgenticParamRemoved}},
	{file: "invalid-agentic-param-removed-false.yaml", valid: false, wantErrorCodes: []string{codeAgenticParamRemoved}},
	{file: "invalid-agentic-param-removed-null-value.yaml", valid: false, wantErrorCodes: []string{codeAgenticParamRemoved}},
	{file: "invalid-agentic-param-removed-loop-substep.yaml", valid: false, wantErrorCodes: []string{codeAgenticParamRemoved}},
	{
		// No false positive: aiv://, atlas://, a templated executor, a non-ai
		// scheme and chat-only ai:// parameters.
		file: "valid-removed-surfaces-not-refused.yaml", valid: true,
		forbidErrorCodes:   []string{codeExecutorRemoved, codeAgenticParamRemoved},
		forbidWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		// v2.760.0 (DC-FORGE-189): `end` is a step, so this is a cycle.
		file: "warn-next-end-cycle.yaml", valid: true,
		wantWarningCodes:   []string{codePotentialInfiniteLop},
		forbidWarningCodes: []string{codeUnreachableStep},
	},
	// A cycle is a potential infinite loop only when control cannot leave it. The
	// valid fixtures forbid the warning (a valid:true check cannot express a
	// false positive); the warn fixtures must keep it.
	{file: "valid-cycle-ask-again-exit.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-self-loop-exit.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-condition-null.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-no-default-ends.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-contains-step-without-next.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-parallel-exit.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "valid-cycle-unreachable-not-reported.yaml", valid: true, forbidWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "warn-cycle-two-steps-no-exit.yaml", valid: true, wantWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "warn-cycle-self-loop-no-exit.yaml", valid: true, wantWarningCodes: []string{codePotentialInfiniteLop}},
	{file: "warn-cycle-after-exiting-cycle.yaml", valid: true, wantWarningCodes: []string{codePotentialInfiniteLop}},
	// v0.6.1 (AIgentFlow CFX-05): a step or loop sub-step query may not declare
	// a server-owned parameter. One fixture per key per surface, so dropping a
	// key from the spec set, or either surface from the scan, goes red here.
	{file: "invalid-server-owned-query-key-api-key.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{file: "invalid-server-owned-query-key-base-url.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{file: "invalid-server-owned-query-key-delegation.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{file: "invalid-server-owned-query-key-loop-api-key.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{file: "invalid-server-owned-query-key-loop-base-url.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{file: "invalid-server-owned-query-key-loop-delegation.yaml", valid: false, wantErrorCodes: []string{codeServerOwnedQueryKey}},
	{
		// The counter-fixture: similar names, another case, a nested key, the
		// name as a value, and a flow input parameter of that name all save.
		file: "valid-server-owned-query-key-lookalikes.yaml", valid: true,
		forbidErrorCodes: []string{codeServerOwnedQueryKey},
	},
	// v0.6.2 (AIgentFlow DC-FORGE-231 / DC-FORGE-233): an endpoint a
	// server-supplied credential will never be sent to. A WARNING, so each
	// warn-* fixture is valid, and the counter-fixtures FORBID the code: the
	// whole risk of a warning is a false positive, which `valid: true` cannot
	// see. The exact fields per fixture are pinned in validate_v0602_test.go.
	// v0.10.0 (AIgentFlow v2.821.0, #217): compliance_residency_unenforced. The
	// two controls forbid the code: a warning never changes the verdict, so
	// `valid: true` alone could not see a false positive.
	// v0.11.0 (AIgentFlow v2.827.0): the here:// scheme, the strict_templates key
	// and the template_default_piped rule. Fixtures are Go-first: the JS port has
	// not taken these yet.
	{file: "valid-executor-here.yaml", valid: true, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-strict-templates.yaml", valid: true, forbidErrorCodes: []string{codeUnknownYAMLKey, codeInvalidType}},
	{file: "invalid-strict-templates-not-bool.yaml", valid: false, wantErrorCodes: []string{codeInvalidType}},
	{file: "invalid-template-default-piped.yaml", valid: false, wantErrorCodes: []string{codeTemplateDefaultPiped}},
	{file: "warn-template-default-piped-loop.yaml", valid: true, wantWarningCodes: []string{codeTemplateDefaultPiped}, forbidErrorCodes: []string{codeTemplateDefaultPiped}},
	{
		file: "valid-template-default-lookalikes.yaml", valid: true,
		forbidWarningCodes: []string{codeTemplateDefaultPiped},
		forbidErrorCodes:   []string{codeTemplateDefaultPiped},
	},
	// v0.12.0 (AIgentFlow v2.828.0): flow_data_key_reserved. The valid fixture
	// forbids the code: a false positive on an author's key is the whole risk.
	{file: "valid-flow-data-keys.yaml", valid: true, forbidErrorCodes: []string{codeFlowDataKeyReserved}},
	{file: "invalid-flow-data-key-exact.yaml", valid: false, wantErrorCodes: []string{codeFlowDataKeyReserved}},
	{file: "invalid-flow-data-key-bookkeeping.yaml", valid: false, wantErrorCodes: []string{codeFlowDataKeyReserved}},
	{file: "invalid-flow-data-key-underscore-prefix.yaml", valid: false, wantErrorCodes: []string{codeFlowDataKeyReserved}},
	{file: "invalid-flow-data-key-dot-prefix.yaml", valid: false, wantErrorCodes: []string{codeFlowDataKeyReserved}},
	{file: "invalid-flow-data-key-namespace.yaml", valid: false, wantErrorCodes: []string{codeFlowDataKeyReserved}},
	// v0.13.0 (AIgentFlow v2.831.0): unknown_function_operation. The valid
	// fixtures forbid the code: a false positive on a real built-in is the risk.
	{file: "invalid-function-operation-unknown.yaml", valid: false, wantErrorCodes: []string{codeUnknownFunctionOperation}},
	{file: "invalid-function-operation-unknown-loop-substep.yaml", valid: false, wantErrorCodes: []string{codeUnknownFunctionOperation}},
	{file: "valid-function-operation-builtins.yaml", valid: true, forbidErrorCodes: []string{codeUnknownFunctionOperation}},
	{file: "valid-function-operation-templated.yaml", valid: true, forbidErrorCodes: []string{codeUnknownFunctionOperation}},
	// v0.15.0: the atlas:// executor scheme and its seven rules. The valid fixtures
	// forbid every atlas code and the unknown-scheme warning.
	{file: "invalid-atlas-operation-unknown.yaml", valid: false, wantErrorCodes: []string{"atlas_operation_unknown"}},
	{file: "invalid-atlas-operation-agent-resource.yaml", valid: false, wantErrorCodes: []string{"atlas_operation_unknown"}},
	{file: "invalid-atlas-agent-missing.yaml", valid: false, wantErrorCodes: []string{"atlas_agent_missing"}},
	{file: "invalid-atlas-agent-not-literal.yaml", valid: false, wantErrorCodes: []string{"atlas_agent_not_literal"}},
	{file: "invalid-atlas-capabilities-not-literal.yaml", valid: false, wantErrorCodes: []string{"atlas_capabilities_not_literal"}},
	{file: "invalid-atlas-prompt-missing.yaml", valid: false, wantErrorCodes: []string{"atlas_prompt_missing"}},
	{file: "invalid-atlas-prompt-both.yaml", valid: false, wantErrorCodes: []string{"atlas_prompt_both"}},
	{file: "invalid-atlas-capability-ref-invalid.yaml", valid: false, wantErrorCodes: []string{"atlas_capability_ref_invalid"}},
	{file: "valid-atlas-agent-run-prompt.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_operation_unknown", "atlas_prompt_both", "atlas_prompt_missing"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-agent-run-prompt-template.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_operation_unknown", "atlas_prompt_both", "atlas_prompt_missing"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-capability-call.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_operation_unknown", "atlas_prompt_both", "atlas_prompt_missing"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-templated-values.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_operation_unknown", "atlas_prompt_both", "atlas_prompt_missing"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-capability-args-templated.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_operation_unknown", "atlas_prompt_both", "atlas_prompt_missing"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	// v0.16.0: three new rules and the alignments with the reference.
	{file: "invalid-reserved-input-query.yaml", valid: false, wantErrorCodes: []string{"reserved_input_name"}},
	{file: "invalid-reserved-input-schema.yaml", valid: false, wantErrorCodes: []string{"reserved_input_name"}},
	{file: "invalid-atlas-model-not-literal.yaml", valid: false, wantErrorCodes: []string{"atlas_model_not_literal"}},
	{file: "invalid-atlas-param-refused.yaml", valid: false, wantErrorCodes: []string{"atlas_param_refused"}},
	{file: "invalid-atlas-param-refused-capability.yaml", valid: false, wantErrorCodes: []string{"atlas_param_refused"}},
	{file: "invalid-atlas-agent-not-a-string.yaml", valid: false, wantErrorCodes: []string{"atlas_agent_missing"}},
	{file: "invalid-atlas-prompt-blank.yaml", valid: false, wantErrorCodes: []string{"atlas_prompt_missing"}},
	{file: "invalid-atlas-capabilities-nested-template.yaml", valid: false, wantErrorCodes: []string{"atlas_capabilities_not_literal"}},
	{file: "invalid-atlas-capability-ref-dot.yaml", valid: false, wantErrorCodes: []string{"atlas_capability_ref_invalid"}},
	{file: "valid-reserved-input-names-ok.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_model_not_literal", "atlas_operation_unknown", "atlas_param_refused", "atlas_prompt_both", "atlas_prompt_missing", "reserved_input_name"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-model-literal.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_model_not_literal", "atlas_operation_unknown", "atlas_param_refused", "atlas_prompt_both", "atlas_prompt_missing", "reserved_input_name"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-capability-url-arg.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_model_not_literal", "atlas_operation_unknown", "atlas_param_refused", "atlas_prompt_both", "atlas_prompt_missing", "reserved_input_name"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "valid-atlas-prompt-empty-with-template.yaml", valid: true, forbidErrorCodes: []string{"atlas_agent_missing", "atlas_agent_not_literal", "atlas_capabilities_not_literal", "atlas_capability_ref_invalid", "atlas_model_not_literal", "atlas_operation_unknown", "atlas_param_refused", "atlas_prompt_both", "atlas_prompt_missing", "reserved_input_name"}, forbidWarningCodes: []string{codeUnknownExecScheme}},
	{file: "warn-compliance-residency-unenforced.yaml", valid: true, wantWarningCodes: []string{codeComplianceResidencyUnenforced}},
	{file: "warn-compliance-residency-unenforced-loop.yaml", valid: true, wantWarningCodes: []string{codeComplianceResidencyUnenforced}},
	{file: "valid-compliance-residency-pinned.yaml", valid: true, forbidWarningCodes: []string{codeComplianceResidencyUnenforced}},
	{file: "valid-compliance-residency-silent-providers.yaml", valid: true, forbidWarningCodes: []string{codeComplianceResidencyUnenforced}},
	{file: "warn-credential-endpoint-ai-step-query.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-ai-loop-sub-step.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-ai-executor-config.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-family-step-query.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-family-executor-config.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{
		// v0.8.0 (AIgentFlow v2.819.0): the AigentChat executor is gone, so
		// aigentchat:// is any unknown scheme: a warning, and no family judgement.
		// (nexus:// is the agent runtime executor again since v2.835.0.)
		file: "warn-removed-executor-aigentchat.yaml", valid: true,
		wantWarningCodes:   []string{codeUnknownExecScheme},
		forbidWarningCodes: []string{codeCredentialEndpointUnpaired},
	},
	{
		// The author's own key beside the author's own URL, on every arm.
		file: "valid-credential-endpoint-own-key-own-url.yaml", valid: true,
		forbidWarningCodes: []string{codeCredentialEndpointUnpaired},
	},
	{
		// A server key beside the default endpoint (by origin), a deployment
		// variable, a template, or no endpoint.
		file: "valid-credential-endpoint-server-endpoints.yaml", valid: true,
		forbidWarningCodes: []string{codeCredentialEndpointUnpaired},
	},
	{
		// ollama and vllm are exempt.
		file: "valid-credential-endpoint-keyless-providers.yaml", valid: true,
		forbidWarningCodes: []string{codeCredentialEndpointUnpaired},
	},
	{
		// Implicit-credential and non-expanding families, no credential, another
		// family's variable, a non-string endpoint, a non-ai step, a non-provider
		// executor_config key.
		file: "valid-credential-endpoint-not-judged.yaml", valid: true,
		forbidWarningCodes: []string{codeCredentialEndpointUnpaired},
	},
	// v0.6.3 (AIgentFlow #187): the optional top-level display_name, at most 80
	// characters counted in runes. The multibyte pair straddles the boundary
	// where a byte count would disagree with a rune count.
	{file: "valid-display-name.yaml", valid: true, forbidErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "valid-display-name-80-code-points.yaml", valid: true, forbidErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "invalid-display-name-81.yaml", valid: false, wantErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "valid-untrusted-inputs.yaml", valid: true, forbidErrorCodes: []string{codeUnknownYAMLKey}},
	{file: "invalid-untrusted-misspelt.yaml", valid: false, wantErrorCodes: []string{codeUnknownYAMLKey}},
	{file: "valid-display-name-80-runes.yaml", valid: true, forbidErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "valid-display-name-80-runes-multibyte.yaml", valid: true, forbidErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "invalid-display-name-81-runes.yaml", valid: false, wantErrorCodes: []string{codeDisplayNameTooLong}},
	{file: "invalid-display-name-81-runes-multibyte.yaml", valid: false, wantErrorCodes: []string{codeDisplayNameTooLong}},
	// v0.18.0 — AIgentFlow v2.835.0: nexus:// is the canonical spelling of the
	// agent runtime executor and atlas:// its alias; one set of rules for both.
	{
		file: "valid-nexus-agent-run.yaml", valid: true,
		forbidWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		file: "valid-nexus-capability.yaml", valid: true,
		forbidWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		file: "valid-atlas-alias-agent-run.yaml", valid: true,
		forbidWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		file: "invalid-nexus-agent-missing.yaml", valid: false,
		wantErrorCodes:     []string{"atlas_agent_missing"},
		forbidWarningCodes: []string{codeUnknownExecScheme},
	},
	{
		file: "invalid-nexus-loop-substep-agent-not-literal.yaml", valid: false,
		wantErrorCodes: []string{"atlas_agent_not_literal"},
	},
	{
		// The pre-2026-10-20 AigentChat shape names no Nexus operation.
		file: "invalid-nexus-aigentchat-shape.yaml", valid: false,
		wantErrorCodes: []string{"atlas_operation_unknown"},
	},
	// v0.18.0 — AIgentFlow v2.834.0 (AGENTCUT B1): signals and DFM removed.
	{
		file: "invalid-signal-removed-aiv-step.yaml", valid: false,
		wantErrorCodes: []string{codeSignalRemoved},
	},
	{
		file: "invalid-flow-modification-removed-step.yaml", valid: false,
		wantErrorCodes: []string{codeFlowModificationRemoved},
	},
	{
		file: "warn-removed-key-ignored-pre-dfm.yaml", valid: true,
		wantWarningCodes:   []string{codeRemovedKeyIgnored},
		forbidWarningCodes: []string{codeUnknownYAMLKey},
	},
	{
		// The no-false-positive control.
		file: "valid-signal-surfaces-not-refused.yaml", valid: true,
		forbidErrorCodes:   []string{codeSignalRemoved, codeFlowModificationRemoved},
		forbidWarningCodes: []string{codeRemovedKeyIgnored},
	},
	// v0.18.0 — AIgentFlow v2.836.0 (AGENTCUT B2): campaigns removed.
	{
		file: "invalid-campaign-removed-block.yaml", valid: false,
		wantErrorCodes:   []string{codeCampaignRemoved},
		forbidErrorCodes: []string{codeInvalidType, codeStepNotFound, codeOrchestratorRemoved},
	},
	{
		file: "invalid-campaign-removed-empty-block.yaml", valid: false,
		wantErrorCodes: []string{codeCampaignRemoved},
	},
	{
		file: "invalid-campaign-removed-retired-key.yaml", valid: false,
		wantErrorCodes:   []string{codeCampaignRemoved},
		forbidErrorCodes: []string{codeUnknownYAMLKey},
	},
	{
		// The no-false-positive control.
		file: "valid-campaign-surfaces-not-refused.yaml", valid: true,
		forbidErrorCodes: []string{codeCampaignRemoved},
	},
	// v0.20.0 — AIgentFlow v2.841.0: for_each.items shape. A string with no
	// template that is not a JSON array can never resolve to a list.
	{
		file: "invalid-for-each-items-bare-path.yaml", valid: false,
		wantErrorCodes: []string{codeForEachItemsUnresolvable},
	},
	{
		file: "invalid-for-each-items-null-literal.yaml", valid: false,
		wantErrorCodes: []string{codeForEachItemsUnresolvable},
	},
	{
		file: "invalid-for-each-items-object-literal.yaml", valid: false,
		wantErrorCodes: []string{codeForEachItemsUnresolvable},
	},
	{
		file: "invalid-for-each-items-plain-text.yaml", valid: false,
		wantErrorCodes: []string{codeForEachItemsUnresolvable},
	},
	{
		// The no-false-positive control: a template, a fromJson template and
		// JSON array literals ([] included) are all fine.
		file: "valid-for-each-items-forms.yaml", valid: true,
		forbidErrorCodes: []string{codeForEachItemsUnresolvable, codeForEachItemsRequired},
	},
	// v0.19.0 — AIgentFlow v2.838.0: the orchestrator removed. The block is
	// refused whole and nothing inside it is judged.
	{
		file: "invalid-orchestrator-removed-block.yaml", valid: false,
		wantErrorCodes: []string{codeOrchestratorRemoved},
	},
	{
		file: "invalid-orchestrator-removed-empty-block.yaml", valid: false,
		wantErrorCodes: []string{codeOrchestratorRemoved},
	},
	{
		file: "invalid-orchestrator-removed-scalar.yaml", valid: false,
		wantErrorCodes:   []string{codeOrchestratorRemoved},
		forbidErrorCodes: []string{codeInvalidType},
	},
	{
		file: "invalid-orchestrator-removed-inner-surfaces.yaml", valid: false,
		wantErrorCodes: []string{codeOrchestratorRemoved},
		forbidErrorCodes: append([]string{codeSignalRemoved, codeCampaignRemoved, codeFlowModificationRemoved,
			codeUnknownYAMLKey, codeInvalidType, codeInvalidDuration}, retiredOrchestratorRuleCodes...),
		forbidWarningCodes: append([]string{codeRemovedKeyIgnored, codeUnknownYAMLKey}, retiredOrchestratorRuleCodes...),
	},
	{
		file: "invalid-orchestrator-removed-enable-signals-false.yaml", valid: false,
		wantErrorCodes:     []string{codeOrchestratorRemoved},
		forbidErrorCodes:   []string{codeSignalRemoved},
		forbidWarningCodes: []string{codeRemovedKeyIgnored},
	},
	{
		file: "invalid-orchestrator-removed-tool-discovery.yaml", valid: false,
		wantErrorCodes:   []string{codeOrchestratorRemoved},
		forbidErrorCodes: []string{"tool_discovery_invalid"},
	},
	{
		file: "invalid-orchestrator-removed-discoverable-tools.yaml", valid: false,
		wantErrorCodes: []string{codeOrchestratorRemoved},
	},
	{
		file: "invalid-orchestrator-removed-next-default.yaml", valid: false,
		wantErrorCodes:   []string{codeOrchestratorRemoved},
		forbidErrorCodes: []string{codeStepNotFound, "orchestrator_next_requires_orchestrator"},
	},
	{
		file: "invalid-orchestrator-removed-next-with-block.yaml", valid: false,
		wantErrorCodes:   []string{codeOrchestratorRemoved},
		forbidErrorCodes: []string{codeStepNotFound, "orchestrator_owner_needs_yield"},
	},
	{
		file: "invalid-orchestrator-removed-next-condition.yaml", valid: false,
		wantErrorCodes:   []string{codeOrchestratorRemoved},
		forbidErrorCodes: []string{codeStepNotFound},
	},
	{
		// The no-false-positive control.
		file: "valid-orchestrator-removed-not-refused.yaml", valid: true,
		forbidErrorCodes:   []string{codeOrchestratorRemoved, codeAgenticParamRemoved, "tool_discovery_invalid"},
		forbidWarningCodes: []string{codeUnknownYAMLKey},
	},
	{
		// On ai:// the step keys are agent-loop parameters, never the flow key.
		file: "invalid-agentic-param-tool-discovery-only.yaml", valid: false,
		wantErrorCodes:   []string{codeAgenticParamRemoved},
		forbidErrorCodes: []string{codeOrchestratorRemoved, "tool_discovery_invalid"},
	},
}

// retiredOrchestratorRuleCodes are the codes of the orchestrator rules that
// left with the orchestrator (AIgentFlow v2.838.0). Named as literals because
// the rules, and their constants, are gone: a fixture that forbids them goes
// red if any of them ever judges a refused block again.
var retiredOrchestratorRuleCodes = []string{
	"orchestrator_exons_required", "orchestrator_mode_invalid", "orchestrator_owner_needs_yield",
	"orchestrator_trigger_unknown", "orchestrator_timer_no_interval", "orchestrator_timer_bad_interval",
	"orchestrator_tool_unknown", "orchestrator_not_agentic", "orchestrator_human_question_timeout_invalid",
	"orchestrator_enforce_region_needs_region", "orchestrator_exons_parse_failed",
	"orchestrator_exons_no_provider", "exons_resources_unhonoured", "exons_attributes",
	"orchestrator_tool_withheld", "tool_discovery_invalid", "orchestrator_next_requires_orchestrator",
}

func TestConformance(t *testing.T) {
	for _, tc := range conformanceCases {
		t.Run(tc.file, func(t *testing.T) {
			source := readFixture(t, tc.file)
			result := ValidateFlow(source, Options{})

			if tc.exonsEngineOnly {
				// The built-in reader cannot judge this document (divergence
				// #16): it must accept the flow and report none of the codes
				// only an engine can produce.
				if !result.Valid {
					t.Errorf("built-in reader: valid = false, want true (engine-only refusal)\nerrors: %s",
						formatIssues(result.Errors))
				}
				assertCodesAbsent(t, "error", result.Errors, tc.wantErrorCodes)
				return
			}
			if result.Valid != tc.valid {
				t.Errorf("valid = %v, want %v\nerrors: %s\nwarnings: %s",
					result.Valid, tc.valid, formatIssues(result.Errors), formatIssues(result.Warnings))
			}
			assertCodesPresent(t, "error", result.Errors, tc.wantErrorCodes)
			assertCodesPresent(t, "warning", result.Warnings, tc.wantWarningCodes)
			assertCodesAbsent(t, "error", result.Errors, tc.forbidErrorCodes)
			assertCodesAbsent(t, "warning", result.Warnings, tc.forbidWarningCodes)

			// Valid ⇔ no errors is the definition, not an incidental property; assert
			// it on every fixture so a validator that reports an error without
			// lowering the verdict cannot slip through.
			if got := len(result.Errors) == 0; got != result.Valid {
				t.Errorf("Valid (%v) disagrees with len(Errors)==0 (%v)", result.Valid, got)
			}
			if result.Summary.ErrorCount != len(result.Errors) {
				t.Errorf("Summary.ErrorCount = %d, want %d", result.Summary.ErrorCount, len(result.Errors))
			}
			if result.Summary.WarningCount != len(result.Warnings) {
				t.Errorf("Summary.WarningCount = %d, want %d", result.Summary.WarningCount, len(result.Warnings))
			}
			if result.SpecVersion != SpecVersion() {
				t.Errorf("SpecVersion = %q, want %q", result.SpecVersion, SpecVersion())
			}
		})
	}
}

// TestConformanceFixturesAreAllCovered fails when a fixture exists with no case
// in the table. Without it, copying a new fixture from the JS repo would silently
// add an unvalidated file — the table, not the directory, would be the real
// coverage, and nothing would say so.
func TestConformanceFixturesAreAllCovered(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	covered := make(map[string]struct{}, len(conformanceCases))
	for _, tc := range conformanceCases {
		covered[tc.file] = struct{}{}
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		if _, ok := covered[entry.Name()]; !ok {
			t.Errorf("fixture %s has no case in conformanceCases (add one here AND upstream)", entry.Name())
		}
	}
	if len(entries) == 0 {
		t.Fatal("no conformance fixtures found — the parity net is not actually running")
	}
}

const fixtureDir = "testdata/conformance"

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}
