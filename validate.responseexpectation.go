package aifvalidate

import "fmt"

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
