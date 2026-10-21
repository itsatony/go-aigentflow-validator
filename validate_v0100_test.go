package aifvalidate

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// v0.10.0: compliance_residency_unenforced (AIgentFlow v2.821.0, #217). The
// rule is the reference's catalogue-free warning; its pseudocode is in
// validate.residency.go and PARITY.md.

// residencyFlow builds a one-step flow. residency is the YAML list body;
// stepQuery is extra query lines (6-space indented); tail is appended verbatim
// (flow-level blocks such as executor_config).
func residencyFlow(residency, executor, stepQuery, tail string) string {
	return fmt.Sprintf(`aigentflow_version: '2.0.0'
name: residency-probe
version: '1.0.0'
description: residency probe
start: a
compliance:
  residency: %s
steps:
  a:
    executor: %s
    query:
      model: m
      prompt: 'hi'
%s    next:
      default: 'null'
%s`, residency, executor, stepQuery, tail)
}

func residencyWarnings(t *testing.T, yamlText string) []Issue {
	t.Helper()
	result := ValidateFlow(yamlText, Options{})
	if !result.Valid {
		t.Fatalf("a residency warning must never change the verdict; errors=%v", result.Errors)
	}
	var out []Issue
	for _, w := range result.Warnings {
		if w.Code == codeComplianceResidencyUnenforced {
			if w.Severity != SeverityWarning {
				t.Errorf("severity = %q, want warning", w.Severity)
			}
			out = append(out, w)
		}
	}
	return out
}

func TestResidencyUnenforcedWarns(t *testing.T) {
	eu := "['EU']"
	cases := map[string]string{
		"mistral with nothing":              residencyFlow(eu, "ai://mistral/chat", "", ""),
		"openai decide with nothing":        residencyFlow(eu, "ai://openai/decide", "", ""),
		"empty region string":               residencyFlow(eu, "ai://mistral/chat", "      region: '  '\n", ""),
		"enforce_region false":              residencyFlow(eu, "ai://mistral/chat", "      enforce_region: false\n", ""),
		"enforce_region other string":       residencyFlow(eu, "ai://mistral/chat", "      enforce_region: 'TRUE'\n", ""),
		"provider name is case-insensitive": residencyFlow(eu, "ai://Mistral/chat", "", ""),
		"member states, mixed case":         residencyFlow("['DE', ' eu ']", "ai://mistral/chat", "", ""),
		"unrelated base_url does not pin":   residencyFlow(eu, "ai://mistral/chat", "      base_url: 'https://x'\n", ""),
		"another provider's base_url":       residencyFlow(eu, "ai://mistral/chat", "      openai_base_url: 'https://x'\n", ""),
		"empty own base_url":                residencyFlow(eu, "ai://mistral/chat", "      mistral_base_url: ''\n", ""),
		"executor_config of another":        residencyFlow(eu, "ai://mistral/chat", "", "executor_config:\n  openai:\n    base_url: 'https://x'\n"),
		"every region-lane provider":        residencyFlow(eu, "ai://deepgram/transcribe", "", ""),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			warns := residencyWarnings(t, text)
			if len(warns) != 1 {
				t.Fatalf("want exactly 1 warning, got %d: %v", len(warns), warns)
			}
			if !strings.Contains(warns[0].Message, "not endpoint evidence") {
				t.Errorf("message = %q", warns[0].Message)
			}
		})
	}
}

func TestResidencyUnenforcedSilent(t *testing.T) {
	eu := "['EU']"
	cases := map[string]string{
		"region set":                    residencyFlow(eu, "ai://mistral/chat", "      region: eu\n", ""),
		"region templated is stated":    residencyFlow(eu, "ai://mistral/chat", "      region: '{{ .query.r }}'\n", ""),
		"enforce_region alone":          residencyFlow(eu, "ai://mistral/chat", "      enforce_region: true\n", ""),
		"enforce_region string true":    residencyFlow(eu, "ai://mistral/chat", "      enforce_region: 'true'\n", ""),
		"enforce_region string 1":       residencyFlow(eu, "ai://mistral/chat", "      enforce_region: '1'\n", ""),
		"enforce_region string yes":     residencyFlow(eu, "ai://mistral/chat", "      enforce_region: 'yes'\n", ""),
		"both":                          residencyFlow(eu, "ai://mistral/chat", "      region: eu\n      enforce_region: true\n", ""),
		"author-directed step endpoint": residencyFlow(eu, "ai://mistral/chat", "      mistral_base_url: 'https://api.eu.mistral.ai'\n", ""),
		"author-directed flow endpoint": residencyFlow(eu, "ai://mistral/chat", "", "executor_config:\n  mistral:\n    base_url: 'https://api.eu.mistral.ai'\n"),
		"no region lane scaleway":       residencyFlow(eu, "ai://scaleway/chat", "", ""),
		"no region lane vaiembed":       residencyFlow(eu, "ai://vaiembed/embed", "", ""),
		"no region lane anthropic":      residencyFlow(eu, "ai://anthropic/chat", "", ""),
		"templated provider":            residencyFlow(eu, "'ai://{{ .query.p }}/chat'", "", ""),
		"provider without an operation": residencyFlow(eu, "ai://mistral", "", ""),
		"exons step":                    residencyFlow(eu, "exons://agent/execute", "", ""),
		"aiv step":                      residencyFlow(eu, "aiv://org/agent", "", ""),
		"non-AI step":                   residencyFlow(eu, "http://example.com/x", "", ""),
		"mixed residency EU and US":     residencyFlow("['EU', 'US']", "ai://mistral/chat", "", ""),
		"US residency":                  residencyFlow("['US']", "ai://mistral/chat", "", ""),
		"empty residency":               residencyFlow("[]", "ai://mistral/chat", "", ""),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			result := ValidateFlow(text, Options{})
			for _, w := range result.Warnings {
				if w.Code == codeComplianceResidencyUnenforced {
					t.Fatalf("want silence, got %v", w)
				}
			}
		})
	}
	t.Run("no compliance block", func(t *testing.T) {
		text := strings.Replace(residencyFlow(eu, "ai://mistral/chat", "", ""), "compliance:\n  residency: ['EU']\n", "", 1)
		if len(residencyWarnings(t, text)) != 0 {
			t.Fatal("no compliance block must be silent")
		}
	})
}

func TestResidencyUnenforcedFieldsAndStepIDs(t *testing.T) {
	text := `aigentflow_version: '2.0.0'
name: residency-fields
version: '1.0.0'
description: residency fields
start: top
compliance:
  residency: ['EU', 'FR']
steps:
  top:
    executor: ai://openai/chat
    query:
      model: m
      prompt: 'hi'
    next:
      default: looper
  looper:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
        - id: inner
          executor: ai://mistral/chat
          query:
            model: m
            prompt: 'hi'
    next:
      default: 'null'
`
	warns := residencyWarnings(t, text)
	type got struct{ field, step string }
	var have []got
	for _, w := range warns {
		have = append(have, got{w.Field, w.StepID})
	}
	want := []got{
		// Step ids sorted ("looper" < "top"), a step's sub-steps right after it:
		// the reference's ExecutorSites order.
		{"steps.looper.loop.steps.inner.executor", "looper"},
		{"steps.top.executor", "top"},
	}
	if !slices.Equal(have, want) {
		t.Fatalf("findings = %v, want %v", have, want)
	}
	if !strings.Contains(warns[1].Message, `step "top"`) || !strings.Contains(warns[1].Message, "[EU FR]") {
		t.Errorf("message = %q", warns[1].Message)
	}
	if !strings.Contains(warns[0].Message, `loop sub-step "inner" of step "looper"`) {
		t.Errorf("message = %q", warns[0].Message)
	}
}

// The provider set is a hand copy of go-vaibstract v1.304.0's
// RuntimeRegionProviders, carried in the spec (this library imports no
// catalogue). Pinning it here makes a one-sided edit of the spec visible; the
// refresh recipe is in PARITY.md.
func TestResidencyRegionLaneSetIsPinned(t *testing.T) {
	want := []string{"assemblyai", "azure", "bedrock", "deepgram", "google", "mistral", "openai", "speechmatics"}
	got := slices.Clone(spec.ResidencyEnforcement.RegionLaneProviders)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("region-lane providers = %v, want %v (go-vaibstract RuntimeRegionProviders at v1.304.0)", got, want)
	}
	for _, p := range []string{"scaleway", "vaiembed", "anthropic"} {
		if slices.Contains(got, p) {
			t.Errorf("%s has no region lane and must not be in the set", p)
		}
	}
}
