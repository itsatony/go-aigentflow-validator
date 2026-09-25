package aifvalidate

import "fmt"

// templateStats is the counter threaded through the walk for Summary.
type templateStats struct {
	found        int
	syntaxErrors int
}

// validateTemplates checks Go-template syntax across a step's templated fields.
//
// Mirrors validateTemplates / validateStepTemplates / countTemplates
// (validation.go): walk the string leaves of `query`, `pre_processing`,
// `post_processing`, and `next.conditions[].if`; anything containing "{{" goes
// through the template syntax checker. Runtime field-resolution warnings (the
// reference's execution pass) are intentionally NOT reproduced — see PARITY.md.
//
// Since v0.3.0 (reference v2.648.0, DC-FORGE-78) the LOOP BODY is walked too:
// each sub-step's condition, query, processing operations and
// next.conditions[].if. Every finding from inside a loop body is a WARNING,
// never an error: the reference consults this validator at its RUN door over
// flows stored before the walk existed, and inside a loop body four of the six
// evaluation sites swallow a template failure. The CODE survives the demotion.
func validateTemplates(flow doc, iss *issues, opts Options) templateStats {
	stats := templateStats{}
	steps := stepsOf(flow)
	if steps == nil {
		return stats
	}

	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		w := templateWalker{stepID: stepID, iss: iss, stats: &stats, opts: opts}

		if has(step, keyQuery) {
			w.walk(stepField(stepID, keyQuery), get(step, keyQuery), true)
		}
		for _, ref := range processingOpsOfStep(stepID, step) {
			w.walkProcessingOperation(ref)
		}
		for _, sub := range loopSubStepsOf(stepID, step) {
			w.walkLoopSubStep(sub)
		}
		for _, ref := range loopSubStepProcessingOps(stepID, step) {
			w.walkProcessingOperation(ref)
		}
		// response_expectation templates are COUNTED (matching countTemplates) but
		// not syntax-checked (matching validateStepTemplates).
		if has(step, keyResponseExpectation) {
			w.walk(stepField(stepID, keyResponseExpectation), get(step, keyResponseExpectation), false)
		}
		// next.conditions[].if expressions are syntax-checked.
		w.checkConditions(stepField(stepID, keyNext), step)
	}
	return stats
}

// templateWalker carries the per-step state of the template walk. demote marks
// a walk inside a loop body, where every finding is a warning.
type templateWalker struct {
	stepID string
	iss    *issues
	stats  *templateStats
	opts   Options
	demote bool
}

// walk visits every string leaf of an arbitrary value, mirroring the
// reference's walkObjectRecursive. When check is false the leaves are only
// counted, not validated.
func (w templateWalker) walk(basePath string, value any, check bool) {
	switch v := value.(type) {
	case string:
		if check {
			w.check(v, basePath)
		} else if isTemplate(v) {
			w.stats.found++
		}
	case []any:
		for i, item := range v {
			w.walk(fmt.Sprintf("%s[%d]", basePath, i), item, check)
		}
	case doc:
		for _, key := range sortedKeys(v) {
			w.walk(basePath+"."+key, v[key], check)
		}
	}
}

// checkConditions syntax-checks a `next.conditions[].if` list under nextPath.
func (w templateWalker) checkConditions(nextPath string, owner doc) {
	next, ok := getRecord(owner, keyNext)
	if !ok {
		return
	}
	conds, ok := getSlice(next, keyConditions)
	if !ok {
		return
	}
	for i, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		expr, ok := getString(cond, keyIf)
		if !ok || expr == "" {
			continue
		}
		// Counted toward found so a syntax error in a condition cannot push
		// TemplatesValid above TemplatesFound.
		w.stats.found++
		w.check(expr, fmt.Sprintf("%s.%s.%s", nextPath, indexed(keyConditions, i), keyIf))
	}
}

// walkProcessingOperation walks one processing operation, producing the field
// paths the REFERENCE produces: the operation name is absorbed into
// OperationType and its body is inline, so a finding is addressed
// `…post_processing[0].<configKey>`, not `…post_processing[0].data.set.<configKey>`.
func (w templateWalker) walkProcessingOperation(ref processingOpRef) {
	w.demote = ref.scope == scopeLoopSubStep
	if _, isMap := asRecord(ref.raw); !isMap {
		w.walk(ref.basePath, ref.raw, true)
		return
	}
	if ref.hasGuard {
		w.walk(ref.basePath+"."+spec.ProcessingOperations.GuardKey, ref.guard, true)
	}
	for _, entry := range ref.entries {
		w.walk(ref.basePath, entry.value, true)
	}
}

// walkLoopSubStep walks one loop sub-step's own templates — condition:, query:
// and next.conditions[].if. Its processing operations come through
// walkProcessingOperation like any other.
func (w templateWalker) walkLoopSubStep(sub loopSubStepRef) {
	w.demote = true
	if condition, ok := getString(sub.raw, keyCondition); ok && condition != "" {
		w.check(condition, sub.basePath+"."+keyCondition)
	}
	if has(sub.raw, keyQuery) {
		w.walk(sub.basePath+"."+keyQuery, get(sub.raw, keyQuery), true)
	}
	w.checkConditions(sub.basePath+"."+keyNext, sub.raw)
}

func (w templateWalker) check(value, field string) {
	if !isTemplate(value) {
		return
	}
	w.stats.found++
	report := w.iss.error
	if w.demote {
		report = w.iss.warn
	}
	for _, problem := range checkGoTemplateSyntax(value) {
		if problem.isFunctionError {
			// Divergence #4: an unknown function is a WARNING by default, because
			// the vendored allow-list can lag the live registry; an error only
			// under StrictRegistries (and never inside a loop body).
			reportFunction := w.iss.warn
			if w.opts.StrictRegistries {
				reportFunction = report
			}
			reportFunction(Issue{
				Field: field, Code: codeTemplateFuncUnkn, StepID: w.stepID,
				Message: "Template " + problem.message,
				Context: "Template: " + value,
			})
			continue
		}
		w.stats.syntaxErrors++
		report(Issue{
			Field: field, Code: codeTemplateSyntax, StepID: w.stepID,
			Message:    "Template syntax error: " + problem.message,
			Context:    "Template: " + value,
			Suggestion: "Check Go template syntax: https://pkg.go.dev/text/template",
		})
	}
}
