package aifvalidate

import "fmt"

// validateErrorStrategies checks the flow-level and per-step `error_strategy`.
//
// Mirrors validateErrorStrategy (parser.go): the action enum, goto target
// existence, max_delay/retry_delay durations, backoff_multiplier > 0, and the
// retry_on categories.
func validateErrorStrategies(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		steps = doc{}
	}

	if strategy, ok := getRecord(flow, keyErrorStrategy); ok {
		validateErrorStrategy(strategy, steps, keyErrorStrategy, "", iss)
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if strategy, ok := getRecord(step, keyErrorStrategy); ok {
			validateErrorStrategy(strategy, steps, stepField(stepID, keyErrorStrategy), stepID, iss)
		}
		validateLoopBodyErrorGoto(step, stepID, iss)
	}
}

// validateLoopBodyErrorGoto warns about a goto_step inside a loop BODY
// (loop_substep_error_goto_ignored, AIF v2.652.0 / DC-FORGE-82).
//
// A loop body is a second step table, and the walk above never enters it.
// Inside one, goto_step is unreachable under EVERY action: the loop driver's
// failure switch has two arms, `continue` and a default that aborts the whole
// loop step. So `goto` lands in the default and the loop fails — which is why
// this is a different finding from unreachable_error_goto, whose remedy
// (`action: goto`) does not help here.
//
// Deliberately NOT routed through validateErrorStrategy: the reference does not
// run its error_strategy checks over loop sub-steps, and an oracle that refuses
// shapes its door accepts is wrong in the more damaging direction.
func validateLoopBodyErrorGoto(step doc, stepID string, iss *issues) {
	loop, ok := getRecord(step, keyLoop)
	if !ok {
		return
	}
	subs, ok := getSlice(loop, keySteps)
	if !ok {
		return
	}
	for i, raw := range subs {
		sub, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		strategy, ok := getRecord(sub, keyErrorStrategy)
		if !ok {
			continue
		}
		target, ok := getString(strategy, keyGotoStep)
		if !ok || target == "" {
			continue
		}
		subID, ok := getString(sub, keyID)
		if !ok || subID == "" {
			subID = fmt.Sprintf("[%d]", i)
		}
		iss.warn(Issue{
			Field:  stepField(stepID, keyLoop, keySteps, subID, keyErrorStrategy, keyGotoStep),
			Code:   codeLoopSubstepErrGotoIgnore,
			StepID: stepID,
			Message: fmt.Sprintf("goto_step '%s' on loop step '%s's sub-step '%s' error_strategy is never read: "+
				"a loop body honours only action \"continue\" (skip to the next sub-step) and fail. Any other "+
				"action, including \"goto\", aborts the whole loop step — which then routes through the LOOP "+
				"step's own error_strategy.", target, stepID, subID),
			Suggestion: fmt.Sprintf("Put the goto_step on the loop step '%s' itself, or use action: \"continue\" here", stepID),
		})
	}
}

func validateErrorStrategy(strategy, steps doc, field, stepID string, iss *issues) {
	action, hasAction := getString(strategy, keyAction)
	if hasAction && action != "" && !errorStrategyActions.has(action) {
		iss.error(Issue{
			Field: field + "." + keyAction, Code: codeInvalidErrStrategy, StepID: stepID,
			Message:    fmt.Sprintf("Invalid error_strategy action '%s'", action),
			Suggestion: "Use one of: " + joinNames(sortedSet(errorStrategyActions)),
		})
	}

	if action == actionGoto {
		target, hasTarget := getString(strategy, keyGotoStep)
		switch {
		case !hasTarget || target == "":
			iss.error(Issue{
				Field: field + "." + keyGotoStep, Code: codeGotoStepMissing, StepID: stepID,
				Message: "error_strategy action 'goto' requires a 'goto_step'",
			})
		case !has(steps, target):
			iss.error(Issue{
				Field: field + "." + keyGotoStep, Code: codeStepNotFound, StepID: stepID,
				Message: fmt.Sprintf("Referenced step '%s' in error_strategy does not exist", target),
			})
		}
	}

	// A goto_step the action can never take (unreachable_error_goto, AIF
	// v2.651.0 / DC-FORGE-81). The engine reads goto_step in the `goto` branch
	// ONLY; retry, fail and an absent action all fall through to failing the
	// mission. A WARNING, as in the reference: it is consulted at the run door
	// over stored flows, and the declaration is inert rather than fatal.
	if target, ok := getString(strategy, keyGotoStep); ok && target != "" && action != actionGoto {
		shown := action
		if shown == "" {
			shown = "(absent, defaults to fail)"
		}
		owner := "the flow-level error_strategy"
		if stepID != "" {
			owner = fmt.Sprintf("step '%s'", stepID)
		}
		iss.warn(Issue{
			Field: field + "." + keyGotoStep, Code: codeUnreachableErrorGoto, StepID: stepID,
			Message: fmt.Sprintf("goto_step '%s' on %s can never be taken: the engine reads goto_step only "+
				"when action is \"goto\", and this action is '%s'", target, owner, shown),
			Suggestion: "Set action: \"goto\" (retries still apply through max_retries), or remove goto_step",
		})
	}

	// max_delay is an ERROR when malformed; retry_delay only a WARNING. That
	// asymmetry is the reference's, not an oversight: a bad max_delay disables the
	// backoff ceiling, a bad retry_delay falls back to a default.
	if d, ok := getString(strategy, keyMaxDelay); ok && d != "" && !isValidGoDuration(d) {
		iss.error(Issue{
			Field: field + "." + keyMaxDelay, Code: codeInvalidDuration, StepID: stepID,
			Message: fmt.Sprintf("Invalid max_delay duration '%s'", d),
		})
	}
	if d, ok := getString(strategy, keyRetryDelay); ok && d != "" && !isValidGoDuration(d) {
		iss.warn(Issue{
			Field: field + "." + keyRetryDelay, Code: codeInvalidDuration, StepID: stepID,
			Message: fmt.Sprintf("retry_delay '%s' is not a valid Go duration", d),
		})
	}

	if raw, present := strategy[keyBackoffMultiplier]; present {
		mult, isNum := asNumber(raw)
		if !isNum || mult <= 0 {
			iss.error(Issue{
				Field: field + "." + keyBackoffMultiplier, Code: codeInvalidBackoffMult, StepID: stepID,
				Message: fmt.Sprintf("backoff_multiplier must be > 0, got %v", raw),
			})
		}
	}

	if categories, ok := getSlice(strategy, keyRetryOn); ok {
		for i, raw := range categories {
			category, isStr := asString(raw)
			if isStr && retryOnCategories.has(category) {
				continue
			}
			iss.error(Issue{
				Field: fmt.Sprintf("%s.%s", field, indexed(keyRetryOn, i)),
				Code:  codeInvalidRetryOnCategry, StepID: stepID,
				Message:    fmt.Sprintf("Invalid retry_on category '%v'", raw),
				Suggestion: "Use one of: " + joinNames(sortedSet(retryOnCategories)),
			})
		}
	}
}
