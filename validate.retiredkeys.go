package aifvalidate

import "fmt"

// validateRetiredKeys refuses grammar keys AIgentFlow deleted because nothing
// read them: the flow-root `budget:`, the flow-root `max_retries:`, a
// `max_retries:` directly on a step (all AIF v2.721.0, DC-FORGE-150), and
// `campaign.budget_max_per_child` (AIF v2.728.0, DC-FORGE-155).
//
// The reference's save door (create, update, POST /flows/validate) parses with
// yaml KnownFields(true), so a flow declaring any of them is REFUSED, and its
// retiredGrammarKeys table attaches migration advice. Stored flows still load
// leniently and run. This validator answers "would this save", so each is an
// ERROR, with the code already used for the other key the reference refuses
// specifically: unknown_yaml_key (a condition's `goto_step`).
//
// The check is on key PRESENCE, not value: the strict decoder refuses the key
// whatever it holds, so `budget: 0` and `budget: null` are refused too.
//
// The working keys one level DOWN must never be reported:
// error_strategy.max_retries (flow and step), quality_gate.max_retries and
// billing.max_credits are live grammar.
func validateRetiredKeys(flow doc, iss *issues) {
	if has(flow, keyBudget) {
		iss.error(Issue{
			Field: keyBudget, Code: codeUnknownYAMLKey,
			Message: "The flow-level `budget:` key was removed from the grammar in AIgentFlow " + retiredKeysRemovedIn +
				" because nothing read it (it limited no spend), and AIgentFlow refuses to save a flow that declares it.",
			Suggestion: "Delete it. To cap spend use `billing: { max_credits: N }` — it caps the credit reservation " +
				"and refuses further agentic turns (agentic ai://, exons://, the orchestrator); plain chat steps " +
				"and flow:// sub-flows are not checked against it.",
		})
	}
	if has(flow, keyMaxRetries) {
		iss.error(Issue{
			Field: keyMaxRetries, Code: codeUnknownYAMLKey,
			Message: retiredMaxRetriesMessage("the flow"), Suggestion: retiredMaxRetriesSuggestion,
		})
	}
	if campaign, ok := getRecord(flow, keyCampaign); ok && has(campaign, keyBudgetMaxPerChild) {
		iss.error(Issue{
			Field: keyCampaign + "." + keyBudgetMaxPerChild, Code: codeUnknownYAMLKey,
			Message: "`campaign.budget_max_per_child` (USD) was removed from the grammar in AIgentFlow " +
				budgetMaxPerChildRemovedIn + " because it capped nothing, and AIgentFlow refuses to save a flow that declares it.",
			Suggestion: "Use `campaign.max_credits_per_child: N` (a whole number of credits) — it is checked before " +
				"each agentic turn of a child, and a spawn may lower it with `max_credits`.",
		})
	}

	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if has(step, keyMaxRetries) {
			iss.error(Issue{
				Field: stepField(stepID, keyMaxRetries), Code: codeUnknownYAMLKey, StepID: stepID,
				Message:    retiredMaxRetriesMessage(fmt.Sprintf("step '%s'", stepID)),
				Suggestion: retiredMaxRetriesSuggestion,
			})
		}

		// A loop sub-step's type never had a `max_retries` field, so the strict
		// decoder refuses it there too — and the advice table matches the key
		// NAME, not the type, so the reference attaches this same advice.
		loop, ok := getRecord(step, keyLoop)
		if !ok {
			continue
		}
		subs, ok := getSlice(loop, keySteps)
		if !ok {
			continue
		}
		for i, raw := range subs {
			sub, isMap := asRecord(raw)
			if !isMap || !has(sub, keyMaxRetries) {
				continue
			}
			subID, isStr := getString(sub, keyID)
			if !isStr {
				subID = fmt.Sprintf("%d", i)
			}
			composite := stepID + reservedStepIDChar + subID
			iss.error(Issue{
				Field: stepField(stepID, keyLoop, indexed(keySteps, i), keyMaxRetries), Code: codeUnknownYAMLKey,
				StepID:     composite,
				Message:    retiredMaxRetriesMessage(fmt.Sprintf("loop sub-step '%s'", composite)),
				Suggestion: retiredMaxRetriesSuggestion,
			})
		}
	}
}

const (
	retiredKeysRemovedIn       = "v2.721.0"
	budgetMaxPerChildRemovedIn = "v2.728.0"

	retiredMaxRetriesSuggestion = "Delete it. To retry a step write `error_strategy: { action: \"retry\", max_retries: N }` " +
		"on that step (or on the flow) — the same key one level down, which is the one that works " +
		"(N counts attempts, including the first)."
)

func retiredMaxRetriesMessage(owner string) string {
	return fmt.Sprintf("`max_retries:` on %s was removed from the grammar in AIgentFlow %s because the retry "+
		"engine never read it, and AIgentFlow refuses to save a flow that declares it.", owner, retiredKeysRemovedIn)
}
