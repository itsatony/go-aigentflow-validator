package aifvalidate

import "fmt"

// validateServerOwnedQueryKeys refuses a step query, or a loop sub-step query,
// that declares a server-owned executor parameter (server_owned_query_key;
// reference: validateNoServerOwnedStepQueryKeys, AIgentFlow CFX-05).
//
// The aiv:// executor sends the running org's aigentverse credential (or a
// signed token naming the running person) to the base URL in its parameters.
// The reference used to lay a step's query over the credential resolver's
// output, so a shared flow declaring `aiv_base_url: https://attacker.example`
// sent that credential to a host its AUTHOR chose. The engine now discards such
// a value at run time; this is the named refusal at save. ERROR: no legitimate
// flow sets these keys, because the resolver is their only producer.
//
// Matching is exact and case-sensitive (`AIV_API_KEY` and `aiv_ref` are
// ordinary query keys), over the key set in the spec (serverOwnedQueryKeys.keys).
// Exactly the two surfaces the reference scans are scanned: a top-level step's
// `query:` and a loop sub-step's `query:`. A parallel branch or a for_each body
// is a top-level step, so it is covered by the first. A key is refused whatever
// its value, templated or null included.
//
// A loop sub-step is addressed by its id as written (source text, like every
// Go-string field). A sub-step without an id is addressed by the empty string,
// the reference's format with its zero value; the reference itself never gets
// that far, because its parse refuses the missing id (loop_step_id_required
// here), so the verdict is the same.
func validateServerOwnedQueryKeys(flow doc, iss *issues) {
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if query, ok := getRecord(step, keyQuery); ok {
			for _, key := range sortedKeys(query) {
				if serverOwnedQueryKeys.has(key) {
					refuseServerOwnedQueryKey(stepID, key,
						fmt.Sprintf(spec.ServerOwnedQueryKeys.StepFieldFormat, stepID, key), iss)
				}
			}
		}
		for _, sub := range loopSubStepsOf(stepID, step) {
			query, ok := getRecord(sub.raw, keyQuery)
			if !ok {
				continue
			}
			subID, _ := iss.stringOf(sub.raw, keyID, sub.basePath)
			for _, key := range sortedKeys(query) {
				if serverOwnedQueryKeys.has(key) {
					refuseServerOwnedQueryKey(stepID, key,
						fmt.Sprintf(spec.ServerOwnedQueryKeys.LoopStepFieldFormat, stepID, subID, key), iss)
				}
			}
		}
	}
}

func refuseServerOwnedQueryKey(stepID, key, field string, iss *issues) {
	iss.error(Issue{
		Field: field, Code: codeServerOwnedQueryKey, StepID: stepID,
		Message: fmt.Sprintf("query key '%s' on step '%s' is set by the server only (it carries the "+
			"aigentverse credential and where it is sent), so a flow may not declare it", key, stepID),
		Suggestion: "Remove it: the running org's aigentverse connection, or the signed-in person, is used automatically",
	})
}
