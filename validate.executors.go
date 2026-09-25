package aifvalidate

import (
	"fmt"
	"strconv"
	"strings"
)

// validateExecutors checks executor URIs.
//
// Since AIgentFlow v2.598.0 (DC-FORGE-30) the reference applies its ONE
// executor-URL parser (URL_PATTERN_REGEX) at save time, and since v2.608.0
// (DC-FORGE-38) to loop sub-step executors too. The shape check here is that
// same regex, vendored verbatim in the spec:
//   - an unparsable URL is an ERROR: AIgentFlow refuses it at save;
//   - an unknown scheme is a WARNING: the vendored list can lag the registry,
//     and the reference never rejects on scheme.
//
// TEMPLATED URLs ARE SKIPPED, as the reference skips them: the engine renders
// step.executor as a template before dispatch, so
// `flow://stored/{{ .query.child_flow_id }}` cannot be judged statically.
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
	scheme := ""
	if sepIdx > 0 {
		scheme = executor[:sepIdx]
	}

	// A templated URL is rendered by the engine before dispatch, so its shape
	// cannot be judged here. The scheme is still checked when the scheme itself
	// carries no template, because that half is knowable.
	templated := strings.Contains(executor, templateOpenDelim)

	if !templated && !executorURLRe.MatchString(executor) {
		iss.error(Issue{
			Field: field, Code: codeInvalidExecutorURL, StepID: stepID,
			Message: fmt.Sprintf("Executor '%s' is not a usable executor URL", executor),
			Suggestion: "Use the form 'scheme://authority/path' with a non-empty authority " +
				"(e.g., 'ai://openai/chat'). Authority and path accept letters, digits, underscore and hyphen only.",
		})
		return
	}

	if scheme == "" || strings.Contains(scheme, templateOpenDelim) {
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
