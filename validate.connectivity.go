package aifvalidate

import "fmt"

// validateConnectivity checks `next` reference existence (errors), unreachable
// steps (warnings), and cycles (warning).
//
// Mirrors validateStepConnectivity + checkForCycles + findReachableSteps
// (AIgentFlow validation.go) and aigentflow-flow-validator-js connectivity.ts.
//
// ⚠ REACHABILITY AND CYCLES FOLLOW DIFFERENT EDGE SETS, faithfully. AIgentFlow
// v2.598.0 (DC-FORGE-30) widened reachability to the five edge kinds —
// next.default, next.conditions[].goto, next.parallel.steps[],
// next.parallel.rendezvous and the step-level error_strategy.goto_step — plus
// the FLOW-level error_strategy.goto_step seeded into the queue. The cycle walk
// still follows next.default + next.conditions[].goto only: a rendezvous or an
// error redirect back to an earlier step is an ordinary shape.
//
// ⛔ A condition's target key is `goto`, NOT `goto_step` (v0.2.0). This port
// read `goto_step` (the Go struct FIELD name, not its yaml tag), so every
// condition branch looked unreachable — and a flow using `goto_step` there,
// which AIgentFlow refuses outright (KnownFields), was accepted. Together with
// the narrow walk, a parallel fan-out and every conditional branch of a valid
// flow were reported "not reachable from start step" (aigentflow#149).
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
			if _, wrongKey := getString(cond, keyGotoStep); wrongKey {
				iss.error(Issue{
					Field:      stepField(stepID, keyNext, indexed(keyConditions, i), keyGotoStep),
					Code:       codeUnknownYAMLKey,
					StepID:     stepID,
					Message:    "A condition's branch target key is 'goto', not 'goto_step'",
					Suggestion: "Rename 'goto_step' to 'goto' (only error_strategy and quality_gate use 'goto_step')",
				})
			}
			target, ok := getString(cond, keyGoto)
			if !ok || target == "" || isNextMarker(target) || has(steps, target) {
				continue
			}
			iss.error(missingStepIssue(
				stepField(stepID, keyNext, indexed(keyConditions, i), keyGoto), stepID, target, names))
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
	// The FLOW-level error strategy names a step nothing else points at; it
	// belongs to the flow, so AIgentFlow seeds it into the walk.
	if fes, ok := getRecord(flow, keyErrorStrategy); ok {
		if target, ok := getString(fes, keyGotoStep); ok && target != "" && !isNextMarker(target) {
			queue = append(queue, target)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := reachable[current]; seen {
			continue
		}
		reachable[current] = struct{}{}
		for _, target := range reachTargets(steps, current) {
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

// cycleTargets returns the edges the CYCLE detector follows — next.default and
// next.conditions[].goto only (see the header).
func cycleTargets(steps doc, stepID string) []string {
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
		if target, ok := getString(cond, keyGoto); ok && target != "" && !isNextMarker(target) {
			out = append(out, target)
		}
	}
	return out
}

// reachTargets returns the edges REACHABILITY follows — all five kinds.
// Under-reporting here is wrong in the silent direction: it turns a missing
// edge into a confident accusation.
func reachTargets(steps doc, stepID string) []string {
	out := cycleTargets(steps, stepID)
	step, ok := asRecord(steps[stepID])
	if !ok {
		return out
	}
	push := func(target string, ok bool) {
		if ok && target != "" && !isNextMarker(target) {
			out = append(out, target)
		}
	}
	if next, ok := getRecord(step, keyNext); ok {
		if parallel, ok := getRecord(next, keyParallel); ok {
			if members, ok := getSlice(parallel, keySteps); ok {
				for _, raw := range members {
					id, isStr := raw.(string)
					push(id, isStr)
				}
			}
			push(getString(parallel, keyRendezvous))
		}
	}
	if es, ok := getRecord(step, keyErrorStrategy); ok {
		push(getString(es, keyGotoStep))
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
	for _, target := range cycleTargets(steps, stepID) {
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
