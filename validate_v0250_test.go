package aifvalidate

import "testing"

// v0.25.0: atlas_skills_not_literal. On an agent run, query.skills picks which of
// the organization's templates the run starts with, so no entry may be computed
// from a template; on a capability step `skills` is an ordinary tool argument.

const (
	skillsAgentQuery = "      agent: '@a/b'\n      prompt: hi\n"
	skillsCode       = "atlas_skills_not_literal"
	skillsMessage    = "step \"s\": every `skills` entry must be a literal, not a template. They choose which of the organization's templates the person's agent run starts with, so they are never rendered from inputs or outside data"
	skillsSuggestion = "Write the agent's name literally; to run different agents, use different steps"
)

func skillsFindings(r Result) []Issue {
	var out []Issue
	for _, e := range r.Errors {
		if e.Code == skillsCode {
			out = append(out, e)
		}
	}
	return out
}

func TestAtlasSkillsNotLiteralFindingShape(t *testing.T) {
	r := ValidateFlow(atlasProbe("nexus://agent/run", skillsAgentQuery+"      skills:\n        - '@a/s'\n        - '{{ .query.skill }}'\n"), Options{})
	got := skillsFindings(r)
	if r.Valid || len(got) != 1 {
		t.Fatalf("valid=%v findings=%v", r.Valid, got)
	}
	e := got[0]
	if e.Field != "steps.s" || e.StepID != "s" || e.Message != skillsMessage || e.Suggestion != skillsSuggestion {
		t.Fatalf("finding = %+v", e)
	}
}

func TestAtlasSkillsNotLiteralDeepAndAlias(t *testing.T) {
	for name, tc := range map[string]struct{ executor, skills string }{
		"object ref":       {"nexus://agent/run", "      skills:\n        - ref: '{{ .query.r }}'\n"},
		"object version":   {"nexus://agent/run", "      skills:\n        - ref: '@a/t'\n          version: '{{ .query.v }}'\n"},
		"templated scalar": {"nexus://agent/run", "      skills: '{{ toJson .query.s }}'\n"},
		"atlas alias":      {"atlas://agent/run", "      skills: ['{{ .query.s }}']\n"},
	} {
		if got := skillsFindings(ValidateFlow(atlasProbe(tc.executor, skillsAgentQuery+tc.skills), Options{})); len(got) != 1 {
			t.Errorf("%s: findings = %v, want one", name, got)
		}
	}
}

func TestAtlasSkillsNotLiteralInALoopBodyIsAnError(t *testing.T) {
	src := `aigentflow_version: '2.0.0'
name: loop-probe
version: '1.0.0'
description: probe
start: spin
steps:
  spin:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
        - id: run
          executor: nexus://agent/run
          query:
            agent: '@a/b'
            prompt: hi
            skills: ['{{ .query.s }}']
    next:
      default: 'null'
`
	r := ValidateFlow(src, Options{})
	got := skillsFindings(r)
	if len(got) != 1 || got[0].Field != "steps.spin.loop.steps[0]" {
		t.Fatalf("findings = %+v", got)
	}
	if hasCode(r.Warnings, skillsCode) {
		t.Fatal("demoted to a warning in a loop body")
	}
}

func TestAtlasSkillsNotLiteralLookalikesPass(t *testing.T) {
	for name, tc := range map[string]struct{ executor, query string }{
		"literal skills":               {"nexus://agent/run", skillsAgentQuery + "      skills:\n        - '@a/s'\n        - ref: '@a/t'\n          version: '1.2.0'\n"},
		"templated files on agent run": {"nexus://agent/run", skillsAgentQuery + "      files: '{{ toJson .data.d.files }}'\n"},
		"capability skills is an arg":  {"nexus://capability/web_search", "      skills: '{{ .query.s }}'\n"},
		"capability files is an arg":   {"nexus://capability/web_search", "      files: '{{ .query.f }}'\n"},
	} {
		r := ValidateFlow(atlasProbe(tc.executor, tc.query), Options{})
		if hasCode(r.Errors, skillsCode) || hasCode(r.Warnings, skillsCode) {
			t.Errorf("%s: fired", name)
		}
	}
}
