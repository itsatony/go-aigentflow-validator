package aifvalidate

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

// v0.5.1: a scalar the reference decodes into a Go `string` field is read here
// as the text that field receives — the scalar-VALUE counterpart of v0.5.0's
// numeric step KEYS. Every expected verdict below was measured on the
// reference's strict save parser (PARITY.md, v0.5.1 note).

// spellings covers every scalar kind whose decoded `any` value is not its text.
var spellings = []string{
	"2", "-0", "+1", "017", "0o17", "0x1F", "1_000", // !!int
	"1e3", ".5", "0.10", ".inf", // !!float
	"true", "True", "FALSE", // !!bool
	"2024-01-01", // !!timestamp
}

// TestYAMLStringFieldReceivesSourceText pins the premise of stringAt against
// yaml.v3 itself: a Go `string` field and a map[string]… key receive a scalar's
// SOURCE text, and ParseFlow + collectScalarSources reproduce exactly that.
func TestYAMLStringFieldReceivesSourceText(t *testing.T) {
	type typed struct {
		V     string            `yaml:"v"`
		Steps map[string]string `yaml:"steps"`
	}
	for _, lit := range spellings {
		t.Run(lit, func(t *testing.T) {
			src := fmt.Sprintf("v: %s\nsteps:\n  %s: x\n", lit, lit)

			var want typed
			if err := yaml.Unmarshal([]byte(src), &want); err != nil {
				t.Fatalf("typed decode: %v", err)
			}
			if want.V != lit {
				t.Fatalf("premise: yaml.v3 gave the string field %q for %s", want.V, lit)
			}
			if _, ok := want.Steps[lit]; !ok {
				t.Fatalf("premise: yaml.v3 gave the map key %v for %s", want.Steps, lit)
			}

			flow, errs, _ := ParseFlow(src)
			if len(errs) > 0 {
				t.Fatalf("ParseFlow: %s", formatIssues(errs))
			}
			steps, _ := getRecord(flow, keySteps)
			if _, ok := steps[lit]; !ok {
				t.Errorf("ParseFlow key = %v, want %q", sortedKeys(steps), lit)
			}
			iss := newIssues()
			iss.sources = collectScalarSources(src)
			if got, ok := iss.stringOf(flow, "v", ""); !ok || got != lit {
				t.Errorf("stringOf = %q (ok=%v), want %q", got, ok, lit)
			}
		})
	}
}

// TestStringTypedFieldsReadNumbersAsText is one case per rule that reads a Go
// `string` field. Each input is a number or boolean where the rule expects text;
// the verdict is the reference's.
func TestStringTypedFieldsReadNumbersAsText(t *testing.T) {
	const head = "aigentflow_version: '2.0.0'\nname: p\nstart: a\n"
	const tail = "    next: {default: 'null'}\n"
	step := func(body string) string { return head + "steps:\n  a:\n    executor: mock://x/y\n" + body + tail }

	cases := []struct {
		name      string
		src       string
		wantError string // "" = the reference saves it
		field     string
		forbid    []string // error codes that must not appear
		// unreachable: the reference ALSO reports unreachable_step here (a
		// quality_gate goto is not a reachability edge in either).
		unreachable bool
	}{
		{name: "flow name", src: "aigentflow_version: '2.0.0'\nname: 123\nstart: a\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeMissingField}},
		{name: "aigentflow_version", src: "aigentflow_version: 2.0\nname: p\nstart: a\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeMissingField}},
		{name: "start", src: "aigentflow_version: '2.0.0'\nname: p\nstart: 1\nsteps:\n  1:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeMissingField, codeStepNotFound}},
		{name: "error_strategy action", src: step("    error_strategy:\n      action: 1\n"),
			wantError: codeInvalidErrStrategy, field: "steps.a.error_strategy.action"},
		{name: "flow-level error_strategy goto_step",
			src:    head + "error_strategy:\n  action: goto\n  goto_step: 2\nsteps:\n  a:\n    executor: mock://x/y\n" + tail + "  2:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeGotoStepMissing}},
		{name: "quality_gate on_fail", src: step("    quality_gate:\n      rubric: good\n      on_fail: 1\n"),
			wantError: codeQGInvalidOnFail, field: "steps.a.quality_gate.on_fail"},
		{name: "quality_gate goto_step",
			src:    step("    quality_gate:\n      rubric: good\n      on_fail: goto\n      goto_step: 2\n") + "  2:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeQGGotoMissing}, unreachable: true},
		{name: "quality_gate rubric", src: step("    quality_gate:\n      rubric: 5\n"),
			forbid: []string{codeQGMissingRubric}},
		{name: "rendezvous names no step", src: head + "steps:\n  a:\n    executor: mock://x/y\n    next:\n      parallel:\n        steps: [b]\n        rendezvous: 9\n  b:\n    executor: mock://x/y\n",
			wantError: codeStepNotFound, field: "steps.a.next.parallel.rendezvous"},
		{name: "for_each resolution", src: step("    for_each:\n      items: '{{ .query.xs }}'\n      resolution: 1\n"),
			wantError: codeForEachResolution, field: "steps.a.for_each.resolution"},
		{name: "for_each items", src: step("    for_each:\n      items: 5\n"),
			forbid: []string{codeForEachItemsRequired}},
		{name: "loop sub-step ids and next target",
			src: head + "steps:\n  a:\n    loop:\n      while: '{{ true }}'\n      max_iterations: 3\n      steps:\n" +
				"        - id: 3\n          executor: mock://x/y\n          next: {default: 4}\n" +
				"        - id: 4\n          executor: mock://x/z\n" + tail,
			forbid: []string{codeLoopStepIDRequired, codeInvalidType, codeLoopSubstepNextNotFound}},
		{name: "query parameter type is an unknown type, not a missing one",
			src:    head + "query:\n  q:\n    type: 1\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeQueryParamTypeMissing}},
		{name: "query property type",
			src:    head + "query:\n  q:\n    type: object\n    properties:\n      inner:\n        type: 1\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codePropertyTypeMissing}},
		{name: "query array items type",
			src:       head + "query:\n  q:\n    type: array\n    items:\n      type: 1\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			wantError: codeArrayItemsTypeInvalid, field: "query.q.items.type"},
		{name: "credential inject_as", src: step("    credentials:\n      k:\n        source: stored/openai/key\n        inject_as: 5\n"),
			forbid: []string{codeCredInjectAsEmpty}},
		{name: "expression function name is looked up, not called empty",
			src:       head + "expression_functions:\n  - function: 5\nsteps:\n  a:\n    executor: mock://x/y\n" + tail,
			wantError: codeExprFnUnknown, field: "expression_functions[0].function"},
		{name: "a timestamp step reference",
			src:    head + "steps:\n  a:\n    executor: mock://x/y\n    next: {default: 2024-01-01}\n  2024-01-01:\n    executor: mock://x/y\n" + tail,
			forbid: []string{codeStepNotFound}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ValidateFlow(tc.src, Options{})
			if tc.wantError == "" {
				assertValid(t, result)
			} else {
				assertError(t, result, tc.wantError, tc.field)
			}
			assertCodesAbsent(t, "error", result.Errors, tc.forbid)
			if !tc.unreachable {
				assertCodesAbsent(t, "warning", result.Warnings, []string{codeUnreachableStep})
			}
		})
	}
}

// TestNumericReferencesInADecodedObject: without source text, ValidateFlowObject
// renders a number by its decoded value — the same rendering it gives a numeric
// key — so an ordinary `2` still matches the step `2`.
func TestNumericReferencesInADecodedObject(t *testing.T) {
	decode := func(target string) map[string]any {
		t.Helper()
		var flow map[string]any
		src := "aigentflow_version: '2.0.0'\nname: p\nstart: a\nsteps:\n" +
			"  a:\n    executor: mock://x/y\n    next: {default: " + target + "}\n" +
			"  2:\n    executor: mock://x/y\n    next: {default: 'null'}\n"
		if err := yaml.Unmarshal([]byte(src), &flow); err != nil {
			t.Fatal(err)
		}
		return flow
	}
	result := ValidateFlowObject(decode("2"), Options{})
	assertValid(t, result)
	assertCodesAbsent(t, "warning", result.Warnings, []string{codeUnreachableStep})

	assertError(t, ValidateFlowObject(decode("3"), Options{}), codeStepNotFound, "steps.a.next.default")
}
