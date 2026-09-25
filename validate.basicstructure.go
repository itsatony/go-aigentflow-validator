package aifvalidate

import (
	"fmt"
	"strings"
)

// validateBasicStructure checks the required top-level fields, the step-map
// shape, the per-step executor requirement, and the reserved-character rule on
// step IDs.
//
// Mirrors validateBasicStructure (validation.go) and the structural head of
// FlowParser.ValidateFlow (parser.go). Because YAML can produce any shape, it
// also emits invalid_type where the Go struct decoder would have failed at
// unmarshal time.
func validateBasicStructure(flow doc, iss *issues) {
	if !isNonEmptyString(get(flow, keyAigentflowVersion)) {
		iss.error(Issue{
			Field: keyAigentflowVersion, Code: codeMissingField,
			Message:    "AIgentFlow version is required",
			Suggestion: `Add 'aigentflow_version: "2.0.0"' to your flow`,
		})
	}
	if !isNonEmptyString(get(flow, keyName)) {
		iss.error(Issue{
			Field: keyName, Code: codeMissingField,
			Message:    "Flow name is required",
			Suggestion: "Add a descriptive name to your flow",
		})
	}
	start, startOK := getString(flow, keyStart)
	if !startOK || start == "" {
		iss.error(Issue{
			Field: keyStart, Code: codeMissingField,
			Message:    "Start step is required",
			Suggestion: "Specify which step should execute first",
		})
	}

	rawSteps := get(flow, keySteps)
	if rawSteps == nil {
		iss.error(Issue{
			Field: keySteps, Code: codeMissingField,
			Message:    "At least one step is required",
			Suggestion: "Define steps for your workflow",
		})
		return
	}
	steps, ok := asRecord(rawSteps)
	if !ok {
		iss.error(Issue{
			Field: keySteps, Code: codeInvalidType,
			Message: "steps must be a mapping of step ID to step definition",
		})
		return
	}
	if len(steps) == 0 {
		iss.error(Issue{
			Field: keySteps, Code: codeMissingField,
			Message:    "At least one step is required",
			Suggestion: "Define steps for your workflow",
		})
		return
	}

	names := stepNames(steps)

	if startOK && start != "" && !has(steps, start) {
		iss.error(Issue{
			Field: keyStart, Code: codeStepNotFound,
			Message:    fmt.Sprintf("Start step '%s' not found in steps", start),
			Context:    "Available steps: " + joinNames(names),
			Suggestion: "Change start to one of: " + joinNames(names),
		})
	}

	for _, stepID := range names {
		if strings.Contains(stepID, reservedStepIDChar) {
			iss.error(Issue{
				Field: stepField(stepID), Code: codeReservedStepID, StepID: stepID,
				Message: fmt.Sprintf("Step ID '%s' must not contain '%s' (reserved for loop sub-step IDs)",
					stepID, reservedStepIDChar),
			})
		}

		// "orchestrator" is the engine-reserved step ID for orchestrator-originated
		// signals and a reserved next: marker (AIF v2.484.0, DC-COND-1): a worker
		// step with this ID could have its soft completion coerced into the
		// mission-complete lifecycle key. The reference refuses it at save.
		if stepID == nextMarkerOrch {
			iss.error(Issue{
				Field: stepField(stepID), Code: codeReservedStepIDOrch, StepID: stepID,
				Message:    fmt.Sprintf("Step ID '%s' is reserved by the engine", stepID),
				Suggestion: "Rename the step; 'orchestrator' is the orchestrator's own ID and a next: marker",
			})
		}

		step, isMap := asRecord(steps[stepID])
		if !isMap {
			iss.error(Issue{
				Field: stepField(stepID), Code: codeInvalidType, StepID: stepID,
				Message: fmt.Sprintf("Step '%s' must be a mapping", stepID),
			})
			continue
		}

		// A loop step defines sub-steps instead of its own executor.
		if present(step, keyLoop) {
			continue
		}
		executor := get(step, keyExecutor)
		switch executor {
		case nil, "":
			iss.error(Issue{
				Field: stepField(stepID, keyExecutor), Code: codeMissingField, StepID: stepID,
				Message:    "Executor is required for each step",
				Suggestion: "Specify an executor URL (e.g., 'function://demo/processor')",
			})
		default:
			if _, isStr := asString(executor); !isStr {
				iss.error(Issue{
					Field: stepField(stepID, keyExecutor), Code: codeInvalidType, StepID: stepID,
					Message: "Executor must be a string URL",
				})
			}
		}
	}
}
