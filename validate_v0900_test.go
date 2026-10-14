package aifvalidate

import "testing"

// v0.9.0: the orchestrator block gained `region` and `enforce_region`
// (AIgentFlow v2.820.0, #217). enforce_region without a named region is
// refused at save; everything else about the pair is accepted.

const orchestratorRegionHead = `aigentflow_version: '2.0.0'
name: orch-region
version: '1.0.0'
description: orchestrator region probe
start: work
steps:
  work:
    executor: function://text/template
    query:
      message: 'working'
    next:
      default: 'null'
orchestrator:
  mode: monitor
  agentic: true
`

const orchestratorRegionExons = `  exons: |
    ---
    name: monitor
    description: observe only
    type: agent
    execution:
      provider: anthropic
      model: claude-sonnet-4-6
    ---
    {~exons.message role="system"~}observe{~/exons.message~}
`

func orchestratorRegionFlow(extra string) string {
	return orchestratorRegionHead + extra + orchestratorRegionExons
}

func TestOrchestratorRegionKeysAreKnown(t *testing.T) {
	result := ValidateFlow(orchestratorRegionFlow("  region: eu\n  enforce_region: true\n"), Options{})
	if !result.Valid {
		t.Fatalf("region + enforce_region must be valid, errors=%v", result.Errors)
	}
	for _, w := range result.Warnings {
		if w.Code == codeUnknownYAMLKey {
			t.Errorf("region/enforce_region must be known orchestrator keys, got %v", w)
		}
	}
}

func TestOrchestratorEnforceRegionNeedsRegion(t *testing.T) {
	cases := []struct {
		name    string
		extra   string
		refused bool
	}{
		{"enforce without region", "  enforce_region: true\n", true},
		{"enforce with empty region", "  region: ''\n  enforce_region: true\n", true},
		{"enforce with whitespace region", "  region: '   '\n  enforce_region: true\n", true},
		{"enforce with null region", "  region: null\n  enforce_region: true\n", true},
		{"enforce with region", "  region: eu\n  enforce_region: true\n", false},
		{"enforce false without region", "  enforce_region: false\n", false},
		{"region alone is a pin", "  region: eu\n", false},
		{"neither", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := ValidateFlow(orchestratorRegionFlow(c.extra), Options{})
			var got *Issue
			for i := range result.Errors {
				if result.Errors[i].Code == codeOrchEnforceRegionNeeds {
					got = &result.Errors[i]
				}
			}
			if c.refused {
				if got == nil || result.Valid {
					t.Fatalf("want %s and an invalid verdict, got valid=%v errors=%v", codeOrchEnforceRegionNeeds, result.Valid, result.Errors)
				}
				if got.Field != "orchestrator.enforce_region" {
					t.Errorf("field = %q, want orchestrator.enforce_region", got.Field)
				}
				return
			}
			if got != nil || !result.Valid {
				t.Fatalf("must be accepted, valid=%v errors=%v", result.Valid, result.Errors)
			}
		})
	}
}
