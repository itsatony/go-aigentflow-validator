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

		if has(step, keyQuery) {
			walkTemplateStrings(stepField(stepID, keyQuery), get(step, keyQuery),
				stepID, iss, &stats, opts, true)
		}
		for _, key := range []string{keyPreProcessing, keyPostProcessing} {
			ops, ok := getSlice(step, key)
			if !ok {
				continue
			}
			for i, op := range ops {
				walkTemplateStrings(stepField(stepID, indexed(key, i)), op,
					stepID, iss, &stats, opts, true)
			}
		}
		// response_expectation templates are COUNTED (matching countTemplates) but
		// not syntax-checked (matching validateStepTemplates).
		if has(step, keyResponseExpectation) {
			walkTemplateStrings(stepField(stepID, keyResponseExpectation),
				get(step, keyResponseExpectation), stepID, iss, &stats, opts, false)
		}
		// next.conditions[].if expressions are syntax-checked.
		next, hasNext := getRecord(step, keyNext)
		if !hasNext {
			continue
		}
		conds, hasConds := getSlice(next, keyConditions)
		if !hasConds {
			continue
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
			stats.found++
			checkTemplateString(expr,
				stepField(stepID, keyNext, indexed(keyConditions, i), keyIf),
				stepID, iss, &stats, opts)
		}
	}
	return stats
}

// walkTemplateStrings visits every string leaf of an arbitrary value, mirroring
// the reference's walkObjectRecursive. When check is false the leaves are only
// counted, not validated.
func walkTemplateStrings(basePath string, value any, stepID string,
	iss *issues, stats *templateStats, opts Options, check bool) {
	switch v := value.(type) {
	case string:
		if check {
			checkTemplateString(v, basePath, stepID, iss, stats, opts)
		} else if isTemplate(v) {
			stats.found++
		}
	case []any:
		for i, item := range v {
			walkTemplateStrings(fmt.Sprintf("%s[%d]", basePath, i), item, stepID, iss, stats, opts, check)
		}
	case doc:
		for _, key := range sortedKeys(v) {
			walkTemplateStrings(basePath+"."+key, v[key], stepID, iss, stats, opts, check)
		}
	}
}

func checkTemplateString(value, field, stepID string,
	iss *issues, stats *templateStats, opts Options) {
	if !isTemplate(value) {
		return
	}
	stats.found++
	for _, problem := range checkGoTemplateSyntax(value, opts.StrictRegistries) {
		if problem.isFunctionError {
			iss.error(Issue{
				Field: field, Code: codeTemplateFuncUnkn, StepID: stepID,
				Message: "Template " + problem.message,
				Context: "Template: " + value,
			})
			continue
		}
		stats.syntaxErrors++
		iss.error(Issue{
			Field: field, Code: codeTemplateSyntax, StepID: stepID,
			Message:    "Template syntax error: " + problem.message,
			Context:    "Template: " + value,
			Suggestion: "Check Go template syntax: https://pkg.go.dev/text/template",
		})
	}
}
