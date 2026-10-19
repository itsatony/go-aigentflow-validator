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
			checkExecutor(executor, stepField(stepID, keyExecutor), stepID, iss)
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
			checkExecutor(executor,
				stepField(stepID, keyLoop, indexed(keySteps, i), keyExecutor),
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
