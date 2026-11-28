package aifvalidate

import (
	"strings"
	"testing"
)

// v0.30.0 (AIgentFlow v2.940.0): a step id that names engine state is refused,
// reserved exactly as a data: key is (a step's output lands under its id in .data).

func v0300Flow(stepID string) string {
	return "aigentflow_version: '2.0.0'\nname: t\nversion: '1.0.0'\nstart: " + stepID +
		"\nsteps:\n  " + stepID + ":\n    executor: function://utils/log\n    query:\n      message: hi\n    next:\n      default: 'null'\n"
}

func v0300Count(src, code string) int {
	n := 0
	for _, e := range ValidateFlow(src, Options{}).Errors {
		if e.Code == code {
			n++
		}
	}
	return n
}

func TestV0300ReservedStepIDFlowData(t *testing.T) {
	for _, id := range []string{"__untrusted_steps", "__anything", "_loop_context", "_x", "memory", "org_id", "current_flow", "campaign"} {
		if got := v0300Count(v0300Flow(id), codeReservedStepIDFlowData); got != 1 {
			t.Errorf("%q names engine state: want 1 %s, got %d", id, codeReservedStepIDFlowData, got)
		}
	}
	for _, id := range []string{"a__b", "untrusted_steps", "Memory", "org", "x_"} {
		if got := v0300Count(v0300Flow(id), codeReservedStepIDFlowData); got != 0 {
			t.Errorf("%q names no engine state: want none, got %d", id, got)
		}
	}
	// The two older rules own their ids: no second finding.
	if got := v0300Count(v0300Flow("orchestrator"), codeReservedStepIDFlowData); got != 0 {
		t.Errorf("orchestrator draws reserved_step_id_orchestrator only, got %d of the new code", got)
	}
	r := ValidateFlow(v0300Flow("__untrusted_steps"), Options{})
	for _, e := range r.Errors {
		if e.Code == codeReservedStepIDFlowData && !strings.Contains(e.Message, `"__untrusted_steps"`) {
			t.Errorf("the message quotes the id as %%q: %s", e.Message)
		}
	}
}
