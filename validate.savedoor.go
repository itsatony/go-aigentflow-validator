package aifvalidate

import (
	"fmt"
	"time"
)

// Save-door refusals the reference applies in FlowParser.ValidateFlow that the
// JS port does not carry yet. Each is a pure function of the document, so it is
// portable; each is an ERROR because AIgentFlow refuses the flow at save. Owed
// upstream to the JS port (see PARITY.md, "Ahead of the JS port").

// toolDiscoveryModes is the closed tool_discovery vocabulary (reference:
// IsValidDiscoveryMode). An empty value means "unset" and is legal everywhere.
var toolDiscoveryModes = newSet([]string{"eager", "lazy", "off"})

// validateSaveDoorExtras runs the rules above.
func validateSaveDoorExtras(flow doc, iss *issues) {
	validateToolDiscovery(flow, iss)
	validateMockScenarioDelays(flow, iss)
	validateOutputParameters(flow, iss)
}

// validateToolDiscovery refuses an unrecognised tool_discovery value on any of
// its three surfaces: the flow root, the orchestrator block, and a step's
// `query` (reference: validateToolDiscoveryVocabulary, AIF DC-FORGE-48).
//
// The value decides TOOL EXPOSURE, and every consuming switch treats an unknown
// value as eager — so `tool_discovery: "of"` (a typo for off) injected every
// tool. A templated value is skipped: it is rendered at run time. On a step the
// surface is a free-form query key, so only a string is judged there.
func validateToolDiscovery(flow doc, iss *issues) {
	check := func(value, field, stepID string) {
		if value == "" || isTemplate(value) || toolDiscoveryModes.has(value) {
			return
		}
		iss.error(Issue{
			Field: field, Code: codeToolDiscoveryInvalid, StepID: stepID,
			Message:    fmt.Sprintf("tool_discovery '%s' is not a recognised mode", value),
			Suggestion: "Use one of: " + joinNames(sortedSet(toolDiscoveryModes)),
		})
	}
	if value, ok := scalarText(get(flow, keyToolDiscovery)); ok {
		check(value, keyToolDiscovery, "")
	}
	if orch, ok := getRecord(flow, keyOrchestrator); ok {
		if value, ok := scalarText(get(orch, keyToolDiscovery)); ok {
			check(value, keyOrchestrator+"."+keyToolDiscovery, "")
		}
	}
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		query, ok := getRecord(step, keyQuery)
		if !ok {
			continue
		}
		if value, ok := getString(query, keyToolDiscovery); ok {
			check(value, stepField(stepID, keyQuery, keyToolDiscovery), stepID)
		}
	}
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
			delay, ok := scalarText(get(mock, keyDelay))
			if !ok || delay == "" {
				continue
			}
			if _, err := time.ParseDuration(delay); err == nil {
				continue
			}
			iss.error(Issue{
				Field: fmt.Sprintf("%s.%s.%s.%s", keyMockScenarios, scenario, stepID, keyDelay),
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
