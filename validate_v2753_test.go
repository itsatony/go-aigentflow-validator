package aifvalidate

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// v0.5.0: the three gaps that were looser than the reference in both ports —
// `end` as a next target, generic unknown-key rejection, and a number in a
// duration field judged by its source spelling. Every expected verdict here was
// measured on the reference's strict save parser (PARITY.md).

func fieldsWithCode(list []Issue, code string) []string {
	var out []string
	for _, i := range list {
		if i.Code == code {
			out = append(out, i.Field)
		}
	}
	sort.Strings(out)
	return out
}

func TestNextEnd(t *testing.T) {
	t.Run("end is refused in a default and in a condition's goto when no step is called end", func(t *testing.T) {
		assertError(t, ValidateFlow(withStep("    executor: function://text/noop\n    next:\n      default: end\n"), Options{}),
			codeStepNotFound, "steps.only.next.default")
		src := withStep("    executor: function://text/noop\n    next:\n      conditions:\n" +
			"        - if: \"{{ true }}\"\n          goto: end\n      default: \"null\"\n")
		assertError(t, ValidateFlow(src, Options{}), codeStepNotFound, "steps.only.next.conditions[0].goto")
	})
	t.Run("null, a YAML null and an empty default finish the flow", func(t *testing.T) {
		for _, v := range []string{`"null"`, "null", `""`, ""} {
			assertValid(t, ValidateFlow(withStep("    executor: function://text/noop\n    next:\n      default: "+v+"\n"), Options{}))
		}
	})
	t.Run("a step named end saves and is unreachable, as in the reference", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    next:\n      default: end\n") +
			"  end:\n    executor: function://text/noop\n"
		result := ValidateFlow(src, Options{})
		assertValid(t, result)
		if got := fieldsWithCode(result.Warnings, codeUnreachableStep); !slices.Equal(got, []string{"steps.end"}) {
			t.Errorf("unreachable_step on %v, want [steps.end]", got)
		}
	})
}

func TestUnknownKeys(t *testing.T) {
	t.Run("refused at the root, on a step and at every nested struct level", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
constraints:
  currency: USD
steps:
  a:
    executor: mock://x/y
    output: [body]
    error_strategy: {action: fail, retries: 3}
    next: {default: "null", otherwise: b}
    for_each: {items: "{{ .query.l }}", throttle: {rate: 2}}
    quality_gate: {rubric: r, threshold: 0.5, on_fail: retry, judge: x}
    credentials: {k: {source: stored/openai/x, scope: x}}
    response_expectation: {f: {type: string, zz: 1}}
    loop:
      while: "{{ true }}"
      max_iterations: 2
      until: x
      steps:
        - {id: s, executor: mock://x/y, timeout: 5}
query:
  q: {type: string, label: Q}
input_schema:
  version: 1
  fields:
    - {name: q, type: string, placeholder: x}
mock_scenarios:
  s:
    a: {delay: 5ms, latency: 3}
billing: {max_credits: 5, currency: EUR}
executor_config:
  openai: {api_key: x, region: eu}
orchestrator:
  exons: x
  zz: 1
  triggers:
    - {type: step_completed, zz: 1}
`
		want := []string{
			"billing.currency", "constraints", "executor_config.openai.region",
			"input_schema.fields[0].placeholder", "mock_scenarios.s.a.latency",
			"orchestrator.triggers[0].zz", "orchestrator.zz", "query.q.label",
			"steps.a.credentials.k.scope", "steps.a.error_strategy.retries",
			"steps.a.for_each.throttle.rate", "steps.a.loop.steps[0].timeout", "steps.a.loop.until",
			"steps.a.next.otherwise", "steps.a.output", "steps.a.quality_gate.judge",
			"steps.a.response_expectation.f.zz",
		}
		result := ValidateFlow(src, Options{})
		if got := fieldsWithCode(result.Errors, codeUnknownYAMLKey); !slices.Equal(got, want) {
			t.Errorf("unknown_yaml_key fields:\n got %v\nwant %v", got, want)
		}
		if issue, _ := findByField(result.Errors, "steps.a.output"); issue.StepID != "a" {
			t.Errorf("steps.a.output carries step id %q, want a", issue.StepID)
		}
	})
	t.Run("a key whose value is null is still refused", func(t *testing.T) {
		assertError(t, ValidateFlow(minimalFlow+"zz:\n", Options{}), codeUnknownYAMLKey, "zz")
	})
	t.Run("a location another rule named is reported once", func(t *testing.T) {
		src := minimalFlow + "budget: 5\n"
		src = strings.Replace(src, "    executor: function://text/noop\n",
			"    executor: function://text/noop\n    max_retries: 2\n    next:\n      conditions:\n"+
				"        - if: \"{{ true }}\"\n          goto_step: only\n      default: \"null\"\n", 1)
		got := fieldsWithCode(ValidateFlow(src, Options{}).Errors, codeUnknownYAMLKey)
		want := []string{"budget", "steps.only.max_retries", "steps.only.next.conditions[0].goto_step"}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("a retired key is named as retired wherever it appears", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    for_each:\n      items: x\n      max_retries: 1\n")
		issue, ok := findByField(ValidateFlow(src, Options{}).Errors, "steps.only.for_each.max_retries")
		if !ok || issue.Code != codeUnknownYAMLKey || !strings.Contains(issue.Message, "removed from the grammar") {
			t.Errorf("got %+v", issue)
		}
	})
	t.Run("author-chosen key sets are left alone", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
currency: USD
max_duration: 10m
data:
  anything: {nested: [1, 2]}
executor_config:
  openai: {extra: {region: eu}}
steps:
  a:
    executor: mock://x/y
    query: {any_key: 1, nested: {deeper: {x: true}}}
    post_processing:
      - data.set: {author_chosen: "{{ .step.response }}"}
    next: {default: "null"}
mock_scenarios:
  s:
    a: {content: {any: {shape: [1]}}}
`
		assertValid(t, ValidateFlow(src, Options{}))
	})
	t.Run("null is accepted for every struct, list and scalar key", func(t *testing.T) {
		src := minimalFlow + "description:\nerror_strategy:\ntags:\n"
		src = strings.Replace(src, "    executor: function://text/noop\n",
			"    executor: function://text/noop\n    for_each:\n    next:\n    error_strategy:\n", 1)
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeInvalidType) || hasCode(result.Errors, codeUnknownYAMLKey) {
			t.Errorf("got %s", formatIssues(result.Errors))
		}
	})
	t.Run("a value of the wrong kind is refused, as the decoder refuses it", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
tags: research
output: {a: 1}
steps:
  a:
    executor: mock://x/y
    next: end
    error_strategy: fail
  b:
    executor: {url: mock://x/y}
    next:
      conditions: {if: x}
`
		got := fieldsWithCode(ValidateFlow(src, Options{}).Errors, codeInvalidType)
		for _, want := range []string{"output", "steps.a.error_strategy", "steps.a.next",
			"steps.b.executor", "steps.b.next.conditions", "tags"} {
			if !contains(got, want) {
				t.Errorf("no invalid_type on %s; got %v", want, got)
			}
		}
	})
	t.Run("a merge key and a numeric step id, as the reference reads them", func(t *testing.T) {
		src := "aigentflow_version: \"2.0.0\"\nname: m\nstart: \"1\"\ndata:\n  shared: &shared\n" +
			"    executor: mock://x/y\nsteps:\n  1:\n    <<: *shared\n    next:\n      default: \"null\"\n"
		assertValid(t, ValidateFlow(src, Options{}))
	})
}

// TestKnownKeyShapesAllResolve keeps an unresolvable shape out of a shipped
// spec: parseKnownKeyShape fails OPEN on one, which would silently disable the
// rule beneath it.
func TestKnownKeyShapesAllResolve(t *testing.T) {
	var check func(text string) bool
	check = func(text string) bool {
		switch {
		case text == shapeScalar || text == shapeAny:
			return true
		case strings.HasPrefix(text, shapeMapPrefix) && strings.HasSuffix(text, shapeSuffix):
			return check(text[len(shapeMapPrefix) : len(text)-len(shapeSuffix)])
		case strings.HasPrefix(text, shapeListPrefix) && strings.HasSuffix(text, shapeSuffix):
			return check(text[len(shapeListPrefix) : len(text)-len(shapeSuffix)])
		}
		_, ok := spec.KnownKeys.Types[text]
		return ok
	}
	if len(spec.KnownKeys.Types) == 0 || !check(spec.KnownKeys.Root) {
		t.Fatalf("knownKeys missing or root %q unresolved", spec.KnownKeys.Root)
	}
	for typeName, keys := range spec.KnownKeys.Types {
		for key, text := range keys {
			if !check(text) {
				t.Errorf("%s.%s: shape %q does not resolve", typeName, key, text)
			}
		}
	}
}

func TestDurationNumericSpellings(t *testing.T) {
	mock := func(spelling string) string {
		return minimalFlow + "mock_scenarios:\n  s:\n    only:\n      delay: " + spelling + "\n"
	}
	t.Run("a mock delay spelled as a zero with no unit is refused", func(t *testing.T) {
		for _, bad := range []string{"0.0", "00", "0x0", ".0", "-0.0", "0.", "0o0", "0_0", "1.5", "100"} {
			assertError(t, ValidateFlow(mock(bad), Options{}), codeMockDelayInvalid, "mock_scenarios.s.only.delay")
		}
		for _, ok := range []string{"0", "+0", "-0", "100ms", `"0"`} {
			assertValid(t, ValidateFlow(mock(ok), Options{}))
		}
	})
	t.Run("throttle delays and max_delay are judged the same way", func(t *testing.T) {
		fan := func(throttle, maxDelay string) string {
			return withStep("    executor: function://text/noop\n    for_each:\n      items: \"{{ .query.l }}\"\n" +
				"      throttle:\n        delay: " + throttle + "\n        batch_size: 2\n        batch_delay: " + throttle + "\n" +
				"    error_strategy:\n      action: retry\n      max_delay: " + maxDelay + "\n")
		}
		got := fieldsWithCode(ValidateFlow(fan("0.0", "100"), Options{}).Errors, codeInvalidDuration)
		want := []string{"steps.only.error_strategy.max_delay", "steps.only.for_each.throttle.batch_delay",
			"steps.only.for_each.throttle.delay"}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		assertValid(t, ValidateFlow(fan("0", "0"), Options{}))
	})
	t.Run("a timer interval of 0 saves and 100 does not", func(t *testing.T) {
		timer := func(interval string) string {
			return minimalFlow + "orchestrator:\n  exons: x\n  triggers:\n    - type: timer\n      interval: " + interval + "\n"
		}
		zero := ValidateFlow(timer("0"), Options{})
		assertCodesAbsent(t, "error", zero.Errors, []string{codeOrchTimerNoInterval, codeOrchTimerBadInterv})
		assertCodesPresent(t, "error", ValidateFlow(timer("100"), Options{}).Errors, []string{codeOrchTimerBadInterv})
	})
	t.Run("an object with no source text falls back to the decoded value", func(t *testing.T) {
		flow, _, _ := ParseFlow(mock("0"))
		flow["mock_scenarios"] = map[string]any{"s": map[string]any{"only": map[string]any{"delay": 0.0}}}
		assertValid(t, ValidateFlowObject(flow, Options{}))
	})
}

func findByField(list []Issue, field string) (Issue, bool) {
	for _, i := range list {
		if i.Field == field {
			return i, true
		}
	}
	return Issue{}, false
}

// A caller that decoded with yaml.v3 itself hands ValidateFlowObject a
// map[any]any beneath a numeric key. It is read as a mapping, and the caller's
// document is left untouched.
func TestValidateFlowObjectNormalisesNonStringKeysWithoutMutating(t *testing.T) {
	inner := map[any]any{1: map[string]any{"executor": "function://text/noop"}}
	flow := map[string]any{
		"aigentflow_version": "2.0.0", "name": "f", "start": "1", "steps": inner,
	}
	assertValid(t, ValidateFlowObject(flow, Options{}))
	if _, still := flow["steps"].(map[any]any); !still {
		t.Errorf("the caller's document was mutated: steps is now %T", flow["steps"])
	}
}
