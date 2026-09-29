package aifvalidate

import (
	"slices"
	"testing"
)

// v0.6.1: server_owned_query_key (AIgentFlow CFX-05). The conformance fixtures
// pin the verdict per key and per surface; these pin what a verdict cannot
// show — the exact field path, the step a finding names, one finding per key.

func serverOwnedFindings(result Result) []Issue {
	var out []Issue
	for _, e := range result.Errors {
		if e.Code == codeServerOwnedQueryKey {
			out = append(out, e)
		}
	}
	return out
}

func TestServerOwnedQueryKeySetIsTheSpecData(t *testing.T) {
	want := []string{"aiv_api_key", "aiv_base_url", "aiv_delegation"}
	if got := sortedSet(serverOwnedQueryKeys); !slices.Equal(got, want) {
		t.Fatalf("server-owned keys = %v, want %v (a new key is a spec change)", got, want)
	}
}

func TestServerOwnedQueryKeyFields(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		wantField []string
		wantStep  string
	}{
		{
			name:      "every key on a step, one finding each, in key order",
			yaml:      minimalFlow + "    query:\n      aiv_delegation: {user_id: x}\n      aiv_base_url: https://h.example\n      aiv_api_key: k\n      aiv_ref: fine\n",
			wantField: []string{"steps.only.query.aiv_api_key", "steps.only.query.aiv_base_url", "steps.only.query.aiv_delegation"},
			wantStep:  "only",
		},
		{
			name:      "a null value is still a declared key",
			yaml:      minimalFlow + "    query:\n      aiv_base_url:\n",
			wantField: []string{"steps.only.query.aiv_base_url"},
			wantStep:  "only",
		},
		{
			name:      "a loop sub-step is addressed by its id and names the loop step",
			yaml:      loopFlow("        - id: fetch\n          executor: function://text/noop\n          query:\n            aiv_api_key: k\n"),
			wantField: []string{"steps.only.loop.steps.fetch.query.aiv_api_key"},
			wantStep:  "only",
		},
		{
			name:      "a numeric sub-step id is its source text",
			yaml:      loopFlow("        - id: 07\n          executor: function://text/noop\n          query:\n            aiv_delegation: x\n"),
			wantField: []string{"steps.only.loop.steps.07.query.aiv_delegation"},
			wantStep:  "only",
		},
		{
			name:      "a sub-step without an id is addressed by the empty string (the reference refuses it at parse)",
			yaml:      loopFlow("        - executor: function://text/noop\n          query:\n            aiv_base_url: https://h.example\n"),
			wantField: []string{"steps.only.loop.steps..query.aiv_base_url"},
			wantStep:  "only",
		},
		{
			name: "another case, a nested key and a value are not the key",
			yaml: minimalFlow + "    query:\n      AIV_API_KEY: a\n      Aiv_Base_Url: b\n      opts: {aiv_delegation: c}\n      ref: aiv_api_key\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := serverOwnedFindings(ValidateFlow(tc.yaml, Options{}))
			var fields []string
			for _, f := range findings {
				fields = append(fields, f.Field)
				if f.StepID != tc.wantStep {
					t.Errorf("finding %q names step %q, want %q", f.Field, f.StepID, tc.wantStep)
				}
			}
			if !slices.Equal(fields, tc.wantField) {
				t.Errorf("fields = %v, want %v", fields, tc.wantField)
			}
		})
	}
}

func loopFlow(subSteps string) string {
	return `aigentflow_version: "2.0.0"
name: test-flow
version: 1.0.0
start: only
steps:
  only:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
` + subSteps
}
