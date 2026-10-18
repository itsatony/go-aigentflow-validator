package aifvalidate

import (
	"fmt"
	"strings"
	"testing"
)

// v0.11.0: the here:// scheme, the strict_templates key and the
// template_default_piped rule.

func defaultProbe(query string) string {
	return fmt.Sprintf(`aigentflow_version: '2.0.0'
name: default-probe
version: '1.0.0'
description: default probe
start: a
steps:
  a:
    executor: function://text/template
    query:
      message: %q
    next:
      default: 'null'
`, query)
}

func TestPipedDefaultIsAnError(t *testing.T) {
	for _, q := range []string{
		`Hello, {{ .query.name | default "world" }}!`,
		`{{ .query.n | default 10 }}`,
		`{{ .query.name | default "w" | upper }}`,
		`{{ (.query.name | default "w") }}`,
		`{{ if .query.a }}{{ .query.b | default "x" }}{{ end }}`,
		`{{ range .query.items }}{{ . | default "x" }}{{ end }}`,
		`{{ with .query.a }}{{ . | default "x" }}{{ end }}`,
		`{{ define "d" }}{{ .x | default "w" }}{{ end }}{{ template "d" . }}`,
		`{{ $v := .query.a | default "w" }}{{ $v }}`,
		`{{ upper (.query.a | default "w") }}`,
		// An unknown function elsewhere must not hide the finding.
		`{{ nosuchfn .x }}{{ .y | default "w" }}`,
	} {
		t.Run(q, func(t *testing.T) {
			r := ValidateFlow(defaultProbe(q), Options{})
			got, ok := findByCode(r.Errors, codeTemplateDefaultPiped)
			if !ok {
				t.Fatalf("no %s for %q; errors: %s", codeTemplateDefaultPiped, q, formatIssues(r.Errors))
			}
			if r.Valid {
				t.Errorf("flow must be invalid")
			}
			if got.Field != "steps.a.query.message" || got.StepID != "a" {
				t.Errorf("field/step = %q/%q", got.Field, got.StepID)
			}
			if !strings.Contains(got.Message, "takes the value FIRST") ||
				!strings.Contains(got.Message, "the fallback is always the result") {
				t.Errorf("message = %q", got.Message)
			}
			if !strings.Contains(got.Suggestion, `{{ default .x "fallback" }}`) {
				t.Errorf("suggestion = %q", got.Suggestion)
			}
		})
	}
}

func TestPipedDefaultHasNoFalsePositive(t *testing.T) {
	for _, q := range []string{
		`{{ default .query.name "world" }}`,
		`{{ atoi (default .query.n "0") }}`,
		`{{ .query.name | upper }}`,
		`{{ .query.default }}`,
		`{{ .query.default | upper }}`,
		`{{ index .query "default" }}`,
		`literal text: | default "w"`,
		`{{ "x | default y" }}`,
		`{{ $default := "w" }}{{ $default | upper }}`,
		`{{ default (.query.a | upper) "w" }}`,
		`{{/* .x | default "w" */}}done`,
		`{{ .query.name | printf "%s" }}`,
		`{{ .query.a | default_value }}`, // unknown function, not this rule
		`{{ if .query.a }}{{ default .query.a "x" }}{{ end }}`,
		`{{ .query.a | `, // syntax error: no tree, no finding from this rule
	} {
		t.Run(q, func(t *testing.T) {
			r := ValidateFlow(defaultProbe(q), Options{})
			if hasCode(r.Errors, codeTemplateDefaultPiped) || hasCode(r.Warnings, codeTemplateDefaultPiped) {
				t.Fatalf("false positive on %q: %s / %s", q, formatIssues(r.Errors), formatIssues(r.Warnings))
			}
		})
	}
}

func TestPipedDefaultInALoopBodyIsAWarning(t *testing.T) {
	src := `aigentflow_version: '2.0.0'
name: loop-default
version: '1.0.0'
description: d
start: looper
steps:
  looper:
    loop:
      while: 'true'
      max_iterations: 3
      steps:
        - id: body
          executor: function://text/template
          query:
            m: '{{ .query.x | default "w" }}'
          next:
            conditions:
              - if: '{{ .query.y | default "z" }}'
                goto: body
    next:
      default: 'null'
`
	r := ValidateFlow(src, Options{})
	if hasCode(r.Errors, codeTemplateDefaultPiped) {
		t.Fatalf("loop body must demote: %s", formatIssues(r.Errors))
	}
	n := 0
	for _, w := range r.Warnings {
		if w.Code == codeTemplateDefaultPiped {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("want 2 warnings (query + condition), got %d: %s", n, formatIssues(r.Warnings))
	}
}

func TestPipedDefaultInNextConditionIsAnError(t *testing.T) {
	src := `aigentflow_version: '2.0.0'
name: cond-default
version: '1.0.0'
description: d
start: a
steps:
  a:
    executor: function://text/template
    query:
      m: x
    next:
      conditions:
        - if: '{{ .query.y | default "z" }}'
          goto: a
      default: 'null'
`
	r := ValidateFlow(src, Options{})
	got, ok := findByCode(r.Errors, codeTemplateDefaultPiped)
	if !ok || got.Field != "steps.a.next.conditions[0].if" {
		t.Fatalf("got %v %s", ok, formatIssues(r.Errors))
	}
}

func TestHereIsAKnownExecutorScheme(t *testing.T) {
	for _, op := range []string{"geocode", "reverse_geocode", "route", "places"} {
		src := fmt.Sprintf(`aigentflow_version: '2.0.0'
name: here
version: '1.0.0'
description: d
start: a
steps:
  a:
    executor: here://maps/%s
    query:
      q: x
    next:
      default: 'null'
`, op)
		r := ValidateFlow(src, Options{})
		if hasCode(r.Warnings, codeUnknownExecScheme) || hasCode(r.Errors, codeUnknownExecScheme) {
			t.Errorf("%s: unknown-scheme finding: %s", op, formatIssues(r.Warnings))
		}
	}
}

func TestStrictTemplatesKey(t *testing.T) {
	flow := func(v string) string {
		return `aigentflow_version: '2.0.0'
name: st
version: '1.0.0'
description: d
strict_templates: ` + v + `
start: a
steps:
  a:
    executor: function://text/template
    query:
      m: x
    next:
      default: 'null'
`
	}
	for _, v := range []string{"true", "false", "null", "yes", "off"} {
		r := ValidateFlow(flow(v), Options{})
		if hasCode(r.Errors, codeUnknownYAMLKey) || hasCode(r.Errors, codeInvalidType) {
			t.Errorf("%s must be accepted: %s", v, formatIssues(r.Errors))
		}
	}
	for _, v := range []string{"5", "'maybe'", "'true'", "[true]", "{a: 1}", "1.5"} {
		r := ValidateFlow(flow(v), Options{})
		got, ok := findByCode(r.Errors, codeInvalidType)
		if !ok || got.Field != "strict_templates" || r.Valid {
			t.Errorf("%s must be refused as invalid_type: valid=%v %s", v, r.Valid, formatIssues(r.Errors))
		}
		if hasCode(r.Errors, codeUnknownYAMLKey) {
			t.Errorf("%s: strict_templates must be a KNOWN key", v)
		}
	}
}
