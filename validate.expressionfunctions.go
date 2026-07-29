package aifvalidate

// validateExpressionFunctions checks the flow-level `expression_functions` list.
//
// Mirrors validateExpressionFunctions (parser.go): each entry is a mapping with
// EXACTLY one key, which must be `package` or `function`, with a non-empty value.
func validateExpressionFunctions(flow doc, iss *issues) {
	raw := get(flow, keyExpressionFunctions)
	if raw == nil {
		return
	}
	list, ok := asSlice(raw)
	if !ok {
		iss.error(Issue{
			Field: keyExpressionFunctions, Code: codeInvalidType,
			Message: "expression_functions must be a list",
		})
		return
	}

	for i, rawEntry := range list {
		field := indexed(keyExpressionFunctions, i)
		entry, isMap := asRecord(rawEntry)
		if !isMap {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "Each expression_functions entry must be a mapping",
			})
			continue
		}
		if len(entry) != 1 {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "Each expression_functions entry must have exactly one key (package OR function)",
			})
			continue
		}
		hasPackage, hasFunction := has(entry, keyPackage), has(entry, keyFunction)
		if !hasPackage && !hasFunction {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "expression_functions entry must use key 'package' or 'function'",
			})
			continue
		}
		key := keyFunction
		if hasPackage {
			key = keyPackage
		}
		if !isNonEmptyString(get(entry, key)) {
			iss.error(Issue{
				Field: field + "." + key, Code: codeInvalidExprFunction,
				Message: "expression_functions '" + key + "' must have a non-empty value",
			})
		}
	}
}
