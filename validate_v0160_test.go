package aifvalidate

import (
	"reflect"
	"testing"
)

// v0.16.0: reserved_input_name, atlas_model_not_literal, atlas_param_refused,
// and the alignments of the atlas rules with the reference.

func inputProbe(top string) string {
	return `aigentflow_version: '2.0.0'
name: input-probe
version: '1.0.0'
description: probe
` + top + `start: s
steps:
  s:
    executor: function://utils/log
    next:
      default: 'null'
`
}

func TestReservedInputNameFieldsAndOrder(t *testing.T) {
	src := inputProbe(`query:
  zeta:
    type: string
  __b:
    type: string
  __a:
    type: string
input_schema:
  version: 1
  fields:
    - name: ok
      type: string
      label: Ok
    - name: __c
      type: string
      label: C
`)
	r := ValidateFlow(src, Options{})
	var fields []string
	for _, e := range r.Errors {
		if e.Code == "reserved_input_name" {
			fields = append(fields, e.Field)
			if e.StepID != "" {
				t.Errorf("step id = %q", e.StepID)
			}
		}
	}
	want := []string{"query.__a", "query.__b", "input_schema.fields.__c"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
}

func TestReservedInputNameLeavesSingleUnderscoreAlone(t *testing.T) {
	r := ValidateFlow(inputProbe("query:\n  _x:\n    type: string\n  topic:\n    type: string\n"), Options{})
	if hasCode(r.Errors, "reserved_input_name") {
		t.Fatalf("got %s", formatIssues(r.Errors))
	}
}

func TestAtlasModelNotLiteral(t *testing.T) {
	q := "      agent: '@a/b'\n      prompt: hi\n      model:\n        reasoning_effort: '{{ .query.e }}'\n"
	if got := errorCodes(ValidateFlow(atlasProbe("atlas://agent/run", q), Options{})); !reflect.DeepEqual(got, []string{"atlas_model_not_literal"}) {
		t.Fatalf("codes = %v", got)
	}
}

func TestAtlasParamRefusedOneFindingSorted(t *testing.T) {
	q := "      agent: '@a/b'\n      prompt: hi\n      url: x\n      attribution: y\n      api_key: z\n"
	r := ValidateFlow(atlasProbe("atlas://agent/run", q), Options{})
	got, ok := findByCode(r.Errors, "atlas_param_refused")
	if !ok || len(errorCodes(r)) != 1 {
		t.Fatalf("got %s", formatIssues(r.Errors))
	}
	want := `step "s" sets api_key, attribution, url under query. Atlas decides these itself, so a step may not set them.`
	if got.Message != want || got.Field != "steps.s" {
		t.Errorf("message = %q field = %q", got.Message, got.Field)
	}
}

func TestAtlasParamRefusedCapabilitySetIsSmaller(t *testing.T) {
	ok := ValidateFlow(atlasProbe("atlas://capability/web_fetch", "      url: x\n      host: y\n"), Options{})
	if len(ok.Errors) != 0 {
		t.Fatalf("got %s", formatIssues(ok.Errors))
	}
	r := ValidateFlow(atlasProbe("atlas://capability/web_fetch", "      untrusted_input_override: true\n      attribution: a\n"), Options{})
	if got, _ := findByCode(r.Errors, "atlas_param_refused"); got.Message != `step "s" sets attribution, untrusted_input_override under query. Atlas decides these itself, so a step may not set them.` {
		t.Errorf("message = %q", got.Message)
	}
}

func TestAtlasPresenceIsTrimmedAndTyped(t *testing.T) {
	cases := map[string][]string{
		"      agent: 42\n      prompt: hi\n":                                  {"atlas_agent_missing"},
		"      agent: '  '\n      prompt: hi\n":                                {"atlas_agent_missing"},
		"      agent: '@a/b'\n      prompt: '  '\n":                            {"atlas_prompt_missing"},
		"      agent: '@a/b'\n      prompt: ''\n      prompt_template: x\n":    nil,
		"      agent: '@a/b'\n      prompt: ' '\n      prompt_template: ' '\n": {"atlas_prompt_missing"},
	}
	for q, want := range cases {
		if got := errorCodes(ValidateFlow(atlasProbe("atlas://agent/run", q), Options{})); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: codes = %v, want %v", q, got, want)
		}
	}
}

func TestAtlasCapabilitiesTemplateIsDeepAndAppliesToCapabilitySteps(t *testing.T) {
	q := "      capabilities:\n        - a: '{{ .x }}'\n"
	for _, ex := range []string{"atlas://capability/calc"} {
		if got := errorCodes(ValidateFlow(atlasProbe(ex, q), Options{})); !reflect.DeepEqual(got, []string{"atlas_capabilities_not_literal"}) {
			t.Errorf("%s: codes = %v", ex, got)
		}
	}
}

func TestAtlasCapabilityRefCharsetHasNoDotOrColon(t *testing.T) {
	for _, ref := range []string{"a.b", "a:b"} {
		r := ValidateFlow(atlasProbe("atlas://capability/"+ref, ""), Options{})
		if !hasCode(r.Errors, "atlas_capability_ref_invalid") {
			t.Errorf("%s: got %s", ref, formatIssues(r.Errors))
		}
	}
}

func TestAtlasUnparsableURLIsJudgedByTheAtlasRules(t *testing.T) {
	r := ValidateFlow(atlasProbe("atlas://agent/run/extra", "      agent: '@a/b'\n      prompt: hi\n"), Options{})
	if !hasCode(r.Errors, "atlas_operation_unknown") {
		t.Fatalf("got %s", formatIssues(r.Errors))
	}
	if r2 := ValidateFlow(atlasProbe("ATLAS://agent/run", ""), Options{}); hasCode(r2.Errors, "atlas_agent_missing") {
		t.Errorf("scheme match must stay exact")
	}
}
