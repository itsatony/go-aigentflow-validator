package aifvalidate

import (
	"fmt"
	"time"
)

// Save-door refusals the reference applies in FlowParser.ValidateFlow. (The
// tool_discovery vocabulary rule went with the orchestrator in AIgentFlow
// v2.838.0: the flow-level key is orchestrator_removed, and nothing reads the
// vocabulary any more.) This
// library carried them first; the JS port ported them in its 0.13.0 (PARITY.md,
// v0.4.0 note). Each is a pure function of the document, so it is portable; each
// is an ERROR because AIgentFlow refuses the flow at save.

// validateSaveDoorExtras runs the rules above.
func validateSaveDoorExtras(flow doc, iss *issues) {
	validateMockScenarioDelays(flow, iss)
	validateOutputParameters(flow, iss)
}

// validateMockScenarioDelays refuses a mock step `delay:` that is not a Go
// duration (reference: validateMockScenarioDelays, AIF DC-FORGE-51). yaml
// decodes `delay: 100` into the string "100", which has no unit and would run
// with no delay at all; the reference makes the author write "100ms".
func validateMockScenarioDelays(flow doc, iss *issues) {
	scenarios, ok := getRecord(flow, keyMockScenarios)
	if !ok {
		return
	}
	for _, scenario := range sortedKeys(scenarios) {
		steps, ok := asRecord(scenarios[scenario])
		if !ok {
			continue
		}
		for _, stepID := range sortedKeys(steps) {
			mock, ok := asRecord(steps[stepID])
			if !ok {
				continue
			}
			field := fmt.Sprintf("%s.%s.%s.%s", keyMockScenarios, scenario, stepID, keyDelay)
			// Judged by the SOURCE spelling: `delay: 0.0` is the text "0.0" to the
			// reference (no unit, refused) although it decodes to the number 0.
			delay, ok := iss.stringAt(get(mock, keyDelay), field)
			if !ok || delay == "" {
				continue
			}
			if _, err := time.ParseDuration(delay); err == nil {
				continue
			}
			iss.error(Issue{
				Field: field,
				Code:  codeMockDelayInvalid,
				Message: fmt.Sprintf("mock delay '%s' in scenario '%s' step '%s' is not a Go duration",
					delay, scenario, stepID),
				Suggestion: "Write a unit, e.g. \"100ms\" or \"2s\"",
			})
		}
	}
}

// validateOutputParameters refuses an empty-string entry in the flow's
// `output:` list (reference: validateOutputParameters). A YAML null entry is
// NOT refused: measured against the reference, it saves.
func validateOutputParameters(flow doc, iss *issues) {
	list, ok := getSlice(flow, keyOutput)
	if !ok {
		return
	}
	for i, raw := range list {
		if value, isStr := asString(raw); isStr && value == "" {
			iss.error(Issue{
				Field: indexed(keyOutput, i), Code: codeOutputParamEmpty,
				Message: fmt.Sprintf("output entry %d is empty", i),
			})
		}
	}
}
