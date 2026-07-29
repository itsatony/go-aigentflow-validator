package aifvalidate

import "fmt"

// validateConnectivity checks `next` reference existence (errors), unreachable
// steps (warnings), and cycles (warning).
//
// Mirrors validateStepConnectivity + checkForCycles + findReachableSteps
// (validation.go). Reachability and cycles follow ONLY next.default and
// next.conditions[].goto_step, exempting the terminal markers null / end /
// orchestrator — exactly as the reference does.
func validateConnectivity(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	names := stepNames(steps)

	// Reference existence.
	for _, stepID := range names {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		next, hasNext := getRecord(step, keyNext)
		if !hasNext {
			continue
		}

		if target, ok := getString(next, keyDefault); ok && target != "" &&
			!isNextMarker(target) && !has(steps, target) {
			iss.error(missingStepIssue(
				stepField(stepID, keyNext, keyDefault), stepID, target, names))
		}

		conds, hasConds := getSlice(next, keyConditions)
		if !hasConds {
			continue
		}
		for i, raw := range conds {
			cond, isMap := asRecord(raw)
			if !isMap {
				continue
			}
			target, ok := getString(cond, keyGotoStep)
			if !ok || target == "" || isNextMarker(target) || has(steps, target) {
				continue
			}
			// Field path mirrors the JS implementation's `…conditions[i].goto`
			// (not `.goto_step`) — the parity contract is the code, but keeping the
			// path identical means one consumer can highlight either's findings.
			iss.error(missingStepIssue(
				stepField(stepID, keyNext, indexed(keyConditions, i), "goto"), stepID, target, names))
		}
	}

	start, _ := getString(flow, keyStart)
	if start == "" || !has(steps, start) {
		// Without a valid start, reachability and cycle analysis are meaningless,
		// and the missing/unknown start is already reported by basicStructure.
		return
	}

	// Reachability (BFS from start).
	reachable := make(map[string]struct{}, len(steps))
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := reachable[current]; seen {
			continue
		}
		reachable[current] = struct{}{}
		for _, target := range gotoTargets(steps, current) {
			if _, seen := reachable[target]; !seen {
				queue = append(queue, target)
			}
		}
	}
	for _, stepID := range names {
		if _, ok := reachable[stepID]; !ok {
			iss.warn(Issue{
				Field: stepField(stepID), Code: codeUnreachableStep, StepID: stepID,
				Message:    fmt.Sprintf("Step '%s' is not reachable from start step", stepID),
				Suggestion: "Add a path to this step or remove it if not needed",
			})
		}
	}

	// Cycle detection (DFS with a recursion stack).
	visited := make(map[string]struct{}, len(steps))
	recStack := make(map[string]struct{}, len(steps))
	if hasCycle(steps, start, visited, recStack) {
		iss.warn(Issue{
			Field: keySteps, Code: codePotentialInfiniteLop,
			Message:    "Potential infinite loop detected in step flow",
			Suggestion: "Review your step transitions to ensure they eventually terminate",
		})
	}
}

// isNextMarker reports whether a `next` target is a terminal marker
// (null / end / orchestrator) rather than a step reference.
func isNextMarker(target string) bool { return nextMarkers.has(target) }

// gotoTargets returns the step IDs a step routes to, excluding terminal markers.
func gotoTargets(steps doc, stepID string) []string {
	step, ok := asRecord(steps[stepID])
	if !ok {
		return nil
	}
	next, ok := getRecord(step, keyNext)
	if !ok {
		return nil
	}
	var out []string
	if target, ok := getString(next, keyDefault); ok && target != "" && !isNextMarker(target) {
		out = append(out, target)
	}
	conds, ok := getSlice(next, keyConditions)
	if !ok {
		return out
	}
	for _, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		if target, ok := getString(cond, keyGotoStep); ok && target != "" && !isNextMarker(target) {
			out = append(out, target)
		}
	}
	return out
}

func hasCycle(steps doc, stepID string, visited, recStack map[string]struct{}) bool {
	if _, onStack := recStack[stepID]; onStack {
		return true
	}
	if _, seen := visited[stepID]; seen {
		return false
	}
	visited[stepID] = struct{}{}
	recStack[stepID] = struct{}{}
	for _, target := range gotoTargets(steps, stepID) {
		if hasCycle(steps, target, visited, recStack) {
			return true
		}
	}
	delete(recStack, stepID)
	return false
}

// missingStepIssue builds the shared "referenced step does not exist" finding.
func missingStepIssue(field, stepID, target string, names []string) Issue {
	return Issue{
		Field: field, Code: codeStepNotFound, StepID: stepID,
		Message:    fmt.Sprintf("Referenced step '%s' does not exist", target),
		Context:    "Available steps: " + joinNames(names),
		Suggestion: "Change to one of: " + joinNames(names),
	}
}
