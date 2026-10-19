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

		nextPath := stepField(stepID, keyNext)
		if target, ok := iss.stringOf(next, keyDefault, nextPath); ok && target != "" &&
			!isSaveDoorMarker(target) && !has(steps, target) {
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
			if _, wrongKey := cond[keyGotoStep]; wrongKey {
				iss.error(Issue{
					Field:      stepField(stepID, keyNext, indexed(keyConditions, i), keyGotoStep),
					Code:       codeUnknownYAMLKey,
					StepID:     stepID,
					Message:    "A condition's branch target key is 'goto', not 'goto_step'",
					Suggestion: "Rename 'goto_step' to 'goto' (only error_strategy and quality_gate use 'goto_step')",
				})
			}
			target, ok := iss.stringOf(cond, keyGoto, childPath(nextPath, indexed(keyConditions, i)))
			if !ok || target == "" || isSaveDoorMarker(target) || has(steps, target) {
				continue
			}
			iss.error(missingStepIssue(
				stepField(stepID, keyNext, indexed(keyConditions, i), keyGoto), stepID, target, names))
		}
	}

	start, _ := iss.stringOf(flow, keyStart, "")
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
		if target, ok := iss.stringOf(fes, keyGotoStep, keyErrorStrategy); ok && target != "" && !isNextMarker(target) {
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
		for _, target := range reachTargets(steps, current, iss) {
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

	// Cycle detection: only a cycle with no way out is potentially infinite.
	if cycleWithoutExit(steps, start, iss) {
		iss.warn(Issue{
			Field: keySteps, Code: codePotentialInfiniteLop,
			Message:    "Potential infinite loop detected in step flow: a cycle of steps has no way out",
			Suggestion: "Give one step in the cycle a `next.conditions` branch that leaves it (or end the flow there)",
		})
	}
}

// TWO marker sets, kept apart although they now hold the same values.
//
// Existence (an ERROR) follows the save door, validateNextLogic (parser.go):
// only `null` and `orchestrator` stand without a step of that name, so
// `default: end` is refused when no step is called `end`.
//
// Reachability and cycles (WARNINGS) follow findReachableSteps /
// checkForCycles (validation.go). Until v2.760.0 (DC-FORGE-189) those walks
// also stopped at `end`, so a real step named `end` was reported unreachable.
// The reference now treats `end` as an ordinary step id everywhere, and so does
// this library: `end` left spec reachabilityTerminalMarkers. The two sets stay
// separate keys in the spec so a future divergence is a one-value change.

// isSaveDoorMarker reports whether a `next` target needs no step of that name.
func isSaveDoorMarker(target string) bool { return nextMarkers.has(target) }

// isNextMarker reports whether the reachability and cycle walks stop at target.
func isNextMarker(target string) bool { return reachabilityMarkers.has(target) }

// cycleTargets returns the edges the CYCLE detector follows — next.default and
// next.conditions[].goto only (see the header).
func cycleTargets(steps doc, stepID string, iss *issues) []string {
	step, ok := asRecord(steps[stepID])
	if !ok {
		return nil
	}
	next, ok := getRecord(step, keyNext)
	if !ok {
		return nil
	}
	nextPath := stepField(stepID, keyNext)
	var out []string
	if target, ok := iss.stringOf(next, keyDefault, nextPath); ok && target != "" && !isNextMarker(target) {
		out = append(out, target)
	}
	conds, ok := getSlice(next, keyConditions)
	if !ok {
		return out
	}
	for i, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		if target, ok := iss.stringOf(cond, keyGoto, childPath(nextPath, indexed(keyConditions, i))); ok &&
			target != "" && !isNextMarker(target) {
			out = append(out, target)
		}
	}
	return out
}

// reachTargets returns the edges REACHABILITY follows — all five kinds.
// Under-reporting here is wrong in the silent direction: it turns a missing
// edge into a confident accusation.
func reachTargets(steps doc, stepID string, iss *issues) []string {
	out := cycleTargets(steps, stepID, iss)
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
			parallelPath := stepField(stepID, keyNext, keyParallel)
			if members, ok := getSlice(parallel, keySteps); ok {
				for i, raw := range members {
					push(iss.stringAt(raw, childPath(parallelPath, indexed(keySteps, i))))
				}
			}
			push(iss.stringOf(parallel, keyRendezvous, parallelPath))
		}
	}
	if es, ok := getRecord(step, keyErrorStrategy); ok {
		push(iss.stringOf(es, keyGotoStep, stepField(stepID, keyErrorStrategy)))
	}
	return out
}

// cycleEdges returns every target the cycle analysis follows from a step —
// next.default and next.conditions[].goto — INCLUDING the terminal markers,
// which the exit check needs to see (cycleTargets drops them).
func cycleEdges(steps doc, stepID string, iss *issues) []string {
	step, ok := asRecord(steps[stepID])
	if !ok {
		return nil
	}
	next, ok := getRecord(step, keyNext)
	if !ok {
		return nil
	}
	nextPath := stepField(stepID, keyNext)
	var out []string
	if target, ok := iss.stringOf(next, keyDefault, nextPath); ok && target != "" {
		out = append(out, target)
	}
	conds, _ := getSlice(next, keyConditions)
	for i, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		if target, ok := iss.stringOf(cond, keyGoto, childPath(nextPath, indexed(keyConditions, i))); ok && target != "" {
			out = append(out, target)
		}
	}
	return out
}

// stepLeavesCycle reports whether control can leave the strongly connected set
// from this step: the flow can end here (no next, or no default so the flow
// ends when no condition matches), it fans out (parallel edges are not
// followed, so a fan-out is treated as an exit rather than guessed at), or a
// branch names a terminal marker or a step outside the set.
func stepLeavesCycle(steps doc, stepID string, inCycle map[string]bool, iss *issues) bool {
	step, ok := asRecord(steps[stepID])
	if !ok {
		return true
	}
	next, ok := getRecord(step, keyNext)
	if !ok {
		return true
	}
	if parallel, present := next[keyParallel]; present && parallel != nil {
		return true
	}
	if def, ok := iss.stringOf(next, keyDefault, stepField(stepID, keyNext)); !ok || def == "" {
		return true
	}
	for _, target := range cycleEdges(steps, stepID, iss) {
		if isNextMarker(target) || !inCycle[target] {
			return true
		}
	}
	return false
}

// cycleWithoutExit reports whether a cycle reachable from start has no exit
// (Tarjan's strongly connected components over default + condition edges).
// It cannot prove an exit condition ever fires; that is undecidable from YAML.
func cycleWithoutExit(steps doc, start string, iss *issues) bool {
	index := 0
	indices := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	found := false

	var strong func(id string)
	strong = func(id string) {
		indices[id] = index
		low[id] = index
		index++
		stack = append(stack, id)
		onStack[id] = true

		for _, target := range cycleEdges(steps, id, iss) {
			if isNextMarker(target) || !has(steps, target) {
				continue
			}
			if _, seen := indices[target]; !seen {
				strong(target)
				if low[target] < low[id] {
					low[id] = low[target]
				}
			} else if onStack[target] && indices[target] < low[id] {
				low[id] = indices[target]
			}
		}

		if low[id] != indices[id] {
			return
		}
		var component []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			component = append(component, top)
			if top == id {
				break
			}
		}
		inCycle := make(map[string]bool, len(component))
		for _, member := range component {
			inCycle[member] = true
		}
		cyclic := len(component) > 1
		if !cyclic {
			for _, target := range cycleEdges(steps, id, iss) {
				if target == id {
					cyclic = true
				}
			}
		}
		if !cyclic {
			return
		}
		for _, member := range component {
			if stepLeavesCycle(steps, member, inCycle, iss) {
				return
			}
		}
		found = true
	}

	if has(steps, start) {
		strong(start)
	}
	return found
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
