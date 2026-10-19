package aifvalidate

import (
	"sort"
	"strings"
	"testing"
)

// v0.13.0: unknown_function_operation, and the actionable unknown_executor_scheme message.

func executorProbe(executor string) string {
	return `aigentflow_version: '2.0.0'
name: executor-probe
version: '1.0.0'
description: executor probe
start: s
steps:
  s:
    executor: ` + executor + `
    query:
      message: hello
    next:
      default: 'null'
`
}

func loopExecutorProbe(executor string) string {
	return `aigentflow_version: '2.0.0'
name: executor-probe
version: '1.0.0'
description: executor probe
start: s
steps:
  s:
    loop:
      while: '{{ lt (atoi .loop.vars.counter) 2 }}'
      max_iterations: 3
      vars:
        counter: '0'
      steps:
        - id: body
          executor: ` + executor + `
          query:
            message: hello
          post_processing:
            - loop.set:
                counter: '{{ add (atoi .loop.vars.counter) 1 }}'
    next:
      default: 'null'
`
}

func TestFunctionOperationEveryBuiltinIsAccepted(t *testing.T) {
	if got := len(spec.FunctionOperations.Operations); got != 32 {
		t.Fatalf("the reference registers 32 built-ins, spec has %d", got)
	}
	for _, op := range spec.FunctionOperations.Operations {
		for name, src := range map[string]string{
			"top":  executorProbe("function://" + op),
			"loop": loopExecutorProbe("function://" + op),
		} {
			r := ValidateFlow(src, Options{})
			if hasCode(r.Errors, codeUnknownFunctionOperation) || !r.Valid {
				t.Errorf("%s: built-in %q refused: %s", name, op, formatIssues(r.Errors))
			}
		}
	}
}

func TestFunctionOperationUnknownIsRefused(t *testing.T) {
	list := strings.Join(sortedCopy(spec.FunctionOperations.Operations), ", ")
	for _, op := range []string{"utils/noop", "text/template", "text/noop", "notify/send", "utils/log/extra", "Utils/Log", "text/count-word", "demo/processor"} {
		t.Run(op, func(t *testing.T) {
			r := ValidateFlow(executorProbe("function://"+op), Options{})
			got, ok := findByCode(r.Errors, codeUnknownFunctionOperation)
			if !ok || r.Valid {
				t.Fatalf("want error and invalid, got %s", formatIssues(r.Errors))
			}
			if got.Field != "steps.s.executor" || got.StepID != "s" {
				t.Errorf("field/step = %q / %q", got.Field, got.StepID)
			}
			want := `step "s" uses function://` + op + ", which is not a built-in function. Available: " + list
			if got.Message != want {
				t.Errorf("message = %q", got.Message)
			}
			if !strings.Contains(got.Suggestion, list) {
				t.Errorf("suggestion lacks the list: %q", got.Suggestion)
			}
			if hasCode(r.Warnings, codeUnknownFunctionOperation) {
				t.Errorf("must be an error only")
			}
		})
	}
}

func TestFunctionOperationLoopSubStep(t *testing.T) {
	r := ValidateFlow(loopExecutorProbe("function://utils/noop"), Options{})
	got, ok := findByCode(r.Errors, codeUnknownFunctionOperation)
	if !ok || r.Valid {
		t.Fatalf("want error, got %s", formatIssues(r.Errors))
	}
	if got.StepID != "s.body" || !strings.HasPrefix(got.Field, "steps.s.loop.steps[0]") {
		t.Errorf("step/field = %q / %q", got.StepID, got.Field)
	}
}

func TestFunctionOperationSkips(t *testing.T) {
	for name, executor := range map[string]string{
		"templated":    `'function://{{ .query.p }}/{{ .query.o }}'`,
		"templated op": `'function://utils/{{ .query.o }}'`,
		"other scheme": "http://utils/noop",
		"unparsable":   "'function://'",
	} {
		t.Run(name, func(t *testing.T) {
			r := ValidateFlow(executorProbe(executor), Options{})
			if hasCode(r.Errors, codeUnknownFunctionOperation) {
				t.Fatalf("must be skipped: %s", formatIssues(r.Errors))
			}
		})
	}
	// An unparsable URL stays owned by invalid_executor_url.
	r := ValidateFlow(executorProbe("'function://'"), Options{})
	if !hasCode(r.Errors, codeInvalidExecutorURL) {
		t.Errorf("invalid_executor_url expected: %s", formatIssues(r.Errors))
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestUnknownExecutorSchemeMessage(t *testing.T) {
	cases := []struct {
		executor     string
		wantContains string
		wantAbsent   string
	}{
		{"decide://openai/choose", "ai://openai/decide", "Did you mean"},
		{"Decide://openai/choose", "ai://openai/decide", "Did you mean"},
		{"functon://utils/log", "Did you mean 'function'?", "ai://openai/decide"},
		{"FUNCTON://utils/log", "Did you mean 'function'?", ""},
		{"zzzzzzzz://a/b", "newer AIgentFlow release", "Did you mean"},
	}
	for _, c := range cases {
		t.Run(c.executor, func(t *testing.T) {
			r := ValidateFlow(executorProbe(c.executor), Options{})
			got, ok := findByCode(r.Warnings, codeUnknownExecScheme)
			if !ok {
				t.Fatalf("no warning: %s", formatIssues(r.Warnings))
			}
			if !r.Valid {
				t.Errorf("verdict must not change: %s", formatIssues(r.Errors))
			}
			if !strings.Contains(got.Message, c.wantContains) {
				t.Errorf("message = %q, want %q", got.Message, c.wantContains)
			}
			if c.wantAbsent != "" && strings.Contains(got.Message, c.wantAbsent) {
				t.Errorf("message = %q must not contain %q", got.Message, c.wantAbsent)
			}
		})
	}
}
