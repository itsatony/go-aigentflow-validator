package aifvalidate

import (
	"fmt"
	"strings"
	"time"
)

// validateLoopForEachThrottle checks the `loop`, `for_each`, and `throttle`
// blocks. Mirrors validateLoop / validateForEach / validateThrottle (parser.go).
func validateLoopForEachThrottle(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if forEach, ok := getRecord(step, keyForEach); ok {
			validateForEach(step, forEach, stepID, iss)
		}
		if loop, ok := getRecord(step, keyLoop); ok {
			validateLoop(step, loop, stepID, iss)
		}
	}
}

func validateForEach(step, forEach doc, stepID string, iss *issues) {
	base := stepField(stepID, keyForEach)

	if !isNonEmptyString(get(forEach, keyItems)) {
		iss.error(Issue{
			Field: base + "." + keyItems, Code: codeForEachItemsRequired, StepID: stepID,
			Message: "for_each requires a non-empty items expression",
		})
	}

	// for_each and next.parallel are two different fan-out mechanisms; a step may
	// use one.
	if next, ok := getRecord(step, keyNext); ok {
		if _, hasPar := getRecord(next, keyParallel); hasPar {
			iss.error(Issue{
				Field: base, Code: codeForEachMutualExcl, StepID: stepID,
				Message: "A step cannot use both for_each and next.parallel",
			})
		}
	}

	if maxParallel, isInt := asInteger(get(forEach, keyMaxParallel)); isInt && maxParallel < 0 {
		iss.error(Issue{
			Field: base + "." + keyMaxParallel, Code: codeForEachMaxParallel, StepID: stepID,
			Message: fmt.Sprintf("for_each max_parallel must be >= 0, got %d", maxParallel),
		})
	}

	if resolution, ok := getString(forEach, keyResolution); ok && resolution != "" &&
		!forEachResolutions.has(resolution) {
		iss.error(Issue{
			Field: base + "." + keyResolution, Code: codeForEachResolution, StepID: stepID,
			Message:    fmt.Sprintf("Invalid for_each resolution '%s'", resolution),
			Suggestion: "Use one of: " + joinNames(sortedSet(forEachResolutions)),
		})
	}

	if throttle, ok := getRecord(forEach, keyThrottle); ok {
		validateThrottle(throttle, base+"."+keyThrottle, stepID, iss)
	}
}

func validateThrottle(throttle doc, field, stepID string, iss *issues) {
	if delay, ok := getString(throttle, keyDelay); ok && delay != "" {
		checkThrottleDuration(delay, field+"."+keyDelay, stepID,
			maxThrottleDelay, maxThrottleDelayS, codeThrottleDelayMax, "throttle delay", iss)
	}

	batchSize, hasBatchSize := asInteger(get(throttle, keyBatchSize))
	batchDelay, _ := getString(throttle, keyBatchDelay)
	hasBatchDelay := batchDelay != ""

	switch {
	case hasBatchSize && batchSize < 0:
		iss.error(Issue{
			Field: field + "." + keyBatchSize, Code: codeThrottleBatchSize, StepID: stepID,
			Message: fmt.Sprintf("throttle batch_size must be >= 0, got %d", batchSize),
		})
	case (!hasBatchSize || batchSize == 0) && hasBatchDelay:
		iss.error(Issue{
			Field: field + "." + keyBatchDelay, Code: codeThrottleBatchNoSize, StepID: stepID,
			Message: "throttle batch_delay requires a batch_size",
		})
	}

	if hasBatchDelay {
		checkThrottleDuration(batchDelay, field+"."+keyBatchDelay, stepID,
			maxBatchDelay, maxBatchDelayS, codeThrottleBatchDelayMax, "throttle batch_delay", iss)
	}
}

// checkThrottleDuration reports a malformed duration or one over the engine's
// ceiling for that knob.
func checkThrottleDuration(value, field, stepID string, maxNS float64, maxLabel, overCode, label string, iss *issues) {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		iss.error(Issue{
			Field: field, Code: codeInvalidDuration, StepID: stepID,
			Message: fmt.Sprintf("Invalid %s '%s'", strings.TrimPrefix(label, "throttle "), value),
		})
		return
	}
	if float64(parsed.Nanoseconds()) > maxNS {
		iss.error(Issue{
			Field: field, Code: overCode, StepID: stepID,
			Message: fmt.Sprintf("%s '%s' exceeds the %s maximum", label, value, maxLabel),
		})
	}
}

func validateLoop(step, loop doc, stepID string, iss *issues) {
	base := stepField(stepID, keyLoop)

	if !isNonEmptyString(get(loop, keyWhile)) {
		iss.error(Issue{
			Field: base + "." + keyWhile, Code: codeLoopWhileRequired, StepID: stepID,
			Message: "loop requires a non-empty while condition",
		})
	}

	maxIter, isInt := asInteger(get(loop, keyMaxIterations))
	switch {
	case !isInt || maxIter <= 0:
		iss.error(Issue{
			Field: base + "." + keyMaxIterations, Code: codeLoopMaxIterRequired, StepID: stepID,
			Message: "loop requires max_iterations > 0",
		})
	case maxIter > spec.LoopMaxIterationsLimit:
		iss.error(Issue{
			Field: base + "." + keyMaxIterations, Code: codeLoopMaxIterRange, StepID: stepID,
			Message: fmt.Sprintf("loop max_iterations (%d) exceeds the limit of %d",
				maxIter, spec.LoopMaxIterationsLimit),
		})
	}

	subs, hasSubs := getSlice(loop, keySteps)
	if !hasSubs || len(subs) == 0 {
		iss.error(Issue{
			Field: base + "." + keySteps, Code: codeLoopStepsRequired, StepID: stepID,
			Message: "loop requires at least one sub-step",
		})
	} else {
		seen := make(map[string]struct{}, len(subs))
		for i, raw := range subs {
			subPath := base + "." + indexed(keySteps, i)
			sub, isMap := asRecord(raw)
			if !isMap {
				iss.error(Issue{
					Field: subPath, Code: codeInvalidType, StepID: stepID,
					Message: "loop sub-step must be a mapping",
				})
				continue
			}
			id, hasID := getString(sub, keyID)
			if !hasID || id == "" {
				iss.error(Issue{
					Field: subPath + "." + keyID, Code: codeLoopStepIDRequired, StepID: stepID,
					Message: fmt.Sprintf("loop sub-step at index %d requires an 'id'", i),
				})
			} else {
				if strings.Contains(id, reservedStepIDChar) {
					iss.error(Issue{
						Field: subPath + "." + keyID, Code: codeReservedStepID, StepID: stepID,
						Message: fmt.Sprintf("loop sub-step id '%s' must not contain '%s'",
							id, reservedStepIDChar),
					})
				}
				if _, dup := seen[id]; dup {
					iss.error(Issue{
						Field: subPath + "." + keyID, Code: codeLoopStepIDDuplicate, StepID: stepID,
						Message: fmt.Sprintf("Duplicate loop sub-step id '%s'", id),
					})
				}
				seen[id] = struct{}{}
			}
			if !isNonEmptyString(get(sub, keyExecutor)) {
				iss.error(Issue{
					Field: subPath + "." + keyExecutor, Code: codeLoopStepExecRequired, StepID: stepID,
					Message: fmt.Sprintf("loop sub-step at index %d requires an 'executor'", i),
				})
			}
		}
	}

	if present(step, keyForEach) {
		iss.error(Issue{
			Field: base, Code: codeLoopMutualExclForEach, StepID: stepID,
			Message: "A step cannot use both loop and for_each",
		})
	}
	if isNonEmptyString(get(step, keyExecutor)) {
		iss.error(Issue{
			Field: stepField(stepID, keyExecutor), Code: codeLoopMutualExclExec, StepID: stepID,
			Message: "A loop step must not define its own executor (it defines sub-steps)",
		})
	}
}
