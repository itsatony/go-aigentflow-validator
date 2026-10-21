package aifvalidate

import "fmt"

// validateNextLogic checks the `next.parallel` block.
//
// Default/condition reference existence lives in validateConnectivity (it pairs
// with reachability). This module covers the parser-only addition:
// validateNextLogic's parallel block. (`next: orchestrator` names the removed
// hand-over and is refused as orchestrator_removed; see
// validateRemovedOrchestrator.)
func validateNextLogic(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
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
	}
}

func validateParallel(par, steps doc, stepID string, iss *issues) {
	base := stepField(stepID, keyNext, keyParallel)

	rendezvous, hasRendezvous := iss.stringOf(par, keyRendezvous, base)
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
		memberPath := fmt.Sprintf("%s.%s", base, indexed(keySteps, i))
		member, isStr := iss.stringAt(raw, memberPath)
		if isStr && has(steps, member) {
			continue
		}
		iss.error(Issue{
			Field: memberPath,
			Code:  codeStepNotFound, StepID: stepID,
			Message: fmt.Sprintf("Referenced parallel step '%v' does not exist", raw),
		})
	}
}
