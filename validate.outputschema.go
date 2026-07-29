package aifvalidate

// validateOutputSchemas checks each step's optional `output_schema` (DC-CP-7).
//
// In the Go reference this reuses the SAME structure and validator as
// input_schema (ValidateInputSchemaDefinition is called verbatim on
// step.OutputSchema), so this delegates to validateSchemaDefinition and differs
// only in the base field path. It is opt-in: a step without output_schema keeps
// lenient behaviour. Loop sub-steps may also carry one.
//
// NOT reproduced: the companion `unresolvable_data_path` rule (a
// `.data.<step>.<field>` reference to a field not declared in <step>'s
// output_schema) requires resolving template field references against the state
// graph — the same runtime concern this validator deliberately does not model
// for templates. Documented divergence; see PARITY.md.
func validateOutputSchemas(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if has(step, keyOutputSchema) {
			validateSchemaDefinition(get(step, keyOutputSchema),
				stepField(stepID, keyOutputSchema), iss)
		}

		loop, hasLoop := getRecord(step, keyLoop)
		if !hasLoop {
			continue
		}
		subs, hasSubs := getSlice(loop, keySteps)
		if !hasSubs {
			continue
		}
		for i, raw := range subs {
			sub, isMap := asRecord(raw)
			if !isMap {
				continue
			}
			if has(sub, keyOutputSchema) {
				validateSchemaDefinition(get(sub, keyOutputSchema),
					stepField(stepID, keyLoop, indexed(keySteps, i), keyOutputSchema), iss)
			}
		}
	}
}
