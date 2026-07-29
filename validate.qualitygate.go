package aifvalidate

import "fmt"

// validateQualityGates checks each step's `quality_gate:` block (DC-CP-8).
//
// Mirrors FlowParser.validateQualityGate (parser.go): rubric non-empty;
// threshold in [0,1]; on_fail ∈ {fail,goto,retry} — the Go enum also lists
// `human`, but it is validation-REJECTED pending the human-task inbox, so it is
// rejected here too, under its own code; when on_fail=goto, goto_step is
// required, must name an existing step, and must not be the gate's own step (a
// failing self-goto loops forever). The Go layer additionally refuses a gate on a
// composite step (loop/for_each/parallel) or on a member of another step's
// parallel fan-out, because gate routing is only wired into the sequential
// driver; both static guards are ported.
func validateQualityGates(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	members := parallelMembers(steps)

	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		gate, hasGate := getRecord(step, keyQualityGate)
		if !hasGate {
			continue
		}
		validateQualityGate(gate, step, steps, members, stepID, iss)
	}
}

// parallelMembers maps a step ID to the step that fans out to it via
// next.parallel.steps.
func parallelMembers(steps doc) map[string]string {
	members := make(map[string]string)
	for _, ownerID := range sortedKeys(steps) {
		step, ok := asRecord(steps[ownerID])
		if !ok {
			continue
		}
		next, ok := getRecord(step, keyNext)
		if !ok {
			continue
		}
		par, ok := getRecord(next, keyParallel)
		if !ok {
			continue
		}
		list, ok := getSlice(par, keySteps)
		if !ok {
			continue
		}
		for _, raw := range list {
			memberID, isStr := asString(raw)
			if !isStr {
				continue
			}
			if _, already := members[memberID]; !already {
				members[memberID] = ownerID
			}
		}
	}
	return members
}

func validateQualityGate(gate, step, steps doc, members map[string]string, stepID string, iss *issues) {
	field := stepField(stepID, keyQualityGate)

	if !isNonEmptyString(trimmed(get(gate, keyRubric))) {
		iss.error(Issue{
			Field: field + "." + keyRubric, Code: codeQGMissingRubric, StepID: stepID,
			Message: fmt.Sprintf("quality_gate on step '%s' requires a non-empty 'rubric'", stepID),
		})
	}

	if has(gate, keyThreshold) {
		threshold, isNum := asNumber(get(gate, keyThreshold))
		if !isNum || threshold < spec.QualityGate.ThresholdMin || threshold > spec.QualityGate.ThresholdMax {
			iss.error(Issue{
				Field: field + "." + keyThreshold, Code: codeQGThresholdRange, StepID: stepID,
				Message: fmt.Sprintf("quality_gate on step '%s' has threshold %v out of range [%v,%v]",
					stepID, get(gate, keyThreshold),
					spec.QualityGate.ThresholdMin, spec.QualityGate.ThresholdMax),
			})
		}
	}

	onFail, hasOnFail := getString(gate, keyOnFail)
	if hasOnFail && onFail != "" {
		switch {
		case qualityGateRejected.has(onFail):
			// In the Go enum but not yet supported (e.g. `human`).
			iss.error(Issue{
				Field: field + "." + keyOnFail, Code: codeQGOnFailUnsupported, StepID: stepID,
				Message: fmt.Sprintf(
					"quality_gate on step '%s' uses on_fail='%s', which is not yet supported; use one of: %s",
					stepID, onFail, joinNames(sortedSet(qualityGateOnFail))),
			})
		case !qualityGateOnFail.has(onFail):
			iss.error(Issue{
				Field: field + "." + keyOnFail, Code: codeQGInvalidOnFail, StepID: stepID,
				Message:    fmt.Sprintf("quality_gate on step '%s' has invalid on_fail '%s'", stepID, onFail),
				Suggestion: "Use one of: " + joinNames(sortedSet(qualityGateOnFail)),
			})
		}
	}

	if onFail == actionGoto {
		target, hasTarget := getString(gate, keyGotoStep)
		switch {
		case !hasTarget || target == "":
			iss.error(Issue{
				Field: field + "." + keyGotoStep, Code: codeQGGotoMissing, StepID: stepID,
				Message: fmt.Sprintf("quality_gate on step '%s' uses on_fail=goto but goto_step is empty", stepID),
			})
		case !has(steps, target):
			iss.error(Issue{
				Field: field + "." + keyGotoStep, Code: codeStepNotFound, StepID: stepID,
				Message: fmt.Sprintf("quality_gate on step '%s' goto_step '%s' does not name an existing step",
					stepID, target),
			})
		case target == stepID:
			iss.error(Issue{
				Field: field + "." + keyGotoStep, Code: codeQGGotoSelf, StepID: stepID,
				Message: fmt.Sprintf(
					"quality_gate on step '%s' uses on_fail=goto pointing at itself; a failing self-goto loops forever (use on_fail=retry with max_retries)",
					stepID),
			})
		}
	}

	if isCompositeStep(step) {
		iss.error(Issue{
			Field: field, Code: codeQGOnComposite, StepID: stepID,
			Message: fmt.Sprintf(
				"quality_gate on step '%s' is not supported on loop/for_each/parallel steps", stepID),
		})
	}
	if owner, isMember := members[stepID]; isMember {
		iss.error(Issue{
			Field: field, Code: codeQGOnParallelMember, StepID: stepID,
			Message: fmt.Sprintf(
				"quality_gate on step '%s' is not supported because it is a parallel member of step '%s'",
				stepID, owner),
		})
	}
}

// isCompositeStep reports whether a step fans out or iterates, which is where
// quality-gate routing is not wired.
func isCompositeStep(step doc) bool {
	if _, ok := getRecord(step, keyLoop); ok {
		return true
	}
	if _, ok := getRecord(step, keyForEach); ok {
		return true
	}
	if next, ok := getRecord(step, keyNext); ok {
		if _, hasPar := getRecord(next, keyParallel); hasPar {
			return true
		}
	}
	return false
}
