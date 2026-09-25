package aifvalidate

import (
	"fmt"
	"strings"
	"testing"
)

// Shared test helpers. Deliberately assertion-light (no testify) so this library
// keeps a single runtime dependency and a consumer vendoring it pulls in nothing
// extra for tests.

// codesOf collects the Code of each issue.
func codesOf(list []Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.Code)
	}
	return out
}

// hasCode reports whether any issue carries code.
func hasCode(list []Issue, code string) bool {
	for _, i := range list {
		if i.Code == code {
			return true
		}
	}
	return false
}

// findByCode returns the first issue with code, for asserting its Field/StepID.
func findByCode(list []Issue, code string) (Issue, bool) {
	for _, i := range list {
		if i.Code == code {
			return i, true
		}
	}
	return Issue{}, false
}

// formatIssues renders issues for a failure message: code, field, and position.
func formatIssues(list []Issue) string {
	if len(list) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(list))
	for _, i := range list {
		parts = append(parts, fmt.Sprintf("%s[%s]@%d:%d", i.Code, i.Field, i.Line, i.Column))
	}
	return strings.Join(parts, ", ")
}

// assertCodesPresent checks that every wanted code appears in list.
func assertCodesPresent(t *testing.T, label string, list []Issue, want []string) {
	t.Helper()
	for _, code := range want {
		if !hasCode(list, code) {
			t.Errorf("missing %s code %q; got: %s", label, code, formatIssues(list))
		}
	}
}

// assertCodesAbsent checks that no forbidden code appears in list.
func assertCodesAbsent(t *testing.T, label string, list []Issue, forbid []string) {
	t.Helper()
	for _, code := range forbid {
		if hasCode(list, code) {
			t.Errorf("forbidden %s code %q is present; got: %s", label, code, formatIssues(list))
		}
	}
}

// assertError asserts the result is invalid and carries code on field.
func assertError(t *testing.T, result Result, code, field string) {
	t.Helper()
	if result.Valid {
		t.Fatalf("expected invalid, got valid (warnings: %s)", formatIssues(result.Warnings))
	}
	issue, ok := findByCode(result.Errors, code)
	if !ok {
		t.Fatalf("expected error code %q; got: %s", code, formatIssues(result.Errors))
	}
	if field != "" && issue.Field != field {
		t.Errorf("error %q on field %q, want %q", code, issue.Field, field)
	}
}

// assertWarningNotError asserts code is present as a warning and the document is
// still valid — the distinction that keeps a publish gate from rejecting a flow
// whose only sin is a newer registry entry.
func assertWarningNotError(t *testing.T, result Result, code string) {
	t.Helper()
	if !result.Valid {
		t.Fatalf("expected valid (warning-only), got errors: %s", formatIssues(result.Errors))
	}
	if !hasCode(result.Warnings, code) {
		t.Fatalf("expected warning code %q; got: %s", code, formatIssues(result.Warnings))
	}
	if hasCode(result.Errors, code) {
		t.Fatalf("code %q must be a warning, not an error", code)
	}
}

// assertValid asserts a clean verdict, reporting every finding when it is not.
func assertValid(t *testing.T, result Result) {
	t.Helper()
	if !result.Valid {
		t.Fatalf("expected valid; errors: %s", formatIssues(result.Errors))
	}
}

// minimalFlow is a valid single-step flow, the base most table tests mutate.
const minimalFlow = `aigentflow_version: "2.0.0"
name: test-flow
version: 1.0.0
start: only
steps:
  only:
    executor: function://text/noop
`
