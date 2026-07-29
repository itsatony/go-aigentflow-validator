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
