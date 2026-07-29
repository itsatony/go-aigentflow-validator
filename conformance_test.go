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
}

var conformanceCases = []conformanceCase{
	{file: "valid-minimal.yaml", valid: true},
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
		// Skope retired in v2.435.0 → skope:// is now an unknown scheme: warns, does
		// not reject. This fixture is the guard against re-hardening scheme checks.
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
		file:           "invalid-orchestrator-owner-no-yield.yaml",
		valid:          false,
		wantErrorCodes: []string{codeOrchOwnerNeedsYield},
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
		file:           "invalid-references-and-templates.yaml",
		valid:          false,
		wantErrorCodes: []string{codeStepNotFound, codeTemplateSyntax},
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
