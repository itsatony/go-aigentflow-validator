package aifvalidate

import (
	"fmt"
	"sort"
	"strings"
)

// The agent surfaces AIgentFlow removed: the exons:// and agent:// executors,
// the flow-level exons_uploads key, and the ai:// parameters that configured its
// own agent loop. Both findings are ERRORS and are addressed to the step
// (`steps.<id>`, or `steps.<id>.loop.steps[<index>]`), exactly as the atlas
// findings are.

// removedExecutorScheme returns the lower-cased scheme of an executor URL, or ""
// when the executor is templated (nothing can be said about what it renders to)
// or has no scheme separator.
func removedExecutorScheme(executor string) string {
	if strings.Contains(executor, templateOpenDelim) {
		return ""
	}
	scheme, _, found := strings.Cut(strings.TrimSpace(executor), schemeSeparator)
	if !found {
		return ""
	}
	return strings.ToLower(scheme)
}

// checkRemovedSurfaces judges one step's executor. It reports whether the step
// is on a removed scheme: such a step yields ONLY executor_removed, so the
// caller skips the scheme-specific rules (URL shape, unknown scheme).
func checkRemovedSurfaces(executor string, query any, field, stepID, label string, iss *issues) bool {
	cfg := spec.RemovedAgentSurfaces
	scheme := removedExecutorScheme(executor)
	if removedSchemes.has(scheme) {
		iss.error(Issue{
			Field: field, Code: codeExecutorRemoved, StepID: stepID,
			Message:    fmt.Sprintf(cfg.SchemeMessageFormat, label, scheme),
			Suggestion: cfg.Suggestion,
		})
		return true
	}
	if scheme != cfg.ParamScheme {
		return false
	}
	q, _ := asRecord(query)
	var found []string
	for key := range q {
		if removedAIParams.has(key) {
			found = append(found, key)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		iss.error(Issue{
			Field: field, Code: codeAgenticParamRemoved, StepID: stepID,
			Message:    fmt.Sprintf(cfg.ParamsMessageFormat, label, strings.Join(found, ", ")),
			Suggestion: cfg.Suggestion,
		})
	}
	return false
}

// validateRemovedFlowKeys refuses the flow-level keys that belonged to a removed
// surface (field = the key itself).
func validateRemovedFlowKeys(flow doc, iss *issues) {
	cfg := spec.RemovedAgentSurfaces
	for _, key := range cfg.FlowKeys {
		if has(flow, key) {
			iss.error(Issue{
				Field: key, Code: codeExecutorRemoved,
				Message:    fmt.Sprintf(cfg.FlowKeyMessageFormat, key),
				Suggestion: cfg.Suggestion,
			})
		}
	}
}
