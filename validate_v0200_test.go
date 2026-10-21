package aifvalidate

import (
	"fmt"
	"strings"
	"testing"
)

func forEachItemsFlow(items string) string {
	return ("aigentflow_version: \"2.0.0\"\nname: t\nversion: \"1.0.0\"\nstart: a\nsteps:\n  a:\n    executor: function://utils/log\n    for_each:\n      items: " +
		fmt.Sprintf("%q", items) + "\n      as: item\n    query:\n      message: x\n    next:\n      default: null\n")
}

func TestForEachItemsUnresolvableMatrix(t *testing.T) {
	fires := []string{".data.seed.list", "a, b, c", `{"a": 1}`, "null", "42", `"str"`, "[1,"}
	for _, items := range fires {
		res := ValidateFlow(forEachItemsFlow(items), Options{})
		assertError(t, res, codeForEachItemsUnresolvable, "steps.a.for_each.items")
		for _, e := range res.Errors {
			if e.Code != codeForEachItemsUnresolvable {
				t.Errorf("%q: unexpected extra error %s", items, e.Code)
			}
			if e.StepID != "a" || e.Suggestion == "" {
				t.Errorf("%q: StepID/Suggestion not set: %+v", items, e)
			}
		}
	}
	quiet := []string{"{{ .data.seed.list }}", "x {{ y", `["a","b"]`, "[]", " [1,2] ", "\n[1]\n"}
	for _, items := range quiet {
		res := ValidateFlow(forEachItemsFlow(items), Options{})
		for _, e := range res.Errors {
			if e.Code == codeForEachItemsUnresolvable {
				t.Errorf("%q must not fire: %+v", items, e)
			}
		}
	}
}

func TestForEachItemsUnresolvableScope(t *testing.T) {
	// Empty stays for_each_items_required alone; a non-string gets no finding.
	for _, items := range []string{"items: \"\"", "items: [1, 2]", "as: item"} {
		src := "aigentflow_version: \"2.0.0\"\nname: t\nversion: \"1.0.0\"\nstart: a\nsteps:\n  a:\n    executor: function://utils/log\n    for_each:\n      " +
			items + "\n    query:\n      message: x\n    next:\n      default: null\n"
		for _, e := range ValidateFlow(src, Options{}).Errors {
			if e.Code == codeForEachItemsUnresolvable {
				t.Errorf("%q: fired: %+v", items, e)
			}
		}
	}
}

func TestForEachItemsUnresolvableMessageTruncates(t *testing.T) {
	long := strings.Repeat("é", 61)
	res := ValidateFlow(forEachItemsFlow(long), Options{})
	var msg string
	for _, e := range res.Errors {
		if e.Code == codeForEachItemsUnresolvable {
			msg = e.Message
		}
	}
	want := `for_each.items "` + strings.Repeat("é", 60) + `…" is neither a template ({{ … }}) nor a JSON array, so it can never resolve to a list; write {{ .data.<step>.<field> }} to read a value, or a JSON array literal such as '["a","b"]'`
	if msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
	exact := strings.Repeat("x", 60)
	for _, e := range ValidateFlow(forEachItemsFlow(exact), Options{}).Errors {
		if e.Code == codeForEachItemsUnresolvable && strings.Contains(e.Message, "…\"") {
			t.Errorf("60 runes must not be truncated: %q", e.Message)
		}
	}
}
