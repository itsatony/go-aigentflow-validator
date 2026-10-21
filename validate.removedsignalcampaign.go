package aifvalidate

import (
	"fmt"
	"strings"
)

// The flow surfaces AIgentFlow removed after its own agent chains: agent
// signals and dynamic flow modification (v2.834.0), campaigns (v2.836.0) and
// the orchestrator (v2.838.0). The vocabulary lives in the spec blocks
// removedSignalSurfaces, removedCampaignSurfaces and
// removedOrchestratorSurfaces; the reference is aigentflow.removed.*surfaces.go.
//
// Four ERROR codes (signal_removed, flow_modification_removed,
// campaign_removed, orchestrator_removed) and one WARNING
// (removed_key_ignored, for checkpoint.pre_dfm, which AIgentFlow may have
// written into a stored flow itself). The removed blocks are judged on
// presence and refused WHOLE: nothing inside the orchestrator: or campaign:
// block is judged, and both stay in knownKeys typed `any`, so each surface is
// reported once, by its own code, with its advice.
//
// Order, as in the reference: the step findings of signals / DFM, the campaign
// block, the orchestrator block, the flow-level discovery keys, then the steps
// that route next to the orchestrator (by id). The ignored-key warning is
// separate (warnings are listed apart from errors anyway).
func validateRemovedOrchestratorSurfaces(flow doc, iss *issues) {
	validateRemovedSignalStepKeys(flow, iss)

	camp := spec.RemovedCampaignSurfaces
	if present(flow, camp.FlowKey) {
		iss.error(Issue{
			Field: camp.FlowKey, Code: codeCampaignRemoved,
			Message: camp.BlockMessage, Suggestion: camp.Suggestion,
		})
	}

	validateRemovedOrchestrator(flow, iss)

	// The inert key: a WARNING, never an error. A *bool in the reference, so a
	// YAML null is absent.
	if checkpoint, ok := getRecord(flow, keyCheckpoint); ok && get(checkpoint, keyPreDFM) != nil {
		sig := spec.RemovedSignalSurfaces
		field := keyCheckpoint + "." + keyPreDFM
		iss.warn(Issue{
			Field: field, Code: codeRemovedKeyIgnored,
			Message:    fmt.Sprintf(sig.IgnoredKeyMessageFormat, field),
			Suggestion: sig.IgnoredKeySuggestion,
		})
	}
}

// validateRemovedSignalStepKeys reports the two removed step keys:
// flow_modification_allowed on any step, enable_signals on an aiv:// step
// (ai:// reports it as agentic_param_removed). Judged on map presence. Skipped:
// templated executors, removed schemes (the step is executor_removed already)
// and flow:// (its query is the child's inputs).
func validateRemovedSignalStepKeys(flow doc, iss *issues) {
	sig := spec.RemovedSignalSurfaces
	forEachExecutorSite(flow, iss, func(executor string, query any, field, stepID string) {
		if strings.Contains(executor, templateOpenDelim) {
			return
		}
		scheme := removedExecutorScheme(executor)
		if removedSchemes.has(scheme) || removedStepSkipSchemes.has(scheme) {
			return
		}
		q, _ := asRecord(query)
		if has(q, sig.FlowModificationStepKey) {
			iss.error(Issue{
				Field: field, Code: codeFlowModificationRemoved, StepID: stepID,
				Message:    fmt.Sprintf(sig.FlowModificationStepMessageFormat, stepID),
				Suggestion: sig.Suggestion,
			})
		}
		if scheme == sig.StepKeyScheme && has(q, sig.StepKey) {
			iss.error(Issue{
				Field: field, Code: codeSignalRemoved, StepID: stepID,
				Message:    fmt.Sprintf(sig.StepKeyMessageFormat, stepID),
				Suggestion: sig.Suggestion,
			})
		}
	})
}

// validateRemovedOrchestrator reports orchestrator_removed: the block (any
// non-null value, a scalar or list included), each flow-level discovery key
// (any non-null value, "" and [] included), and every top-level step whose
// next.default or a condition's goto is the literal `orchestrator` (one finding
// per step). Loop sub-steps are not judged here: their next refuses the marker
// already (loop_substep_next_sentinel).
func validateRemovedOrchestrator(flow doc, iss *issues) {
	cfg := spec.RemovedOrchestratorSurfaces
	if present(flow, cfg.BlockKey) {
		iss.error(Issue{
			Field: cfg.BlockKey, Code: codeOrchestratorRemoved,
			Message: cfg.BlockMessage, Suggestion: cfg.Suggestion,
		})
	}
	for _, key := range cfg.DiscoveryKeys {
		if present(flow, key) {
			iss.error(Issue{
				Field: key, Code: codeOrchestratorRemoved,
				Message:    fmt.Sprintf(cfg.FlowKeyMessageFormat, key),
				Suggestion: cfg.Suggestion,
			})
		}
	}
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		next, ok := getRecord(step, keyNext)
		if !ok || !routesTo(next, cfg.NextMarker) {
			continue
		}
		iss.error(Issue{
			Field: stepField(stepID), Code: codeOrchestratorRemoved, StepID: stepID,
			Message:    fmt.Sprintf(cfg.NextMessageFormat, stepID),
			Suggestion: cfg.Suggestion,
		})
	}
}

// routesTo reports whether a next block names target, exactly, as its default
// or as any condition's goto. (A condition's target key is `goto`, never the
// reference's Go field name `goto_step`.)
func routesTo(next doc, target string) bool {
	if value, ok := getString(next, keyDefault); ok && value == target {
		return true
	}
	conds, ok := getSlice(next, keyConditions)
	if !ok {
		return false
	}
	for _, raw := range conds {
		cond, isMap := asRecord(raw)
		if !isMap {
			continue
		}
		if value, ok := getString(cond, keyGoto); ok && value == target {
			return true
		}
	}
	return false
}

// forEachExecutorSite calls fn for every step and loop sub-step that carries a
// non-empty string executor, steps by id and each step's sub-steps after it.
// field is the step itself (`steps.<id>`, `steps.<id>.loop.steps[<index>]`);
// stepID is the composite id for a sub-step.
func forEachExecutorSite(flow doc, iss *issues, fn func(executor string, query any, field, stepID string)) {
	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if executor, isStr := get(step, keyExecutor).(string); isStr && executor != "" {
			fn(executor, get(step, keyQuery), stepField(stepID), stepID)
		}
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
			if !isMap {
				continue
			}
			executor, isStr := get(sub, keyExecutor).(string)
			if !isStr || executor == "" {
				continue
			}
			fn(executor, get(sub, keyQuery), stepField(stepID, keyLoop, indexed(keySteps, i)),
				subStepID(stepID, sub, i, iss))
		}
	}
}
