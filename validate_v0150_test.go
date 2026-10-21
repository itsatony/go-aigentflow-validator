package aifvalidate

import (
	"reflect"
	"strings"
	"testing"
)

// v0.15.0: the atlas:// executor scheme and its rules.

func atlasProbe(executor, query string) string {
	src := `aigentflow_version: '2.0.0'
name: atlas-probe
version: '1.0.0'
description: atlas probe
start: s
steps:
  s:
    executor: ` + executor + "\n"
	if query != "" {
		src += "    query:\n" + query
	}
	return src + "    next:\n      default: 'null'\n"
}

func errorCodes(r Result) []string {
	var out []string
	for _, e := range r.Errors {
		out = append(out, e.Code)
	}
	return out
}

func TestAtlasIsAKnownScheme(t *testing.T) {
	r := ValidateFlow(atlasProbe("atlas://agent/run", "      agent: '@a/b'\n      prompt: hi\n"), Options{})
	if hasCode(r.Warnings, codeUnknownExecScheme) || !r.Valid {
		t.Fatalf("want valid and no scheme warning: %s", formatIssues(r.Errors))
	}
}

func TestAtlasFindingFieldAndMessage(t *testing.T) {
	r := ValidateFlow(atlasProbe("atlas://agent/run", "      prompt: hi\n"), Options{})
	got, ok := findByCode(r.Errors, "atlas_agent_missing")
	if !ok || r.Valid {
		t.Fatalf("want error, got %s", formatIssues(r.Errors))
	}
	if got.Field != "steps.s" || got.StepID != "s" {
		t.Errorf("field/step = %q / %q", got.Field, got.StepID)
	}
	if want := `step "s" runs a Nexus agent but query.agent is missing, empty or not a string.`; got.Message != want {
		t.Errorf("message = %q", got.Message)
	}
}

func TestAtlasEmptyAgentIsMissing(t *testing.T) {
	r := ValidateFlow(atlasProbe("atlas://agent/run", "      agent: ''\n      prompt: hi\n"), Options{})
	if !hasCode(r.Errors, "atlas_agent_missing") {
		t.Fatalf("got %s", formatIssues(r.Errors))
	}
}

func TestAtlasOperationUnknown(t *testing.T) {
	for _, ex := range []string{"atlas://bogus/x", "atlas://agent/start"} {
		r := ValidateFlow(atlasProbe(ex, "      agent: '@a/b'\n"), Options{})
		if got := errorCodes(r); !reflect.DeepEqual(got, []string{"atlas_operation_unknown"}) {
			t.Errorf("%s: codes = %v", ex, got)
		}
	}
}

func TestAtlasCapabilityRef(t *testing.T) {
	cases := map[string][]string{
		"atlas://capability/calc_evaluate":                nil,
		"atlas://capability/a1":                           nil,
		"atlas://capability/_x":                           {"atlas_capability_ref_invalid"},
		"atlas://capability/a/b":                          {"atlas_capability_ref_invalid"},
		"atlas://capability/a" + strings.Repeat("b", 128): {"atlas_capability_ref_invalid"},
		"atlas://capability/a" + strings.Repeat("b", 127): nil,
	}
	for ex, want := range cases {
		got := errorCodes(ValidateFlow(atlasProbe(ex, ""), Options{}))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%.40s: codes = %v, want %v", ex, got, want)
		}
	}
}

func TestAtlasTemplatedExecutorIsSkipped(t *testing.T) {
	r := ValidateFlow(atlasProbe(`'atlas://{{ .query.p }}/run'`, ""), Options{})
	if got := errorCodes(r); len(got) != 0 {
		t.Fatalf("codes = %v", got)
	}
}

func TestAtlasLoopSubStep(t *testing.T) {
	src := `aigentflow_version: '2.0.0'
name: t
version: '1.0.0'
description: t
start: s
steps:
  s:
    loop:
      while: '{{ lt (atoi .loop.vars.n) 3 }}'
      max_iterations: 3
      vars:
        n: '0'
      steps:
        - id: body
          executor: atlas://agent/run
          query:
            agent: '@a/b'
    next:
      default: 'null'
`
	r := ValidateFlow(src, Options{})
	got, ok := findByCode(r.Errors, "atlas_prompt_missing")
	if !ok {
		t.Fatalf("got %s", formatIssues(r.Errors))
	}
	if got.StepID != "s.body" || got.Field != "steps.s.loop.steps[0]" {
		t.Errorf("step/field = %q / %q", got.StepID, got.Field)
	}
}
