package aifvalidate

import (
	"testing"
)

// v0.26.0 (AIgentFlow v2.912.0): catalogue sub-flows (flow://aiv/<org>/<name>),
// partials (include://aiv/<org>/<name>) and the flow_version step key.

func v0260Flow(executor, extra string) string {
	return "aigentflow_version: \"2.0.0\"\nname: test-flow\nversion: 1.0.0\nstart: only\nsteps:\n  only:\n    executor: '" +
		executor + "'\n" + extra + "    next:\n      default: 'null'\n"
}

func v0260Codes(t *testing.T, src string) map[string]Issue {
	t.Helper()
	res := ValidateFlow(src, Options{})
	out := map[string]Issue{}
	for _, e := range res.Errors {
		out[e.Code] = e
	}
	for _, w := range res.Warnings {
		if w.Code == "unknown_yaml_key" || w.Code == codeUnknownExecScheme {
			out[w.Code] = w
		}
	}
	return out
}

func TestV0260CatalogFlows(t *testing.T) {
	for _, ok := range []string{
		v0260Flow("flow://aiv/acme/summarise-doc", "    flow_version: '1.2.0'\n"),
		v0260Flow("flow://aiv/acme/summarise-doc", ""),
		v0260Flow("include://aiv/acme/review-text", "    flow_version: '0.3.0-rc.1'\n"),
		v0260Flow("flow://aiv/{{ .query.org }}/x", ""),
	} {
		if got := v0260Codes(t, ok); len(got) != 0 {
			t.Errorf("want no catalogue finding, got %v for\n%s", got, ok)
		}
	}

	got := v0260Codes(t, v0260Flow("flow://aiv/acme/x", "    flow_version: '1.2'\n"))
	issue, found := got["flow_version_invalid"]
	if !found {
		t.Fatalf("flow_version_invalid missing: %v", got)
	}
	if want := `step only: flow_version "1.2" is not a semver (MAJOR.MINOR.PATCH, e.g. 1.2.0)`; issue.Message != want {
		t.Errorf("message = %q, want %q", issue.Message, want)
	}
	if issue.Field != "steps.only.flow_version" {
		t.Errorf("field = %q", issue.Field)
	}

	for _, executor := range []string{"ai://openai/chat", "flow://stored/flw_abc123", "aiv://acme/agent"} {
		if _, found := v0260Codes(t, v0260Flow(executor, "    flow_version: '1.2.0'\n"))["flow_version_not_catalog"]; !found {
			t.Errorf("%s: flow_version_not_catalog missing", executor)
		}
	}
	for _, executor := range []string{"flow://aiv/acme/a/b", "include://aiv/acme", "flow://aiv/-acme/x"} {
		issue, found := v0260Codes(t, v0260Flow(executor, ""))["catalog_ref_invalid"]
		if !found {
			t.Errorf("%s: catalog_ref_invalid missing", executor)
			continue
		}
		if issue.Field != "steps.only.executor" {
			t.Errorf("%s: field = %q", executor, issue.Field)
		}
	}
}
