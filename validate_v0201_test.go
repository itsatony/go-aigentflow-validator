package aifvalidate

import (
	"strings"
	"testing"
)

func aiflowFlow(executor string) string {
	return "aigentflow_version: \"2.0.0\"\nname: t\nversion: \"1.0.0\"\nstart: a\nsteps:\n  a:\n    executor: \"" +
		executor + "\"\n    next:\n      default: null\n"
}

func TestAiflowSchemeIsKnown(t *testing.T) {
	found := false
	for _, s := range ExecutorSchemes() {
		if s == "aiflow" {
			found = true
		}
	}
	if !found {
		t.Fatal("aiflow missing from ExecutorSchemes")
	}
	for _, exec := range []string{"aiflow://vai/baeckerei-zettel", "aiflow://org_acme/translator/v3", "AIFLOW://vai/x"} {
		res := ValidateFlow(aiflowFlow(exec), Options{})
		for _, w := range res.Warnings {
			if w.Code == codeUnknownExecScheme {
				t.Errorf("%s: unexpected %s: %s", exec, w.Code, w.Message)
			}
		}
		for _, e := range res.Errors {
			if e.Code == codeInvalidExecutorURL {
				t.Errorf("%s: unexpected %s", exec, e.Code)
			}
		}
	}
}

func TestAiflowOneSegmentStillRefused(t *testing.T) {
	res := ValidateFlow(aiflowFlow("aiflow://onlyone"), Options{})
	assertError(t, res, codeInvalidExecutorURL, "steps.a.executor")
}

func TestAiflowTypoStillWarnsAndSuggestsAiflow(t *testing.T) {
	res := ValidateFlow(aiflowFlow("aiflw://vai/x"), Options{})
	for _, w := range res.Warnings {
		if w.Code == codeUnknownExecScheme && strings.Contains(w.Message, "Did you mean 'aiflow'?") {
			return
		}
	}
	t.Errorf("expected a did-you-mean aiflow warning, got %+v", res.Warnings)
}
