package aifvalidate

import (
	"slices"
	"strings"
	"testing"
)

// The agent surfaces AIgentFlow removed: executor_removed and
// agentic_param_removed. The conformance fixtures carry the verdicts; these
// tests pin the exact semantics the fixtures' subset comparison cannot say.

func withBody(body string) string { return minimalFlow + body }

func TestExecutorRemoved(t *testing.T) {
	t.Run("an exons or agent step yields ONLY executor_removed", func(t *testing.T) {
		for _, executor := range []string{"exons://agent/execute", "exons://upload/slot", "agent://modify/flow", "exons:///bad"} {
			src := strings.Replace(minimalFlow, "function://utils/log", executor, 1)
			r := ValidateFlow(src, Options{})
			if got := codesOf(r.Errors); !slices.Equal(got, []string{codeExecutorRemoved}) {
				t.Errorf("%s: errors %v, want only executor_removed", executor, got)
			}
			if len(r.Warnings) != 0 {
				t.Errorf("%s: warnings %v", executor, codesOf(r.Warnings))
			}
			if issue, _ := findByField(r.Errors, "steps.only"); issue.StepID != "only" {
				t.Errorf("%s: finding not addressed to steps.only: %+v", executor, r.Errors)
			}
		}
	})
	t.Run("case and surrounding whitespace do not hide the scheme", func(t *testing.T) {
		for _, executor := range []string{"' EXONS://agent/execute '", "Agent://x/y"} {
			src := strings.Replace(minimalFlow, "function://utils/log", executor, 1)
			assertError(t, ValidateFlow(src, Options{}), codeExecutorRemoved, "steps.only")
		}
	})
	t.Run("a templated executor is not judged", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "function://utils/log", "'exons://{{ .query.x }}'", 1)
		assertCodeAbsent(t, ValidateFlow(src, Options{}), codeExecutorRemoved)
	})
	t.Run("a loop sub-step is addressed by its index", func(t *testing.T) {
		src := withBody(`  loopy:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
        - id: a
          executor: function://utils/log
        - id: b
          executor: agent://modify/flow
`)
		r := ValidateFlow(src, Options{})
		if got := fieldsWithCode(r.Errors, codeExecutorRemoved); !slices.Equal(got, []string{"steps.loopy.loop.steps[1]"}) {
			t.Errorf("fields %v", got)
		}
		if issue, _ := findByField(r.Errors, "steps.loopy.loop.steps[1]"); issue.StepID != "loopy.b" {
			t.Errorf("step id %q, want loopy.b", issue.StepID)
		}
	})
	t.Run("the exons_uploads key is refused at the key", func(t *testing.T) {
		r := ValidateFlow(withBody("exons_uploads:\n  allowed_providers: [anthropic]\n"), Options{})
		if got := fieldsWithCode(r.Errors, codeExecutorRemoved); !slices.Equal(got, []string{"exons_uploads"}) {
			t.Errorf("fields %v", got)
		}
		assertCodeAbsent(t, r, codeUnknownYAMLKey)
	})
	t.Run("the orchestrator's own exons document is untouched", func(t *testing.T) {
		src := withBody("orchestrator:\n  exons: |\n    ---\n    name: o\n    description: d\n    execution:\n      provider: anthropic\n    ---\n    hi\n")
		assertCodeAbsent(t, ValidateFlow(src, Options{}), codeExecutorRemoved)
	})
}

func TestAgenticParamRemoved(t *testing.T) {
	aiStep := func(query string) string {
		return strings.Replace(minimalFlow, "function://utils/log", "ai://openai/chat\n    query:\n"+query, 1)
	}
	t.Run("one finding per step, keys sorted", func(t *testing.T) {
		r := ValidateFlow(aiStep("      max_tool_turns: 3\n      agentic: true\n      guardrails: {}\n"), Options{})
		if got := fieldsWithCode(r.Errors, codeAgenticParamRemoved); !slices.Equal(got, []string{"steps.only"}) {
			t.Fatalf("fields %v", got)
		}
		issue, _ := findByField(r.Errors, "steps.only")
		if !strings.Contains(issue.Message, "agentic, guardrails, max_tool_turns") {
			t.Errorf("keys not sorted in message: %q", issue.Message)
		}
	})
	t.Run("every one of the thirteen keys is refused on presence, null and false included", func(t *testing.T) {
		keys := []string{"agentic", "max_tool_turns", "parallel_tool_calls", "mcp_server_url", "tool_auth_token",
			"tool_discovery", "discoverable_tools", "enable_signals", "deny_tools", "max_tool_calls",
			"require_confirmation", "guardrails", "timeout_seconds"}
		if len(keys) != 13 {
			t.Fatal("table is not the contract's 13 keys")
		}
		for _, key := range keys {
			for _, value := range []string{"true", "false", "null", "5"} {
				assertError(t, ValidateFlow(aiStep("      "+key+": "+value+"\n"), Options{}), codeAgenticParamRemoved, "steps.only")
			}
		}
	})
	t.Run("what stays legal on ai://", func(t *testing.T) {
		q := "      tools: []\n      tool_choice: auto\n      autofix: true\n      content_parts: []\n      stream: true\n      persist_conversation: true\n"
		assertCodeAbsent(t, ValidateFlow(aiStep(q), Options{}), codeAgenticParamRemoved)
	})
	t.Run("only the ai scheme is judged", func(t *testing.T) {
		for _, executor := range []string{"aiv://acme/helper", "atlas://agent/run", "http://example.com/x", "function://utils/log"} {
			src := strings.Replace(minimalFlow, "function://utils/log", executor+"\n    query:\n      agentic: true\n      timeout_seconds: 5\n", 1)
			assertCodeAbsent(t, ValidateFlow(src, Options{}), codeAgenticParamRemoved)
		}
		templated := strings.Replace(minimalFlow, "function://utils/log", "'{{ .query.e }}'\n    query:\n      agentic: true\n", 1)
		assertCodeAbsent(t, ValidateFlow(templated, Options{}), codeAgenticParamRemoved)
	})
	t.Run("flow-level discovery keys and the orchestrator block's own keys stay", func(t *testing.T) {
		src := withBody("tool_discovery: lazy\ndiscoverable_tools: [a]\n")
		assertCodeAbsent(t, ValidateFlow(src, Options{}), codeAgenticParamRemoved)
	})
	t.Run("a loop sub-step is addressed by its index", func(t *testing.T) {
		src := withBody(`  loopy:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
        - id: a
          executor: ai://openai/chat
          query:
            enable_signals: true
`)
		r := ValidateFlow(src, Options{})
		if got := fieldsWithCode(r.Errors, codeAgenticParamRemoved); !slices.Equal(got, []string{"steps.loopy.loop.steps[0]"}) {
			t.Errorf("fields %v", got)
		}
	})
}

func assertCodeAbsent(t *testing.T, r Result, code string) {
	t.Helper()
	for _, i := range append(slices.Clone(r.Errors), r.Warnings...) {
		if i.Code == code {
			t.Errorf("unexpected %s at %s: %s", code, i.Field, i.Message)
		}
	}
}
