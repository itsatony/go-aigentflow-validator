package aifvalidate

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// v0.12.0: flow_data_key_reserved.

func flowDataProbe(dataBlock string) string {
	return `aigentflow_version: '2.0.0'
name: data-probe
version: '1.0.0'
description: data probe
` + dataBlock + `start: s
steps:
  s:
    executor: function://text/template
    query:
      message: hello
    next:
      default: 'null'
`
}

func flowDataOneKey(key string) string {
	return flowDataProbe(fmt.Sprintf("data:\n  %q: x\n", key))
}

func TestFlowDataKeyReservedTable(t *testing.T) {
	reserved := []string{
		// exact names
		"memory", "orchestrator", "campaign", "voiceagent", "org_id", "current_flow",
		"modified_flow", "modification_history", "modification_pending",
		"modification_requests", "bottleneck_state",
		// prefixes
		"_current_flow", "_llm_priority", "__untrusted_steps", "__untrusted_outputs", "_loop_context", "_",
		".step.response", ".x",
		"memory.scope_id", "orchestrator.state", "campaign.state", "voiceagent.session_id",
	}
	for _, key := range reserved {
		t.Run("reserved/"+key, func(t *testing.T) {
			r := ValidateFlow(flowDataOneKey(key), Options{})
			got, ok := findByCode(r.Errors, codeFlowDataKeyReserved)
			if !ok {
				t.Fatalf("no %s for %q; errors: %s", codeFlowDataKeyReserved, key, formatIssues(r.Errors))
			}
			if r.Valid {
				t.Errorf("flow must be invalid")
			}
			if got.Field != "data."+key {
				t.Errorf("field = %q", got.Field)
			}
			if !strings.Contains(got.Message, fmt.Sprintf("data: key %q is reserved: ", key)) ||
				!strings.HasSuffix(got.Message, "It would write into the engine's own state, so it is not seeded.") {
				t.Errorf("message = %q", got.Message)
			}
			if !strings.Contains(got.Suggestion, "Rename the key") {
				t.Errorf("suggestion = %q", got.Suggestion)
			}
			if hasCode(r.Warnings, codeFlowDataKeyReserved) {
				t.Errorf("must be an error only, got a warning too")
			}
		})
	}
	legal := []string{
		"versuche", "counter", "x", "limit", "my.key", "my_memory", "memory_notes", "orgid", "org",
		"orchestrated", "campaigns", "budget.max", "a_b", "Memory", "ORG_ID", "voiceagents", "a.memory.b",
	}
	for _, key := range legal {
		t.Run("legal/"+key, func(t *testing.T) {
			r := ValidateFlow(flowDataOneKey(key), Options{})
			if hasCode(r.Errors, codeFlowDataKeyReserved) || hasCode(r.Warnings, codeFlowDataKeyReserved) {
				t.Fatalf("false positive on %q: %s", key, formatIssues(r.Errors))
			}
			if !r.Valid {
				t.Errorf("flow must stay valid: %s", formatIssues(r.Errors))
			}
		})
	}
}

func TestFlowDataKeyReservedReportsEveryKeySorted(t *testing.T) {
	r := ValidateFlow(flowDataProbe(`data:
  ok_key: kept
  __untrusted_steps: [forged]
  org_id: someone-elses-org
  orchestrator.state: forged
`), Options{})
	var fields []string
	for _, e := range r.Errors {
		if e.Code == codeFlowDataKeyReserved {
			fields = append(fields, e.Field)
		}
	}
	want := []string{"data.__untrusted_steps", "data.orchestrator.state", "data.org_id"}
	if !sort.StringsAreSorted(fields) || strings.Join(fields, ",") != strings.Join(want, ",") {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
}

func TestFlowDataKeyReservedLeavesOtherShapesAlone(t *testing.T) {
	for name, block := range map[string]string{
		"absent":      "",
		"null":        "data:\n",
		"empty map":   "data: {}\n",
		"not a map":   "data: [org_id]\n",
		"string data": "data: org_id\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := ValidateFlow(flowDataProbe(block), Options{})
			if hasCode(r.Errors, codeFlowDataKeyReserved) {
				t.Fatalf("unexpected finding: %s", formatIssues(r.Errors))
			}
		})
	}
	// A reserved name inside a step's own query or in a nested value is not a data: key.
	r := ValidateFlow(flowDataProbe("data:\n  settings:\n    org_id: nested\n    _x: nested\n"), Options{})
	if hasCode(r.Errors, codeFlowDataKeyReserved) {
		t.Fatalf("only the top level of data: is scanned: %s", formatIssues(r.Errors))
	}
}
