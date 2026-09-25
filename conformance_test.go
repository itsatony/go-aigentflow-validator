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
}

func TestConformance(t *testing.T) {
	for _, tc := range conformanceCases {
		t.Run(tc.file, func(t *testing.T) {
			source := readFixture(t, tc.file)
			result := ValidateFlow(source, Options{})

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
