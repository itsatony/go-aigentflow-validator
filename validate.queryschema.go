package aifvalidate

import "fmt"

// validateQuerySchema checks the flow-level `query:` input-parameter schema.
//
// Mirrors validateQueryParameters / validateProperties / validateArrayItems /
// validateArrayConstraints (parser.go). Note, matching the Go reference: the
// TOP-LEVEL param type is only required to be non-empty — it is NOT constrained
// to the known data-type set (an unknown top-level type is a warning). Array
// ITEM types ARE constrained, as in Go.
func validateQuerySchema(flow doc, iss *issues) {
	raw := get(flow, keyQuery)
	if raw == nil {
		return
	}
	query, ok := asRecord(raw)
	if !ok {
		iss.error(Issue{
			Field: keyQuery, Code: codeInvalidType,
			Message: "query must be a mapping of parameter name to definition",
		})
		return
	}

	for _, paramName := range sortedKeys(query) {
		path := keyQuery + "." + paramName
		def, isMap := asRecord(query[paramName])
		if !isMap {
			iss.error(Issue{
				Field: path, Code: codeInvalidType,
				Message: fmt.Sprintf("Query parameter '%s' must be a mapping", paramName),
			})
			continue
		}
		dtype, hasType := getString(def, keyType)
		if !hasType || dtype == "" {
			iss.error(Issue{
				Field: path + "." + keyType, Code: codeQueryParamTypeMissing,
				Message: fmt.Sprintf("Query parameter '%s' is missing a 'type'", paramName),
			})
			continue
		}
		if !dataTypes.has(dtype) {
			iss.warn(Issue{
				Field: path + "." + keyType, Code: codeUnknownDataType,
				Message: fmt.Sprintf("Query parameter '%s' has unrecognised type '%s'", paramName, dtype),
			})
		}
		switch dtype {
		case typeObject:
			validateProperties(get(def, keyProperties), path, iss)
		case typeArray:
			validateArrayItems(get(def, keyItems), path, iss)
			validateArrayConstraints(def, path, iss)
		}
	}
}

func validateProperties(raw any, parentPath string, iss *issues) {
	if raw == nil {
		return
	}
	properties, ok := asRecord(raw)
	if !ok {
		iss.error(Issue{
			Field: parentPath + "." + keyProperties, Code: codeInvalidType,
			Message: fmt.Sprintf("properties of '%s' must be a mapping", parentPath),
		})
		return
	}
	for _, propName := range sortedKeys(properties) {
		path := parentPath + "." + propName
		def, isMap := asRecord(properties[propName])
		if !isMap {
			iss.error(Issue{
				Field: path, Code: codeInvalidType,
				Message: fmt.Sprintf("Property '%s' must be a mapping", propName),
			})
			continue
		}
		dtype, hasType := getString(def, keyType)
		if !hasType || dtype == "" {
			iss.error(Issue{
				Field: path + "." + keyType, Code: codePropertyTypeMissing,
				Message: fmt.Sprintf("Property '%s' in '%s' is missing a 'type'", propName, parentPath),
			})
			continue
		}
		if !dataTypes.has(dtype) {
			iss.warn(Issue{
				Field: path + "." + keyType, Code: codeUnknownDataType,
				Message: fmt.Sprintf("Property '%s' has unrecognised type '%s'", propName, dtype),
			})
		}
		switch dtype {
		case typeObject:
			validateProperties(get(def, keyProperties), path, iss)
		case typeArray:
			validateArrayItems(get(def, keyItems), path, iss)
		}
	}
}

func validateArrayItems(raw any, path string, iss *issues) {
	items, ok := asRecord(raw)
	if !ok {
		iss.error(Issue{
			Field: path, Code: codeArrayItemsMissing,
			Message: fmt.Sprintf("Array '%s' must define an 'items' schema", path),
		})
		return
	}
	dtype, hasType := getString(items, keyType)
	if !hasType || dtype == "" {
		iss.error(Issue{
			Field: path + "." + keyItems + "." + keyType, Code: codeArrayItemsTypeInvalid,
			Message: fmt.Sprintf("Array items for '%s' must define a 'type'", path),
		})
		return
	}
	if !dataTypes.has(dtype) {
		iss.error(Issue{
			Field: path + "." + keyItems + "." + keyType, Code: codeArrayItemsTypeInvalid,
			Message:    fmt.Sprintf("Array items for '%s' have invalid type '%s'", path, dtype),
			Suggestion: "Use one of: " + joinNames(sortedSet(dataTypes)),
		})
		return
	}
	itemPath := path + "[item]"
	switch dtype {
	case typeObject:
		validateProperties(get(items, keyProperties), itemPath, iss)
	case typeArray:
		validateArrayItems(get(items, keyItems), itemPath, iss)
	}
}

func validateArrayConstraints(def doc, path string, iss *issues) {
	minRaw, hasMin := def[keyMinItems]
	minVal, minIsInt := asInteger(minRaw)
	if hasMin && (!minIsInt || minVal < 0) {
		iss.error(Issue{
			Field: path + "." + keyMinItems, Code: codeArrayMinItemsInvalid,
			Message: fmt.Sprintf("min_items for '%s' must be a non-negative integer", path),
		})
	}
	maxVal, maxIsInt := asInteger(def[keyMaxItems])
	if minIsInt && maxIsInt && maxVal < minVal {
		iss.error(Issue{
			Field: path + "." + keyMaxItems, Code: codeArrayMaxItemsInvalid,
			Message: fmt.Sprintf("max_items (%d) for '%s' must be >= min_items (%d)", maxVal, path, minVal),
		})
	}
}
