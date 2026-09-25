package aifvalidate

import (
	"fmt"
	"strconv"
	"strings"
)

// validateResponseExpectations checks each step's `response_expectation` fields.
//
// Union of the detailed validator (validateSemantics → the type must be a known
// data type) and the parser (ValidateFlow → an array field requires `items`;
// `required` must be a boolean or a string template). See PARITY.md.
func validateResponseExpectations(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		raw := get(step, keyResponseExpectation)
		if raw == nil {
			continue
		}
		re, isMap := asRecord(raw)
		if !isMap {
			iss.error(Issue{
				Field: stepField(stepID, keyResponseExpectation), Code: codeInvalidType, StepID: stepID,
				Message: "response_expectation must be a mapping",
			})
			continue
		}

		warnIfExpectationUnread(stepID, step, re, iss)

		for _, fieldName := range sortedKeys(re) {
			base := stepField(stepID, keyResponseExpectation, fieldName)
			field, isFieldMap := asRecord(re[fieldName])
			if !isFieldMap {
				iss.error(Issue{
					Field: base, Code: codeInvalidType, StepID: stepID,
					Message: fmt.Sprintf("response_expectation field '%s' must be a mapping", fieldName),
				})
				continue
			}

			dtype, hasType := getString(field, keyType)
			switch {
			case !hasType || !dataTypes.has(dtype):
				shown := "(none)"
				if hasType {
					shown = dtype
				}
				iss.error(Issue{
					Field: base + "." + keyType, Code: codeInvalidDataType, StepID: stepID,
					Message:    "Invalid data type: " + shown,
					Suggestion: "Use one of: " + joinNames(sortedSet(dataTypes)),
				})
			case dtype == typeArray && get(field, keyItems) == nil:
				iss.error(Issue{
					Field: base + "." + keyItems, Code: codeRespExpArrayItems, StepID: stepID,
					Message: fmt.Sprintf("Array field '%s' in step '%s' must define 'items'", fieldName, stepID),
				})
			}

			// `required` may be a boolean OR a string template (resolved at runtime).
			if requiredRaw, present := field[keyRequired]; present {
				_, isBool := asBool(requiredRaw)
				_, isStr := asString(requiredRaw)
				if !isBool && !isStr {
					iss.error(Issue{
						Field: base + "." + keyRequired, Code: codeInvalidValue, StepID: stepID,
						Message: fmt.Sprintf(
							"response_expectation.%s.required must be a boolean or a string template", fieldName),
					})
				}
			}
		}
	}
}

// asyncExecutorPrefix is the scheme whose respond route enforces a
// response_expectation on its own, with no evaluation mode.
const asyncExecutorPrefix = "async://"

// warnIfExpectationUnread ports response_expectation_unread (reference:
// validateResponseExpectationIsRead, AIF DC-FORGE-145). The engine reads a
// step's response_expectation ONLY when response_evaluation is set; with no
// evaluation mode it returns the raw response untouched, so required, type and
// fallback are never consulted. async:// is exempt.
//
// A WARNING, as in the reference: consulted at the run door over stored flows,
// and the declaration is inert, not fatal. Loop sub-steps cannot declare an
// expectation, so there is no loop-body walk.
func warnIfExpectationUnread(stepID string, step, re doc, iss *issues) {
	if len(re) == 0 {
		return
	}
	if evaluation, _ := scalarText(get(step, keyResponseEvaluation)); evaluation != "" {
		return
	}
	if executor, ok := getString(step, keyExecutor); ok && strings.HasPrefix(executor, asyncExecutorPrefix) {
		return
	}
	iss.warn(Issue{
		Field: stepField(stepID, keyResponseExpectation), Code: codeRespExpUnread, StepID: stepID,
		Message: fmt.Sprintf("response_expectation on step %s is never checked: the engine reads it only when "+
			"response_evaluation is set, and this step sets none, so required, type and fallback do nothing. "+
			"Add response_evaluation: \"raw-text\" to check these fields against the executor's response "+
			"unchanged, or remove response_expectation.", strconv.Quote(stepID)),
	})
}
