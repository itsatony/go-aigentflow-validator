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
		// DC-COND-1: a monitor-mode orchestrator over a self-terminating DAG is valid.
		file: "valid-orchestrator-monitor.yaml", valid: true,
	},
	{
		// v2.695.0 (DC-FORGE-125): a NEGATIVE duration is well-formed Go and still refused.
		file: "invalid-orchestrator-human-question-timeout.yaml", valid: false,
		wantErrorCodes: []string{codeOrchHumanQTimeoutInvalid},
	},
	{
		file: "valid-orchestrator-human-question-timeout.yaml", valid: true,
	},
	{
		// DC-COND-2: campaign.on_children_complete naming a real step.
		file: "valid-campaign-handoff.yaml", valid: true,
	},
	{
		file:           "invalid-campaign-handoff-unknown-step.yaml",
		valid:          false,
		wantErrorCodes: []string{codeCampaignHandoffStep},
	},
	{
		file:           "invalid-orchestrator-owner-no-yield.yaml",
		valid:          false,
		wantErrorCodes: []string{codeOrchOwnerNeedsYield},
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
		// v2.728.0 (DC-FORGE-155): campaign.budget_max_per_child was deleted.
		file: "invalid-retired-campaign-budget-max-per-child.yaml", valid: false,
		wantErrorCodes: []string{codeUnknownYAMLKey},
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
		// All three surfaces: the flow root, the orchestrator and a step query.
		file: "invalid-tool-discovery-vocabulary.yaml", valid: false,
		wantErrorCodes: []string{codeToolDiscoveryInvalid},
	},
	{
		file: "valid-tool-discovery-vocabulary.yaml", valid: true,
		forbidErrorCodes: []string{codeToolDiscoveryInvalid},
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
		// A list of nothing but nulls is empty after the reference decodes it.
		file: "invalid-campaign-no-child-flows.yaml", valid: false,
		wantErrorCodes: []string{codeCampaignNoChildFlows},
	},
	{
		file: "invalid-campaign-child-flow-no-id.yaml", valid: false,
		wantErrorCodes: []string{codeCampaignChildFlowNoID},
	},
	{
		// max_credits_per_child 1.5 truncates and saves; a null child entry is
		// dropped; flow_id is a Go string, so a number counts.
		file: "valid-campaign-decoded-shapes.yaml", valid: true,
		forbidErrorCodes: []string{codeInvalidType, codeCampaignMaxCreditsChild,
			codeCampaignNoChildFlows, codeCampaignChildFlowNoID},
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
		// 0, +0, -0 save — and a timer interval of 0.
		file: "valid-duration-numeric-zero.yaml", valid: true,
		forbidErrorCodes: []string{codeMockDelayInvalid, codeInvalidDuration,
			codeOrchTimerNoInterval, codeOrchTimerBadInterv},
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
	{
		// v2.777.0 (DC-FORGE-214): tags the engine's resolver refuses.
		file: "invalid-exons-step-attributes.yaml", valid: false,
		wantErrorCodes:  []string{codeExonsAttributes},
		exonsEngineOnly: true,
	},
	{
		// Templated, non-exons executor, non-string: none is judged.
		file: "valid-exons-step-attributes-skipped.yaml", valid: true,
		forbidErrorCodes: []string{codeExonsAttributes},
	},
	{
		file: "invalid-orchestrator-exons-no-spec.yaml", valid: false,
		wantErrorCodes: []string{codeOrchExonsParseFailed},
	},
	{
		file: "invalid-orchestrator-exons-unclosed-frontmatter.yaml", valid: false,
		wantErrorCodes: []string{codeOrchExonsParseFailed},
	},
	{
		file: "invalid-orchestrator-exons-no-provider.yaml", valid: false,
		wantErrorCodes: []string{codeOrchExonsNoProvider},
	},
	{
		// v2.767.0 (DC-FORGE-205): every declared resource is refused.
		file: "invalid-orchestrator-exons-resources.yaml", valid: false,
		wantErrorCodes: []string{codeExonsResourcesRefused},
	},
	{
		file: "valid-orchestrator-exons-resources-empty.yaml", valid: true,
		forbidErrorCodes: []string{codeExonsResourcesRefused},
	},
	{
		file: "invalid-orchestrator-exons-attributes.yaml", valid: false,
		wantErrorCodes:  []string{codeExonsAttributes},
		exonsEngineOnly: true,
	},
	{
		// The engine's parse validates the decoded spec.
		file: "invalid-orchestrator-exons-spec-invalid.yaml", valid: false,
		wantErrorCodes:  []string{codeOrchExonsParseFailed},
		exonsEngineOnly: true,
	},
	{
		// v2.760.0 (DC-FORGE-190): the four withholds, each its own fixture.
		file: "warn-orchestrator-tool-withheld-ask-human.yaml", valid: true,
		wantWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		file: "warn-orchestrator-tool-withheld-named.yaml", valid: true,
		wantWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		file: "warn-orchestrator-tool-withheld-signals-off.yaml", valid: true,
		wantWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		file: "warn-orchestrator-tool-withheld-campaign.yaml", valid: true,
		wantWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		// The counter-fixtures: a false positive here goes red.
		file: "valid-orchestrator-tool-allow-consistent.yaml", valid: true,
		forbidWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		file: "valid-orchestrator-tool-allow-campaign.yaml", valid: true,
		forbidWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		file: "valid-orchestrator-tool-allow-absent.yaml", valid: true,
		forbidWarningCodes: []string{codeOrchToolWithheld},
	},
	{
		// v2.760.0 (DC-FORGE-189): `end` is a step, so this is a cycle.
		file: "warn-next-end-cycle.yaml", valid: true,
		wantWarningCodes:   []string{codePotentialInfiniteLop},
		forbidWarningCodes: []string{codeUnreachableStep},
	},
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
	{file: "warn-credential-endpoint-ai-step-query.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-ai-loop-sub-step.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-ai-executor-config.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-family-step-query.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-family-executor-config.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
	{file: "warn-credential-endpoint-nexus.yaml", valid: true, wantWarningCodes: []string{codeCredentialEndpointUnpaired}},
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
