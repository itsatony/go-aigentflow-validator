package aifvalidate

import (
	"fmt"
	"regexp"
)

// validateInputSchema checks the flow-level `input_schema` definition.
func validateInputSchema(flow doc, iss *issues) {
	validateSchemaDefinition(get(flow, keyInputSchema), keyInputSchema, iss)
}

// validateSchemaDefinition validates one InputSchema-shaped definition at an
// arbitrary base path.
//
// Mirrors ValidateInputSchemaDefinition + LintInputSchemaFieldOrdering
// (input_schema.go), which the Go reference reuses VERBATIM for both
// flow.input_schema and a step's output_schema (DC-CP-7). The emitted codes are
// therefore the shared input_schema_* set regardless of the site; only the field
// path differs. This validates the schema DEFINITION only — payload validation
// (ValidateInputAgainstSchema) is runtime and out of scope.
//
// DIVERGENCE, in this implementation's favour: `pattern` is compiled with Go's
// RE2 (regexp), which is what the engine uses. The JS port compiles with the JS
// regex engine, so a pattern valid in one and not the other is a known
// divergence — recorded in PARITY.md, and this side is the authoritative one.
func validateSchemaDefinition(raw any, schemaPath string, iss *issues) {
	if raw == nil {
		return
	}
	schema, ok := asRecord(raw)
	if !ok {
		iss.error(Issue{
			Field: schemaPath, Code: codeInvalidType,
			Message: schemaPath + " must be a mapping",
		})
		return
	}

	version, isInt := asInteger(get(schema, keyVersion))
	if !isInt || version != spec.InputSchemaVersion {
		iss.error(Issue{
			Field: schemaPath + "." + keyVersion, Code: codeISInvalidVersion,
			Message: fmt.Sprintf("%s version %v is not supported (expected %d)",
				schemaPath, get(schema, keyVersion), spec.InputSchemaVersion),
		})
	}

	rawFields := get(schema, keyFields)
	if rawFields == nil {
		return
	}
	fields, ok := asSlice(rawFields)
	if !ok {
		iss.error(Issue{
			Field: schemaPath + "." + keyFields, Code: codeInvalidType,
			Message: schemaPath + ".fields must be a list",
		})
		return
	}

	// Resolution index for visible_when. Built first so forward references are
	// allowed (a field may gate on one declared later).
	allNames := make(map[string]struct{}, len(fields))
	for _, raw := range fields {
		if f, ok := asRecord(raw); ok {
			if name, ok := getString(f, keyName); ok && name != "" {
				allNames[name] = struct{}{}
			}
		}
	}

	seen := make(map[string]int, len(fields))
	for i, raw := range fields {
		f, ok := asRecord(raw)
		if !ok {
			iss.error(Issue{
				Field: schemaFieldPath(schemaPath, i), Code: codeInvalidType,
				Message: fmt.Sprintf("%s field at index %d must be a mapping", schemaPath, i),
			})
			continue
		}
		validateSchemaField(f, schemaPath, i, seen, iss)
	}

	// Second pass: visible_when references must resolve.
	for i, raw := range fields {
		f, ok := asRecord(raw)
		if !ok {
			continue
		}
		vw, ok := getRecord(f, keyVisibleWhen)
		if !ok {
			continue
		}
		ref, ok := getString(vw, keyField)
		if !ok || ref == "" {
			continue
		}
		if _, known := allNames[ref]; !known {
			iss.error(Issue{
				Field: schemaFieldPath(schemaPath, i) + "." + keyVisibleWhen + "." + keyField,
				Code:  codeISVisibleWhenUnknown,
				Message: fmt.Sprintf("field '%s': visible_when references unknown field '%s'",
					schemaFieldName(f, i), ref),
			})
		}
	}

	lintSchemaFieldOrdering(fields, schemaPath, iss)
}

func schemaFieldPath(schemaPath string, i int) string {
	return fmt.Sprintf("%s.%s[%d]", schemaPath, keyFields, i)
}

// schemaFieldName renders a field's name for a message, falling back to "#i" so
// a nameless field is still identifiable.
func schemaFieldName(f doc, i int) string {
	if name, ok := getString(f, keyName); ok && name != "" {
		return name
	}
	return fmt.Sprintf("#%d", i)
}

func validateSchemaField(f doc, schemaPath string, i int, seen map[string]int, iss *issues) {
	base := schemaFieldPath(schemaPath, i)
	name, nameIsStr := getString(f, keyName)

	switch {
	case !nameIsStr || !fieldNameRe.MatchString(name):
		iss.error(Issue{
			Field: base, Code: codeISInvalidFieldName,
			Message: fmt.Sprintf("input_schema field name '%v' is invalid (must match %s)",
				get(f, keyName), spec.InputSchema.FieldNamePattern),
		})
	default:
		if first, dup := seen[name]; dup {
			iss.error(Issue{
				Field: base, Code: codeISDuplicateFieldName,
				Message: fmt.Sprintf("Duplicate input_schema field name '%s' (first declared at index %d)",
					name, first),
			})
		} else {
			seen[name] = i
		}
	}

	ftype, typeIsStr := getString(f, keyType)
	if !typeIsStr || !inputSchemaTypes.has(ftype) {
		iss.error(Issue{
			Field: base + "." + keyType, Code: codeISUnknownType,
			Message:    fmt.Sprintf("Unknown input_schema field type '%v'", get(f, keyType)),
			Suggestion: "Use one of: " + joinNames(sortedSet(inputSchemaTypes)),
		})
		return // per-type checks are meaningless without a known type
	}

	label := schemaFieldName(f, i)

	if ftype == typeEnum {
		if values, ok := getSlice(f, keyEnum); !ok || len(values) == 0 {
			iss.error(Issue{
				Field: base + "." + keyEnum, Code: codeISEnumEmpty,
				Message: fmt.Sprintf("enum field '%s' must list at least one allowed value", label),
			})
		}
	}

	checkSchemaRange(f, base, label, keyMin, keyMax, iss)
	checkSchemaRange(f, base, label, keyMinLength, keyMaxLength, iss)
	checkSchemaRange(f, base, label, keyMinItems, keyMaxItems, iss)

	// Constraint-to-type compatibility: a constraint that cannot apply to the
	// declared type is an author error, not a harmless extra key.
	mismatch := func(constraint string) {
		iss.error(Issue{
			Field: base + "." + constraint, Code: codeISConstraintMismatch,
			Message: fmt.Sprintf("constraint '%s' is not meaningful for field '%s' of type '%s'",
				constraint, label, ftype),
		})
	}
	if !inputStringTypes.has(ftype) {
		if has(f, keyMinLength) {
			mismatch(keyMinLength)
		}
		if has(f, keyMaxLength) {
			mismatch(keyMaxLength)
		}
		if isNonEmptyString(get(f, keyPattern)) {
			mismatch(keyPattern)
		}
	}
	if ftype != typeNumber {
		if has(f, keyMin) {
			mismatch(keyMin)
		}
		if has(f, keyMax) {
			mismatch(keyMax)
		}
	}
	if ftype != typeArrayOfStrings {
		if has(f, keyMinItems) {
			mismatch(keyMinItems)
		}
		if has(f, keyMaxItems) {
			mismatch(keyMaxItems)
		}
	}
	if ftype != typeFile {
		if accept, ok := getSlice(f, keyAccept); ok && len(accept) > 0 {
			mismatch(keyAccept)
		}
		if has(f, keyMaxSize) {
			mismatch(keyMaxSize)
		}
	}
	if ftype != typeEnum {
		if values, ok := getSlice(f, keyEnum); ok && len(values) > 0 {
			mismatch(keyEnum)
		}
	}

	// Constraint value ceilings.
	for _, constraint := range []string{keyMinLength, keyMaxLength, keyMinItems, keyMaxItems} {
		val, isInt := asInteger(get(f, constraint))
		if isInt && float64(val) > spec.InputSchema.MaxConstraintValue {
			iss.error(Issue{
				Field: base + "." + constraint, Code: codeISConstraintRange,
				Message: fmt.Sprintf("constraint '%s' (%d) for field '%s' exceeds the maximum of %.0f",
					constraint, val, label, spec.InputSchema.MaxConstraintValue),
			})
		}
	}

	if vw, ok := getRecord(f, keyVisibleWhen); ok {
		hasEquals := get(vw, keyEquals) != nil
		inValues, inOK := getSlice(vw, keyIn)
		hasIn := inOK && len(inValues) > 0
		switch {
		case !isNonEmptyString(get(vw, keyField)):
			iss.error(Issue{
				Field: base + "." + keyVisibleWhen, Code: codeISVisibleWhenNoPred,
				Message: fmt.Sprintf("field '%s': visible_when requires a 'field'", label),
			})
		case hasEquals == hasIn:
			// Both or neither: the predicate is ambiguous or absent.
			iss.error(Issue{
				Field: base + "." + keyVisibleWhen, Code: codeISVisibleWhenNoPred,
				Message: fmt.Sprintf("field '%s': visible_when requires exactly one of 'equals' or 'in'", label),
			})
		}
	}

	if pattern, ok := getString(f, keyPattern); ok && pattern != "" {
		if len(pattern) > spec.InputSchema.MaxPatternLength {
			iss.error(Issue{
				Field: base + "." + keyPattern, Code: codeISPatternTooLong,
				Message: fmt.Sprintf("field '%s': pattern length %d exceeds the maximum of %d",
					label, len(pattern), spec.InputSchema.MaxPatternLength),
			})
		} else if _, err := regexp.Compile(pattern); err != nil {
			iss.error(Issue{
				Field: base + "." + keyPattern, Code: codeISInvalidPattern,
				Message: fmt.Sprintf("field '%s': invalid pattern: %v", label, err),
			})
		}
	}
}

// checkSchemaRange reports a min > max pair. Both must be present and numeric for
// the comparison to mean anything.
func checkSchemaRange(f doc, base, label, minKey, maxKey string, iss *issues) {
	minVal, minOK := asNumber(get(f, minKey))
	maxVal, maxOK := asNumber(get(f, maxKey))
	if !minOK || !maxOK || minVal <= maxVal {
		return
	}
	iss.error(Issue{
		Field: base, Code: codeISInvalidRange,
		Message: fmt.Sprintf("field '%s': %s (%v) must be <= %s (%v)",
			label, minKey, get(f, minKey), maxKey, get(f, maxKey)),
	})
}

// lintSchemaFieldOrdering warns when a `file` field is declared after a
// parametric (number/enum) field. Mirrors LintInputSchemaFieldOrdering: the
// form renderer resolves parametric constraints in declaration order, so a file
// upload after them reads out of sequence to a user.
func lintSchemaFieldOrdering(fields []any, schemaPath string, iss *issues) {
	firstParametricIdx := -1
	firstParametricName, firstParametricType := "", ""
	for i, raw := range fields {
		f, ok := asRecord(raw)
		if !ok {
			continue
		}
		ftype, ok := getString(f, keyType)
		if !ok {
			continue
		}
		if firstParametricIdx == -1 && inputParametricTypes.has(ftype) {
			firstParametricIdx = i
			firstParametricName = schemaFieldName(f, i)
			firstParametricType = ftype
			continue
		}
		if ftype == typeFile && firstParametricIdx != -1 {
			iss.warn(Issue{
				Field: schemaFieldPath(schemaPath, i), Code: codeISFileAfterParam,
				Message: fmt.Sprintf(
					"file field '%s' is declared after parametric field '%s' (%s); consider moving file fields first",
					schemaFieldName(f, i), firstParametricName, firstParametricType),
			})
		}
	}
}
