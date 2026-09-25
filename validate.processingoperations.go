package aifvalidate

import (
	"fmt"
	"sort"
)

// processingScope says WHERE a processing operation is declared, because the
// dispatch set is not the same in both places.
//
// scopeStandard is the top-level handler. scopeLoopSubStep is the loop
// sub-step's post-processing, which dispatches loop.set / loop.break itself and
// falls through to the standard handler for everything else. ⛔ The two are a
// PARTITION, never a union: loop.set at the top level is an operation the
// standard handler cannot dispatch.
type processingScope int

const (
	scopeStandard processingScope = iota
	scopeLoopSubStep
)

// processingOpRef is one entry of a pre_processing: / post_processing: list,
// decomposed the way the reference's custom unmarshaller decomposes it.
//
// An operation is written as a single-key map (`- data.set: { … }`) plus an
// optional `if:` guard. The reference unmarshals the key into OperationType and
// its body into an INLINE Config, which is why the operation name is NOT a path
// segment in any finding: the reference addresses
// `steps.<id>.post_processing[0].<configKey>`.
type processingOpRef struct {
	stepID   string
	basePath string // steps.<id>.post_processing[0]
	raw      any
	guard    any
	hasGuard bool
	// entries holds every non-guard key as written, in sorted order. The
	// reference refuses a map with more than one at unmarshal time; this port
	// has no typed unmarshal, so each rule decides what it can say.
	entries []processingEntry
	scope   processingScope
}

type processingEntry struct {
	key   string
	value any
}

// processingPhases are the two lists an operation can be declared in, in
// execution order.
var processingPhases = []string{keyPreProcessing, keyPostProcessing}

// dispatchableOperation reports whether the handler for scope dispatches
// operationType.
func dispatchableOperation(scope processingScope, operationType string) bool {
	if processingStandardTypes.has(operationType) {
		return true
	}
	return scope == scopeLoopSubStep && processingLoopSubStepTypes.has(operationType)
}

func dispatchableOperationList(scope processingScope) []string {
	out := sortedSet(processingStandardTypes)
	if scope == scopeLoopSubStep {
		out = append(out, sortedSet(processingLoopSubStepTypes)...)
		sort.Strings(out)
	}
	return out
}

// processingConfigKeys returns the top-level config keys an operation reads and
// whether that set is CLOSED. A false second value means there is no basis for
// a verdict about a key: either the author chooses the key names (data.set,
// output.set, conversation.append, loop.set), or the type is not dispatchable.
func processingConfigKeys(operationType string) (set, bool) {
	if processingOpenKeyTypes.has(operationType) {
		return nil, false
	}
	keys, ok := spec.ProcessingOperations.ClosedConfigKeys[operationType]
	if !ok {
		return nil, false
	}
	return newSet(keys), true
}

// loopSubStepRef is one sub-step of a loop: body with the path the reference
// addresses it by.
type loopSubStepRef struct {
	basePath string // steps.<parent>.loop.steps[i]
	raw      doc
}

// loopSubStepsOf enumerates a loop step's sub-steps. Empty for a step with no
// loop: block.
func loopSubStepsOf(stepID string, step doc) []loopSubStepRef {
	loop, ok := getRecord(step, keyLoop)
	if !ok {
		return nil
	}
	subs, ok := getSlice(loop, keySteps)
	if !ok {
		return nil
	}
	var out []loopSubStepRef
	for i, raw := range subs {
		sub, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		out = append(out, loopSubStepRef{basePath: stepField(stepID, keyLoop, indexed(keySteps, i)), raw: sub})
	}
	return out
}

// processingOpsOfStep decomposes both phases of a top-level step.
func processingOpsOfStep(stepID string, step doc) []processingOpRef {
	return decomposeProcessingPhases(stepID, step, stepField(stepID), scopeStandard)
}

// loopSubStepProcessingOps decomposes the processing operations of every
// sub-step of a loop: step. Findings are attributed to the PARENT step id; the
// sub-step is named by its index in the path, as in the reference.
func loopSubStepProcessingOps(stepID string, step doc) []processingOpRef {
	var out []processingOpRef
	for _, sub := range loopSubStepsOf(stepID, step) {
		out = append(out, decomposeProcessingPhases(stepID, sub.raw, sub.basePath, scopeLoopSubStep)...)
	}
	return out
}

func decomposeProcessingPhases(stepID string, container doc, prefix string, scope processingScope) []processingOpRef {
	var out []processingOpRef
	guardKey := spec.ProcessingOperations.GuardKey
	for _, phase := range processingPhases {
		ops, ok := getSlice(container, phase)
		if !ok {
			continue
		}
		for i, op := range ops {
			ref := processingOpRef{
				stepID: stepID, basePath: fmt.Sprintf("%s.%s", prefix, indexed(phase, i)),
				raw: op, scope: scope,
			}
			if m, isMap := asRecord(op); isMap {
				for _, key := range sortedKeys(m) {
					if key == guardKey {
						ref.guard, ref.hasGuard = m[key], true
						continue
					}
					ref.entries = append(ref.entries, processingEntry{key: key, value: m[key]})
				}
			}
			out = append(out, ref)
		}
	}
	return out
}

// validateProcessingOperations warns about an operation type the handler cannot
// dispatch (unknown_processing_operation) and about a config key its handler
// never reads (unknown_processing_config_key). Mirrors
// validateProcessingOperationShape (AIF v2.647.0; loop body since v2.648.0).
//
// Both are WARNINGS in the reference and here: a flow carrying either saves and
// runs. An undispatchable type fails at run time with "unsupported operation
// type"; an unread key is silently ignored, so `transformation:` written where
// the handler reads `transformer:` gives a step that succeeds and does nothing.
//
// Scope: top-level config keys only. Sub-keys of `metadata:` and `parameters:`
// are out of scope in both ports.
func validateProcessingOperations(flow doc, iss *issues) {
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		refs := append(processingOpsOfStep(stepID, step), loopSubStepProcessingOps(stepID, step)...)
		for _, ref := range refs {
			// The reference refuses an entry that is not a one-key map at
			// unmarshal time; there is no operation to judge (divergence #11).
			if len(ref.entries) != 1 {
				continue
			}
			operationType, config := ref.entries[0].key, ref.entries[0].value

			if !dispatchableOperation(ref.scope, operationType) {
				// The operation type is the YAML MAP KEY, not a field, so the
				// finding addresses the operation itself. One verdict per defect:
				// an unknown type's keys are not judged as well.
				iss.warn(Issue{
					Field: ref.basePath, Code: codeUnknownProcessingOp, StepID: ref.stepID,
					Message: fmt.Sprintf("Processing operation type '%s' is not dispatched by the engine; at run "+
						"time the step fails with \"unsupported operation type\" unless the operation's `if:` "+
						"guard is false", operationType),
					Suggestion: "Operation types the engine dispatches here: " + joinNames(dispatchableOperationList(ref.scope)),
				})
				continue
			}

			allowed, closed := processingConfigKeys(operationType)
			if !closed {
				continue
			}
			configMap, isMap := asRecord(config)
			if !isMap {
				continue // a non-map config fails to unmarshal upstream; not a key question
			}
			readable := joinNames(sortedSet(allowed))
			for _, key := range sortedKeys(configMap) {
				if allowed.has(key) {
					continue
				}
				iss.warn(Issue{
					Field: ref.basePath + "." + key, Code: codeUnknownProcessingConfigKey, StepID: ref.stepID,
					Message: fmt.Sprintf("Operation '%s' does not read the config key '%s', so it is silently "+
						"ignored at run time", operationType, key),
					Suggestion: fmt.Sprintf("'%s' reads: %s", operationType, readable),
				})
			}
		}
	}
}
