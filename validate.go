package aifvalidate

// ValidateFlow parses flow YAML and validates it.
//
// Parse failures (syntax, duplicate keys, non-mapping root) come back as
// error-severity findings and the structural validators are skipped, because
// there is no usable document to inspect.
//
// It never panics on malformed input, and never performs I/O. There is
// deliberately NO recover() at this boundary: a panic here would be a port bug,
// and swallowing it would report a broken document as valid — on a publish gate
// that is the worst possible failure. Every validator narrows `any` explicitly
// instead.
func ValidateFlow(yamlText string, opts Options) Result {
	flow, parseErrors, parseWarnings := ParseFlow(yamlText)
	if len(parseErrors) > 0 || flow == nil {
		return Result{
			Valid:       false,
			Errors:      parseErrors,
			Warnings:    parseWarnings,
			SpecVersion: spec.SpecVersion,
			Summary: Summary{
				ErrorCount:   len(parseErrors),
				WarningCount: len(parseWarnings),
			},
		}
	}

	result := ValidateFlowObject(flow, opts)

	// Non-fatal YAML warnings sit alongside the validation warnings.
	if len(parseWarnings) > 0 {
		result.Warnings = append(parseWarnings, result.Warnings...)
		result.Summary.WarningCount = len(result.Warnings)
	}

	// Resolve every finding's field path to a source position. This is why
	// ValidateFlow is preferable to ValidateFlowObject for an authoring surface:
	// the object form has no source to locate findings in.
	idx := buildPositionIndex(yamlText)
	applyPositions(idx, result.Errors)
	applyPositions(idx, result.Warnings)

	return result
}

// ValidateFlowObject validates an already-decoded flow mapping. Use it when the
// document came from a caller's own loader; use ValidateFlow to parse and
// validate in one step and to get source positions on findings.
func ValidateFlowObject(flow map[string]any, opts Options) Result {
	iss := newIssues()

	if flow == nil {
		iss.error(Issue{
			Field: "", Code: codeInvalidRoot,
			Message: "Flow must be a mapping (object) at the top level",
		})
		return finish(iss, doc{}, templateStats{})
	}

	// Order matters only for readability of the output: basicStructure first so a
	// document missing its identity reports that before anything else. No
	// validator depends on another's findings.
	validateBasicStructure(flow, iss)
	validateRetiredKeys(flow, iss)
	validateExecutors(flow, iss)
	validateQuerySchema(flow, iss)
	validateResponseExpectations(flow, iss)
	validateErrorStrategies(flow, iss)
	validateConnectivity(flow, iss)
	validateNextLogic(flow, iss)
	validateExpressionFunctions(flow, iss)
	validateLoopForEachThrottle(flow, iss)
	validateOrchestratorCampaign(flow, iss, opts)
	validateCredentialBindings(flow, iss)
	validateInputSchema(flow, iss)
	validateOutputSchemas(flow, iss)
	validateQualityGates(flow, iss)
	validateProcessingOperations(flow, iss)
	validateStepMaxDuration(flow, iss)
	validateSaveDoorExtras(flow, iss)
	stats := validateTemplates(flow, iss, opts)

	return finish(iss, flow, stats)
}

// finish assembles the Result and its summary.
func finish(iss *issues, flow doc, stats templateStats) Result {
	totalSteps := 0
	if steps := stepsOf(flow); steps != nil {
		totalSteps = len(steps)
	}

	// A step is "valid" when no error names it. Steps with only warnings count as
	// valid, matching Valid's own errors-only rule.
	withErrors := make(map[string]struct{})
	for _, e := range iss.errors {
		if e.StepID != "" {
			withErrors[e.StepID] = struct{}{}
		}
	}

	return Result{
		Valid:       len(iss.errors) == 0,
		Errors:      iss.errors,
		Warnings:    iss.warnings,
		SpecVersion: spec.SpecVersion,
		Summary: Summary{
			TotalSteps:     totalSteps,
			ValidSteps:     max(0, totalSteps-len(withErrors)),
			ErrorCount:     len(iss.errors),
			WarningCount:   len(iss.warnings),
			TemplatesFound: stats.found,
			TemplatesValid: max(0, stats.found-stats.syntaxErrors),
		},
	}
}
