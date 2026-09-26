package aifvalidate

import (
	"fmt"
	"sort"
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
	// Both delays are Go `string` fields, which yaml.v3 fills from ANY scalar by
	// its source text: `delay: 100` is "100" (no unit, refused), `0` is "0"
	// (saves) and `0.0` is "0.0" (refused). A number used to be skipped here.
	if delay, ok := scalarTextAt(get(throttle, keyDelay), field+"."+keyDelay, iss.sources); ok && delay != "" {
		checkThrottleDuration(delay, field+"."+keyDelay, stepID,
			maxThrottleDelay, maxThrottleDelayS, codeThrottleDelayMax, "throttle delay", iss)
	}

	batchSize, hasBatchSize := asInteger(get(throttle, keyBatchSize))
	batchDelay, _ := scalarTextAt(get(throttle, keyBatchDelay), field+"."+keyBatchDelay, iss.sources)
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
				// A WARNING in the reference: the sub-step still runs, and its
				// result stays reachable through the flat `index .data "<loop>.<sub>"`
				// key. Reported on loop.steps, naming the parent step, as upstream.
				if loopResultSummaryFields.has(id) {
					iss.warn(Issue{
						Field: base + "." + keySteps, Code: codeLoopSubStepIDReserved, StepID: stepID,
						Message: fmt.Sprintf("loop sub-step id '%s' is also a field of the loop's own result, "+
							"so {{ .data.%s.%s }} reads the loop's %s instead of this sub-step's output",
							id, stepID, id, id),
						Suggestion: "rename the sub-step; ids reserved by the loop result: " +
							joinNames(sortedSet(loopResultSummaryFields)),
					})
				}
			}
			if !isNonEmptyString(get(sub, keyExecutor)) {
				iss.error(Issue{
					Field: subPath + "." + keyExecutor, Code: codeLoopStepExecRequired, StepID: stepID,
					Message: fmt.Sprintf("loop sub-step at index %d requires an 'executor'", i),
				})
			}
		}

		// Second pass, deliberately: a FORWARD jump is legal, so the whole id
		// set has to exist before any target can be judged.
		for i, raw := range subs {
			if sub, isMap := asRecord(raw); isMap {
				validateLoopSubStepNext(sub, seen, base, i, stepID, iss)
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

// loopResultSummaryFields are the loop result's own summary fields (reference:
// LoopResultSummaryFields, AIF v2.648.0 / DC-FORGE-78). The loop step's
// committed result carries these AND one entry per sub-step under its id, so a
// sub-step named `vars` is unreadable through {{ .data.<loop>.vars }}.
var loopResultSummaryFields = newSet([]string{"iterations", "break", "vars", "duration_ms"})

// loopSubStepNextSentinels are the two top-level next: sentinels, which carry no
// meaning inside a loop body: the reference's resolveLoopSubStepNext has no
// sentinel awareness, so both take the same miss path as a typo.
//
// `end` is deliberately NOT listed. The reference refuses only these two by
// name and lets everything else fall through to the existence check, so `end`
// is reported as a missing target. Divergence #2 ("end is terminal") is about
// TOP-LEVEL next targets and is not extended into a loop body.
var loopSubStepNextSentinels = newSet([]string{nextMarkerNull, nextMarkerOrch})

// validateLoopSubStepNext checks one loop sub-step's `next:` block against the
// LOOP's own sub-step table and nothing else (reference: validateLoopSubStepNext,
// parser.go, AIF v2.672.0 / DC-FORGE-102).
//
// A loop body is a SECOND step table: validateNextLogic and connectivity walk
// flow.steps only, so before this rule a sub-step could route to `pol` when the
// sub-step is called `poll`, and the loop driver silently advanced
// sequentially. Both jump directions are legal, and an EMPTY target is the
// documented "advance sequentially".
func validateLoopSubStepNext(sub doc, subStepIDs map[string]struct{}, base string, index int, stepID string, iss *issues) {
	raw := get(sub, keyNext)
	if raw == nil {
		return
	}
	path := fmt.Sprintf("%s.%s.%s", base, indexed(keySteps, index), keyNext)
	subStepID, ok := getString(sub, keyID)
	if !ok {
		subStepID = fmt.Sprintf("[%d]", index)
	}
	next, isMap := asRecord(raw)
	if !isMap {
		iss.error(Issue{
			Field: path, Code: codeInvalidType, StepID: stepID,
			Message: fmt.Sprintf("loop sub-step '%s' next must be a mapping", subStepID),
		})
		return
	}

	// The loop driver reads conditions and default only, so a parallel block
	// never fans out. The reference returns on this one without looking at the
	// targets; so does this port.
	if _, hasPar := getRecord(next, keyParallel); hasPar {
		iss.error(Issue{
			Field: path + "." + keyParallel, Code: codeLoopSubstepNextParallel, StepID: stepID,
			Message: fmt.Sprintf("loop sub-step '%s' declares next.parallel, which a loop body does not "+
				"support (sub-steps run sequentially)", subStepID),
			Suggestion: "Use next.conditions/default to branch within the iteration, or a top-level step for parallel fan-out",
		})
		return
	}

	type target struct {
		field string
		value any
	}
	targets := []target{{field: path + "." + keyDefault, value: get(next, keyDefault)}}
	if conds, ok := getSlice(next, keyConditions); ok {
		for j, rawCond := range conds {
			cond, isCond := asRecord(rawCond)
			if !isCond {
				continue
			}
			targets = append(targets, target{
				field: fmt.Sprintf("%s.%s.%s", path, indexed(keyConditions, j), keyGoto),
				value: get(cond, keyGoto),
			})
		}
	}

	available := make([]string, 0, len(subStepIDs))
	for id := range subStepIDs {
		available = append(available, id)
	}
	sort.Strings(available)

	for _, t := range targets {
		if t.value == nil {
			continue
		}
		value, isStr := asString(t.value)
		if !isStr {
			iss.error(Issue{
				Field: t.field, Code: codeInvalidType, StepID: stepID,
				Message: fmt.Sprintf("loop sub-step '%s' next target must be a string", subStepID),
			})
			continue
		}
		if value == "" {
			continue // the documented sequential advance
		}
		if loopSubStepNextSentinels.has(value) {
			iss.error(Issue{
				Field: t.field, Code: codeLoopSubstepNextSentinel, StepID: stepID,
				Message: fmt.Sprintf("loop sub-step '%s' routes next to the reserved marker '%s', which has "+
					"no meaning inside a loop body", subStepID, value),
				Suggestion: "Name another sub-step of the same loop, or leave the target empty for sequential advance",
			})
			continue
		}
		if _, ok := subStepIDs[value]; ok {
			continue
		}
		suggestion := "A sub-step's next: resolves only against loop.steps of the SAME loop"
		if len(available) > 0 {
			suggestion += ". Available: " + joinNames(available)
		}
		iss.error(Issue{
			Field: t.field, Code: codeLoopSubstepNextNotFound, StepID: stepID,
			Message: fmt.Sprintf("loop sub-step '%s' routes next to '%s', which is not a sub-step of that loop",
				subStepID, value),
			Suggestion: suggestion,
		})
	}
}
