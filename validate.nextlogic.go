package aifvalidate

import "fmt"

// validateNextLogic checks the `next.parallel` block and the orchestrator-next
// rule.
//
// Default/condition reference existence lives in validateConnectivity (it pairs
// with reachability). This module covers the parser-only additions:
// validateNextLogic's parallel block + validateOrchestratorNext.
func validateNextLogic(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	_, hasOrchestrator := getRecord(flow, keyOrchestrator)

	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		next, hasNext := getRecord(step, keyNext)
		if !hasNext {
			continue
		}

		if par, hasPar := getRecord(next, keyParallel); hasPar {
			validateParallel(par, steps, stepID, iss)
		}

		// Routing to "orchestrator" requires the flow to declare one.
		if !hasOrchestrator && routesToOrchestrator(next) {
			iss.error(Issue{
				Field: stepField(stepID, keyNext), Code: codeOrchNextRequiresOrch, StepID: stepID,
				Message: fmt.Sprintf(
					"Step '%s' routes to 'orchestrator' but the flow has no orchestrator block", stepID),
			})
		}
	}
}

func validateParallel(par, steps doc, stepID string, iss *issues) {
	base := stepField(stepID, keyNext, keyParallel)

	rendezvous, hasRendezvous := getString(par, keyRendezvous)
	switch {
	case !hasRendezvous || rendezvous == "":
		iss.error(Issue{
			Field: base + "." + keyRendezvous, Code: codeMissingField, StepID: stepID,
			Message: "parallel block requires a rendezvous step",
		})
	case rendezvous != nextMarkerNull && !has(steps, rendezvous):
		iss.error(Issue{
			Field: base + "." + keyRendezvous, Code: codeStepNotFound, StepID: stepID,
			Message: fmt.Sprintf("Referenced rendezvous step '%s' does not exist", rendezvous),
		})
	}

	members, hasMembers := getSlice(par, keySteps)
	if !hasMembers || len(members) == 0 {
		iss.error(Issue{
			Field: base + "." + keySteps, Code: codeMissingField, StepID: stepID,
			Message: "parallel block requires at least one step",
		})
		return
	}
	for i, raw := range members {
		member, isStr := asString(raw)
		if isStr && has(steps, member) {
			continue
		}
		iss.error(Issue{
			Field: fmt.Sprintf("%s.%s", base, indexed(keySteps, i)),
			Code:  codeStepNotFound, StepID: stepID,
			Message: fmt.Sprintf("Referenced parallel step '%v' does not exist", raw),
		})
	}
}

// routesToOrchestrator reports whether a next block targets the orchestrator via
// its default or any conditional goto.
func routesToOrchestrator(next doc) bool {
	if target, ok := getString(next, keyDefault); ok && target == nextMarkerOrch {
		return true
	}
	conds, ok := getSlice(next, keyConditions)
	if !ok {
		return false
	}
	for _, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		if target, ok := getString(cond, keyGotoStep); ok && target == nextMarkerOrch {
			return true
		}
	}
	return false
}
