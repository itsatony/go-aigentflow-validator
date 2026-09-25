package aifvalidate

import (
	"fmt"
	"strconv"
	"time"
)

// stepMaxDurationNoBound are the reference's no-bound spellings for a step
// max_duration (shared with its async/HITL park expiry).
var stepMaxDurationNoBound = newSet([]string{"none", "never", "infinite"})

// validateStepMaxDuration warns on a step `max_duration:` the engine cannot
// turn into a deadline (step_max_duration_ignored; reference:
// validateStepMaxDurationIsApplied, AIF DC-FORGE-147).
//
// The engine bounds each executor invocation of a step by its max_duration. Two
// declarations fall outside that, both WARNINGS:
//   - a value that does not parse as a Go duration: the engine runs the step
//     with no time limit rather than fail a stored flow at its run door;
//   - a value on a `loop:` step, which makes no executor call of its own.
//
// Loop sub-steps have no max_duration field in the reference, so there is no
// second step table to walk. "0s" and negative values parse (the engine treats
// them as no bound) and do not warn.
func validateStepMaxDuration(flow doc, iss *issues) {
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		// The reference's field is a Go string; any YAML scalar decodes into it,
		// so `max_duration: 90` arrives as "90" and fails to parse.
		declared, ok := scalarText(get(step, keyMaxDuration))
		if !ok || declared == "" {
			continue
		}
		field := stepField(stepID, keyMaxDuration)
		if !isApplicableStepMaxDuration(declared) {
			iss.warn(Issue{
				Field: field, Code: codeStepMaxDurationIgnored,
				Message: fmt.Sprintf("max_duration %s on step %s is not a duration, so the step runs with no time "+
					"limit. Use a Go duration such as \"90s\", \"5m\" or \"2h\" (there is no day unit: write \"48h\", "+
					"not \"2d\"; templates are not rendered here), or \"none\" for no limit.",
					strconv.Quote(declared), strconv.Quote(stepID)),
			})
			continue
		}
		if _, isLoop := getRecord(step, keyLoop); isLoop {
			iss.warn(Issue{
				Field: field, Code: codeStepMaxDurationIgnored,
				Message: fmt.Sprintf("max_duration %s on loop step %s bounds nothing: a loop step makes no executor "+
					"call of its own, and loop sub-steps have no max_duration key. Bound the loop with "+
					"loop.max_iterations, or remove max_duration.", strconv.Quote(declared), strconv.Quote(stepID)),
			})
		}
	}
}

// isApplicableStepMaxDuration is the reference's parseStepMaxDuration: a
// no-bound spelling or a time.ParseDuration string.
func isApplicableStepMaxDuration(raw string) bool {
	if stepMaxDurationNoBound.has(raw) {
		return true
	}
	_, err := time.ParseDuration(raw)
	return err == nil
}
