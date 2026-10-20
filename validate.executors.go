package aifvalidate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// validateExecutors checks executor URIs.
//
// Since AIgentFlow v2.598.0 (DC-FORGE-30) the reference applies its ONE
// executor-URL parser (URL_PATTERN_REGEX) at save time, and since v2.608.0
// (DC-FORGE-38) to loop sub-step executors too. The shape check here is that
// same regex, vendored verbatim in the spec:
//   - an unparsable URL is an ERROR: AIgentFlow refuses it at save;
//   - an unknown scheme is a WARNING: the vendored list can lag the registry,
//     and the reference never rejects on scheme.
//
// TEMPLATED URLs ARE SKIPPED, as the reference skips them: the engine renders
// step.executor as a template before dispatch, so
// `flow://stored/{{ .query.child_flow_id }}` cannot be judged statically.
// See PARITY.md.
func validateExecutors(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if executor, isStr := iss.stringOf(step, keyExecutor, stepField(stepID)); isStr && executor != "" {
			if !checkRemovedSurfaces(executor, get(step, keyQuery), stepField(stepID), stepID, stepID, iss) {
				checkExecutor(executor, stepField(stepID, keyExecutor), stepID, iss)
			}
			checkAtlasStep(executor, get(step, keyQuery), stepField(stepID), stepID, iss)
		}

		// Loop sub-steps each carry their own executor.
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
			executor, isStr := iss.stringOf(sub, keyExecutor, stepField(stepID, keyLoop, indexed(keySteps, i)))
			if !isStr || executor == "" {
				continue
			}
			if !checkRemovedSurfaces(executor, get(sub, keyQuery),
				stepField(stepID, keyLoop, indexed(keySteps, i)),
				subStepID(stepID, sub, i, iss), subStepID(stepID, sub, i, iss), iss) {
				checkExecutor(executor,
					stepField(stepID, keyLoop, indexed(keySteps, i), keyExecutor),
					subStepID(stepID, sub, i, iss), iss)
			}
			checkAtlasStep(executor, get(sub, keyQuery),
				stepField(stepID, keyLoop, indexed(keySteps, i)),
				subStepID(stepID, sub, i, iss), iss)
		}
	}
}

// subStepID composes the engine's composite loop sub-step ID ("parent.child"),
// falling back to the index when the sub-step declares no id.
func subStepID(stepID string, sub doc, i int, iss *issues) string {
	if id, ok := iss.stringOf(sub, keyID, stepField(stepID, keyLoop, indexed(keySteps, i))); ok && id != "" {
		return stepID + reservedStepIDChar + id
	}
	return stepID + reservedStepIDChar + strconv.Itoa(i)
}

func checkExecutor(executor, field, stepID string, iss *issues) {
	sepIdx := strings.Index(executor, schemeSeparator)
	scheme := ""
	if sepIdx > 0 {
		scheme = executor[:sepIdx]
	}

	// A templated URL is rendered by the engine before dispatch, so its shape
	// cannot be judged here. The scheme is still checked when the scheme itself
	// carries no template, because that half is knowable.
	templated := strings.Contains(executor, templateOpenDelim)

	if !templated && !executorURLRe.MatchString(executor) {
		iss.error(Issue{
			Field: field, Code: codeInvalidExecutorURL, StepID: stepID,
			Message: fmt.Sprintf("Executor '%s' is not a usable executor URL", executor),
			Suggestion: "Use the form 'scheme://authority/path' with a non-empty authority " +
				"(e.g., 'ai://openai/chat'). Authority and path accept letters, digits, underscore and hyphen only.",
		})
		return
	}

	if scheme == "" || strings.Contains(scheme, templateOpenDelim) {
		return
	}
	lower := strings.ToLower(scheme)
	if !executorSchemes.has(lower) {
		iss.warn(Issue{
			Field: field, Code: codeUnknownExecScheme, StepID: stepID,
			Message: unknownSchemeMessage(scheme),
		})
		return
	}
	if lower == schemeFunction && !templated {
		checkFunctionOperation(executor, field, stepID, iss)
	}
}

const (
	schemeFunction = "function"
	// schemeDecide is a scheme authors guess for decision models; they run
	// through ai://{provider}/decide.
	schemeDecide = "decide"
	// schemeSuggestMaxDistance is the edit distance within which a known scheme
	// is offered as a "did you mean".
	schemeSuggestMaxDistance = 2
)

// unknownSchemeMessage keeps the unknown_executor_scheme warning actionable
// without changing its severity: the vendored list can lag the registry.
func unknownSchemeMessage(scheme string) string {
	msg := fmt.Sprintf(
		"Unknown executor scheme '%s'. It may be valid in a newer AIgentFlow release, or it could be a typo.",
		scheme)
	lower := strings.ToLower(scheme)
	if lower == schemeDecide {
		return msg + " Decision models run through ai://{provider}/decide (for example ai://openai/decide), not through a scheme of their own."
	}
	best, bestDist := "", schemeSuggestMaxDistance+1
	for _, known := range spec.ExecutorSchemes {
		if d := editDistance(lower, strings.ToLower(known)); d < bestDist || (d == bestDist && known < best) {
			best, bestDist = known, d
		}
	}
	if best != "" {
		msg += fmt.Sprintf(" Did you mean '%s'?", best)
	}
	return msg
}

// editDistance is the Levenshtein distance between two strings.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// checkFunctionOperation refuses a function:// executor whose operation
// (authority + "/" + path) is not a registered built-in
// (unknown_function_operation; reference: validateFunctionOperationKnown).
// The URL shape was already judged by checkExecutor, so a URL that reaches here
// parses; the split is the reference's: provider = authority, resource = path.
func checkFunctionOperation(executor, field, stepID string, iss *issues) {
	rest := executor[strings.Index(executor, schemeSeparator)+len(schemeSeparator):]
	provider, resource, _ := strings.Cut(rest, "/")
	op := provider + "/" + resource
	if functionOperations.has(op) {
		return
	}
	available := append([]string(nil), spec.FunctionOperations.Operations...)
	sort.Strings(available)
	list := strings.Join(available, ", ")
	iss.error(Issue{
		Field: field, Code: codeUnknownFunctionOperation, StepID: stepID,
		Message:    fmt.Sprintf(spec.FunctionOperations.MessageFormat, stepID, op, list),
		Suggestion: fmt.Sprintf(spec.FunctionOperations.Suggestion, list),
	})
}

// checkAtlasStep applies the atlas:// rules. It runs only for an untemplated
// atlas:// URL (the scheme match is exact and case-sensitive). The URL says which
// Atlas operation the step performs; the query is checked against what that
// operation needs. field is the step itself, as the findings are about the step
// as a whole.
//
// The URL is split at the first slash after "atlas://" rather than through the
// shared executor-URL shape, so a reference the shape refuses (a dot or colon in
// a capability reference) is still judged by the atlas rules; the shape's own
// invalid_executor_url finding is reported separately, as before.
func checkAtlasStep(executor string, query any, field, stepID string, iss *issues) {
	if strings.Contains(executor, templateOpenDelim) {
		return
	}
	prefix := spec.AtlasExecutor.Scheme + "://"
	if !strings.HasPrefix(executor, prefix) {
		return
	}
	provider, resource, _ := strings.Cut(strings.TrimPrefix(executor, prefix), "/")
	quoted := strconv.Quote(stepID)
	report := func(rule atlasRule, args ...any) {
		iss.error(Issue{
			Field: field, Code: rule.Code, StepID: stepID,
			Message:    fmt.Sprintf(rule.MessageFormat, append([]any{quoted}, args...)...),
			Suggestion: rule.Suggestion,
		})
	}
	q, _ := asRecord(query)

	refusedKeys := spec.AtlasExecutor.ParamRefused.CapabilityKeys
	switch {
	case provider == spec.AtlasExecutor.ProviderCapability:
		if !atlasCapabilityRefRe.MatchString(resource) {
			report(spec.AtlasExecutor.CapabilityRefInvalid, resource)
		}
	case provider == spec.AtlasExecutor.ProviderAgent && resource == spec.AtlasExecutor.AgentResource:
		refusedKeys = spec.AtlasExecutor.ParamRefused.AgentKeys
		agent, isStr := getString(q, "agent")
		if !isStr || strings.TrimSpace(agent) == "" {
			report(spec.AtlasExecutor.AgentMissing)
		} else if strings.Contains(agent, templateOpenDelim) {
			report(spec.AtlasExecutor.AgentNotLiteral)
		}
		hasPrompt := atlasTextPresent(get(q, "prompt"))
		hasTemplate := atlasTextPresent(get(q, "prompt_template"))
		if !hasPrompt && !hasTemplate {
			report(spec.AtlasExecutor.PromptMissing)
		} else if hasPrompt && hasTemplate {
			report(spec.AtlasExecutor.PromptBoth)
		}
		if anyTemplate(get(q, "model")) {
			report(spec.AtlasExecutor.ModelNotLiteral)
		}
	default:
		report(spec.AtlasExecutor.OperationUnknown, executor)
		return
	}

	// A templated capability entry is refused wherever the list appears on an
	// atlas:// step, nested values included.
	if anyTemplate(get(q, "capabilities")) {
		report(spec.AtlasExecutor.CapabilitiesNotLiteral)
	}

	// One finding per step, listing every key the platform owns that it carries.
	var refused []string
	for _, key := range refusedKeys {
		if has(q, key) {
			refused = append(refused, key)
		}
	}
	if len(refused) > 0 {
		sort.Strings(refused)
		report(spec.AtlasExecutor.ParamRefused.atlasRule, strings.Join(refused, ", "))
	}
}

// anyTemplate reports whether a string, or any string nested in a list or map,
// contains a template action.
func anyTemplate(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, templateOpenDelim)
	case []any:
		for _, item := range t {
			if anyTemplate(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range t {
			if anyTemplate(item) {
				return true
			}
		}
	}
	return false
}

// atlasTextPresent reports a value that is set: not absent, null or a string
// that is empty after trimming whitespace. A non-string value counts as present.
func atlasTextPresent(v any) bool {
	if v == nil {
		return false
	}
	s, isStr := v.(string)
	return !isStr || strings.TrimSpace(s) != ""
}
