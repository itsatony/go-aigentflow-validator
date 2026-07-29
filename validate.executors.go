package aifvalidate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// schemePattern is the identifier grammar for an executor URI scheme.
var schemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-_]*$`)

// validateExecutors checks executor URIs.
//
// The Go reference's static validator does NOT reject executor URIs — the live
// executor registry is the source of truth and schemes are added frequently.
// This module adds two authoring-time aids, classified conservatively:
//   - a malformed URI (no scheme://path) is an ERROR: always genuinely broken;
//   - an unknown scheme is a WARNING: the vendored list can lag the registry.
//
// See PARITY.md.
func validateExecutors(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if executor, isStr := getString(step, keyExecutor); isStr && executor != "" {
			checkExecutor(executor, stepField(stepID, keyExecutor), stepID, iss)
		}

		// Loop sub-steps each carry their own executor.
		loop, hasLoop := getRecord(step, keyLoop)
		if !hasLoop {
			continue
		}
		subs, hasSubs := getSlice(loop, keySteps)
		if !hasSubs {
			continue
		}
		for i, raw := range subs {
			sub, isMap := asRecord(raw)
			if !isMap {
				continue
			}
			executor, isStr := getString(sub, keyExecutor)
			if !isStr || executor == "" {
				continue
			}
			checkExecutor(executor,
				stepField(stepID, keyLoop, indexed(keySteps, i), keyExecutor),
				subStepID(stepID, sub, i), iss)
		}
	}
}

// subStepID composes the engine's composite loop sub-step ID ("parent.child"),
// falling back to the index when the sub-step declares no id.
func subStepID(stepID string, sub doc, i int) string {
	if id, ok := getString(sub, keyID); ok && id != "" {
		return stepID + reservedStepIDChar + id
	}
	return stepID + reservedStepIDChar + strconv.Itoa(i)
}

func checkExecutor(executor, field, stepID string, iss *issues) {
	sepIdx := strings.Index(executor, schemeSeparator)
	if sepIdx <= 0 {
		iss.error(Issue{
			Field: field, Code: codeInvalidExecutorURL, StepID: stepID,
			Message:    fmt.Sprintf("Executor '%s' is not a valid '<scheme>://<path>' URI", executor),
			Suggestion: "Use the form 'scheme://path' (e.g., 'ai://openai/chat')",
		})
		return
	}
	scheme := executor[:sepIdx]
	path := executor[sepIdx+len(schemeSeparator):]

	if !schemePattern.MatchString(scheme) {
		iss.error(Issue{
			Field: field, Code: codeInvalidExecutorURL, StepID: stepID,
			Message: fmt.Sprintf("Executor scheme '%s' is not a valid identifier", scheme),
		})
		return
	}
	if path == "" {
		iss.error(Issue{
			Field: field, Code: codeInvalidExecutorURL, StepID: stepID,
			Message: fmt.Sprintf("Executor '%s' has an empty path after '%s'", executor, schemeSeparator),
		})
		return
	}
	if !executorSchemes.has(strings.ToLower(scheme)) {
		iss.warn(Issue{
			Field: field, Code: codeUnknownExecScheme, StepID: stepID,
			Message: fmt.Sprintf(
				"Unknown executor scheme '%s'. It may be valid in a newer AIgentFlow release, or it could be a typo.",
				scheme),
		})
	}
}
