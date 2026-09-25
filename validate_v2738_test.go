package aifvalidate

import (
	"strings"
	"testing"
)

// Rules ported in the v2.485.0 → v2.738.0 parity refresh (v0.3.0). The shared
// conformance fixtures cover most of them; these focused tests pin the edges
// the fixtures do not, and every warning rule has a false-positive guard: a
// shape the reference accepts must not be flagged.

// withStep replaces minimalFlow's step body with body (indented as step keys).
func withStep(body string) string {
	return strings.Replace(minimalFlow, "    executor: function://text/noop\n", body, 1)
}

func TestRetiredKeys(t *testing.T) {
	t.Run("presence, not value: budget: 0 and budget: null are refused", func(t *testing.T) {
		for _, v := range []string{"0", "", "null", "5.0"} {
			assertError(t, ValidateFlow(minimalFlow+"budget: "+v+"\n", Options{}), codeUnknownYAMLKey, keyBudget)
		}
	})
	t.Run("flow-level max_retries", func(t *testing.T) {
		assertError(t, ValidateFlow(minimalFlow+"max_retries: 0\n", Options{}), codeUnknownYAMLKey, keyMaxRetries)
	})
	t.Run("step-level max_retries", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    max_retries: 3\n")
		assertError(t, ValidateFlow(src, Options{}), codeUnknownYAMLKey, "steps.only.max_retries")
	})
	t.Run("loop sub-step max_retries", func(t *testing.T) {
		src := withStep("    loop:\n      while: \"{{ true }}\"\n      max_iterations: 2\n      steps:\n" +
			"        - id: s\n          executor: function://text/noop\n          max_retries: 1\n")
		assertError(t, ValidateFlow(src, Options{}), codeUnknownYAMLKey, "steps.only.loop.steps[0].max_retries")
	})
	t.Run("the working keys one level down are not refused", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    error_strategy:\n      action: retry\n      max_retries: 3\n") +
			"error_strategy:\n  action: retry\n  max_retries: 2\nbilling:\n  max_credits: 100\n"
		assertValid(t, ValidateFlow(src, Options{}))
	})
}

func TestUnreachableErrorGoto(t *testing.T) {
	t.Run("retry beside goto_step warns", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    error_strategy:\n      action: retry\n      goto_step: only\n")
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeUnreachableErrorGoto)
	})
	t.Run("absent action beside goto_step warns (flow level)", func(t *testing.T) {
		src := minimalFlow + "error_strategy:\n  goto_step: only\n"
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeUnreachableErrorGoto)
	})
	t.Run("action goto does not warn", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    error_strategy:\n      action: goto\n      goto_step: only\n")
		if r := ValidateFlow(src, Options{}); hasCode(r.Warnings, codeUnreachableErrorGoto) {
			t.Fatalf("goto beside action goto must not warn: %s", formatIssues(r.Warnings))
		}
	})
}

func TestLoopSubStepNextAndIDs(t *testing.T) {
	loop := func(subs string) string {
		return withStep("    loop:\n      while: \"{{ true }}\"\n      max_iterations: 2\n      steps:\n" + subs)
	}
	t.Run("end inside a loop body is a missing target, not a sentinel", func(t *testing.T) {
		src := loop("        - id: a\n          executor: function://text/noop\n          next:\n            default: end\n")
		assertError(t, ValidateFlow(src, Options{}), codeLoopSubstepNextNotFound, "steps.only.loop.steps[0].next.default")
	})
	t.Run("a non-string target is invalid_type", func(t *testing.T) {
		src := loop("        - id: a\n          executor: function://text/noop\n          next:\n            default: [x]\n")
		assertError(t, ValidateFlow(src, Options{}), codeInvalidType, "steps.only.loop.steps[0].next.default")
	})
	t.Run("forward jump and empty target are legal", func(t *testing.T) {
		src := loop("        - id: a\n          executor: function://text/noop\n          next:\n            default: b\n" +
			"        - id: b\n          executor: function://text/noop\n          next:\n            default: \"\"\n")
		assertValid(t, ValidateFlow(src, Options{}))
	})
	t.Run("reserved sub-step id warns, others do not", func(t *testing.T) {
		src := loop("        - id: vars\n          executor: function://text/noop\n")
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeLoopSubStepIDReserved)
		src = loop("        - id: variables\n          executor: function://text/noop\n")
		if r := ValidateFlow(src, Options{}); hasCode(r.Warnings, codeLoopSubStepIDReserved) {
			t.Fatalf("a non-reserved id must not warn: %s", formatIssues(r.Warnings))
		}
	})
	t.Run("goto_step in a loop body warns; the LOOP step's own goto does not", func(t *testing.T) {
		src := loop("        - id: a\n          executor: function://text/noop\n          error_strategy:\n            action: goto\n            goto_step: only\n")
		r := ValidateFlow(src, Options{})
		assertWarningNotError(t, r, codeLoopSubstepErrGotoIgnore)
		if issue, _ := findByCode(r.Warnings, codeLoopSubstepErrGotoIgnore); issue.Field != "steps.only.loop.steps.a.error_strategy.goto_step" {
			t.Errorf("field = %q", issue.Field)
		}
		if hasCode(r.Warnings, codeUnreachableErrorGoto) {
			t.Errorf("unreachable_error_goto must not fire inside a loop body: %s", formatIssues(r.Warnings))
		}
	})
	t.Run("a loop-body template error is a warning, even under StrictRegistries", func(t *testing.T) {
		src := loop("        - id: a\n          executor: function://text/noop\n          query:\n            x: \"{{ if }}\"\n            y: \"{{ nosuchfn .a }}\"\n")
		r := ValidateFlow(src, Options{StrictRegistries: true})
		assertWarningNotError(t, r, codeTemplateSyntax)
		assertWarningNotError(t, r, codeTemplateFuncUnkn)
	})
}

func TestProcessingOperationPaths(t *testing.T) {
	t.Run("a template finding is addressed without the operation name", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    post_processing:\n      - data.set:\n          out: \"{{ if }}\"\n")
		assertError(t, ValidateFlow(src, Options{}), codeTemplateSyntax, "steps.only.post_processing[0].out")
	})
	t.Run("a malformed two-key entry gets no shape verdict", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    post_processing:\n      - data.set: {a: 1}\n        nope: {b: 2}\n")
		r := ValidateFlow(src, Options{})
		if hasCode(r.Warnings, codeUnknownProcessingOp) || hasCode(r.Warnings, codeUnknownProcessingConfigKey) {
			t.Fatalf("divergence #11: no verdict on a malformed entry: %s", formatIssues(r.Warnings))
		}
	})
}

func TestStepMaxDurationScalars(t *testing.T) {
	for _, tc := range []struct {
		value string
		warn  bool
	}{
		{"90", true}, {"true", true}, {"2d", true}, {"\"{{ .query.t }}\"", true},
		{"0", false}, {"0s", false}, {"-5m", false}, {"none", false}, {"1h30m", false},
	} {
		src := withStep("    executor: function://text/noop\n    max_duration: " + tc.value + "\n")
		r := ValidateFlow(src, Options{})
		if got := hasCode(r.Warnings, codeStepMaxDurationIgnored); got != tc.warn {
			t.Errorf("max_duration %s: warn = %v, want %v (%s)", tc.value, got, tc.warn, formatIssues(r.Warnings))
		}
		assertValid(t, r)
	}
}

func TestResponseExpectationUnreadEmptyEvaluation(t *testing.T) {
	src := withStep("    executor: function://text/noop\n    response_evaluation: \"\"\n" +
		"    response_expectation:\n      x:\n        type: string\n")
	assertWarningNotError(t, ValidateFlow(src, Options{}), codeRespExpUnread)
}

func TestHumanQuestionTimeout(t *testing.T) {
	orch := func(v string) string {
		return minimalFlow + "orchestrator:\n  exons: \"spec\"\n  agentic: true\n  human_question_timeout: " + v + "\n"
	}
	for _, v := range []string{"-5m", "0s", "30", "soon", "[1]"} {
		assertError(t, ValidateFlow(orch(v), Options{}), codeOrchHumanQTimeoutInvalid, "orchestrator.human_question_timeout")
	}
	for _, v := range []string{"30m", "\"\"", "null", ""} {
		assertValid(t, ValidateFlow(orch(v), Options{}))
	}
}

func TestOrchestratorYieldViaConditionGoto(t *testing.T) {
	// A conditional yield counts as the owner-mode yield edge. v0.2.0 read the
	// condition's `goto_step`, so this valid flow was refused.
	src := withStep("    executor: function://text/noop\n    next:\n      conditions:\n"+
		"        - if: \"{{ true }}\"\n          goto: orchestrator\n") +
		"orchestrator:\n  exons: \"spec\"\n  agentic: true\n  mode: owner\n"
	assertValid(t, ValidateFlow(src, Options{}))
}

func TestCampaignRules(t *testing.T) {
	camp := func(body string) string {
		return minimalFlow + "orchestrator:\n  exons: \"spec\"\n  agentic: true\ncampaign:\n" + body
	}
	t.Run("max_credits_per_child", func(t *testing.T) {
		for _, v := range []string{"-1", "-1.5"} {
			assertError(t, ValidateFlow(camp("  child_flows:\n    - flow_name: c\n  max_credits_per_child: "+v+"\n"), Options{}),
				codeCampaignMaxCreditsChild, "campaign.max_credits_per_child")
		}
		for _, v := range []string{"\"5\"", "true", "1e20"} {
			assertError(t, ValidateFlow(camp("  child_flows:\n    - flow_name: c\n  max_credits_per_child: "+v+"\n"), Options{}),
				codeInvalidType, "campaign.max_credits_per_child")
		}
		// The reference truncates a fractional number into its int64 field and
		// saves the flow; refusing it would be stricter than the door.
		for _, v := range []string{"0", "5", "1.5", "-0.5", "9223372036854775807", "null"} {
			assertValid(t, ValidateFlow(camp("  child_flows:\n    - flow_name: c\n  max_credits_per_child: "+v+"\n"), Options{}))
		}
	})
	t.Run("child_flows", func(t *testing.T) {
		assertError(t, ValidateFlow(camp("  max_concurrent: 2\n"), Options{}), codeCampaignNoChildFlows, "campaign.child_flows")
		assertError(t, ValidateFlow(camp("  child_flows: []\n"), Options{}), codeCampaignNoChildFlows, "campaign.child_flows")
		assertError(t, ValidateFlow(camp("  child_flows:\n    - alias: x\n"), Options{}), codeCampaignChildFlowNoID, "campaign.child_flows[0]")
		assertValid(t, ValidateFlow(camp("  child_flows:\n    - flow_id: 123\n"), Options{}))
		assertValid(t, ValidateFlow(camp("  child_flows:\n    - flow_name: c\n"), Options{}))
	})
}

// Save-door refusals ported ahead of the JS port (PARITY.md, "Ahead of the JS
// port"). Each has its accepted shapes pinned beside it.
func TestSaveDoorExtras(t *testing.T) {
	t.Run("reserved step id orchestrator", func(t *testing.T) {
		src := strings.ReplaceAll(minimalFlow, "only", "orchestrator")
		assertError(t, ValidateFlow(src, Options{}), codeReservedStepIDOrch, "steps.orchestrator")
		src = strings.ReplaceAll(minimalFlow, "only", "orchestrate")
		assertValid(t, ValidateFlow(src, Options{}))
	})
	t.Run("tool_discovery vocabulary", func(t *testing.T) {
		assertError(t, ValidateFlow(minimalFlow+"tool_discovery: of\n", Options{}), codeToolDiscoveryInvalid, keyToolDiscovery)
		src := minimalFlow + "orchestrator:\n  exons: \"spec\"\n  agentic: true\n  tool_discovery: nope\n"
		assertError(t, ValidateFlow(src, Options{}), codeToolDiscoveryInvalid, "orchestrator.tool_discovery")
		src = withStep("    executor: function://text/noop\n    query:\n      tool_discovery: eagr\n")
		assertError(t, ValidateFlow(src, Options{}), codeToolDiscoveryInvalid, "steps.only.query.tool_discovery")
		for _, ok := range []string{"eager", "lazy", "off", "\"\"", "\"{{ .query.mode }}\""} {
			assertValid(t, ValidateFlow(minimalFlow+"tool_discovery: "+ok+"\n", Options{}))
		}
		// On a step the surface is a free-form query key: only a string is judged.
		assertValid(t, ValidateFlow(withStep("    executor: function://text/noop\n    query:\n      tool_discovery: 5\n"), Options{}))
	})
	t.Run("mock scenario delay", func(t *testing.T) {
		mock := func(v string) string {
			return minimalFlow + "mock_scenarios:\n  s1:\n    only:\n      delay: " + v + "\n"
		}
		assertError(t, ValidateFlow(mock("100"), Options{}), codeMockDelayInvalid, "mock_scenarios.s1.only.delay")
		assertError(t, ValidateFlow(mock("fast"), Options{}), codeMockDelayInvalid, "mock_scenarios.s1.only.delay")
		for _, ok := range []string{"100ms", "0", "2s", "\"\""} {
			assertValid(t, ValidateFlow(mock(ok), Options{}))
		}
	})
	t.Run("output entries", func(t *testing.T) {
		assertError(t, ValidateFlow(minimalFlow+"output:\n  - x\n  - \"\"\n", Options{}), codeOutputParamEmpty, "output[1]")
		// Measured: a YAML null entry saves in the reference.
		assertValid(t, ValidateFlow(minimalFlow+"output:\n  - x\n  -\n", Options{}))
	})
}

func TestExecutorURLShape(t *testing.T) {
	for _, bad := range []string{"openai:///gpt-4", "ai://openai", "http://api.example.com/v1", "ai://openai/gpt-4.1", "notauri"} {
		src := strings.Replace(minimalFlow, "function://text/noop", bad, 1)
		assertError(t, ValidateFlow(src, Options{}), codeInvalidExecutorURL, "steps.only.executor")
	}
	for _, good := range []string{"exons://agent/execute", "exons://agent/execute/deep/path", "flow://stored/{{ .query.id }}"} {
		src := strings.Replace(minimalFlow, "function://text/noop", "\""+good+"\"", 1)
		assertValid(t, ValidateFlow(src, Options{}))
	}
}

func TestExpressionFunctionUsageScan(t *testing.T) {
	t.Run("the scan runs with no expression_functions block", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    query:\n      x: \"{{ fn_slugify .query.t }}\"\n")
		assertError(t, ValidateFlow(src, Options{}), codeExprFnUndeclaredUse, "steps.only.query.x")
	})
	t.Run("one finding per distinct name", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    query:\n      x: \"{{ fn_slugify .a }}\"\n      y: \"{{ fn_slugify .b }}\"\n")
		r := ValidateFlow(src, Options{})
		n := 0
		for _, e := range r.Errors {
			if e.Code == codeExprFnUndeclaredUse {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("want 1 finding, got %d: %s", n, formatIssues(r.Errors))
		}
	})
	t.Run("fields, data keys and quoted names are not calls", func(t *testing.T) {
		src := withStep("    executor: function://text/noop\n    query:\n" +
			"      a: \"{{ .data.fn_total }}\"\n      b: \"{{ index .data \\\"fn_result\\\" }}\"\n      c: \"fn_slugify in prose\"\n")
		assertValid(t, ValidateFlow(src, Options{}))
	})
}
