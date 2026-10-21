package aifvalidate

import (
	"fmt"
	"slices"
	"strings"
)

// The orchestrator surfaces AIgentFlow removed after its own agent chains:
// agent signals and dynamic flow modification (v2.834.0, AGENTCUT B1) and
// campaigns (v2.836.0, AGENTCUT B2). The vocabulary lives in the spec blocks
// removedSignalSurfaces and removedCampaignSurfaces; the reference is
// aigentflow.removed.signalsurfaces.go / .campaignsurfaces.go.
//
// Three ERROR codes (signal_removed, flow_modification_removed,
// campaign_removed) and one WARNING (removed_key_ignored, for the two inert
// keys AIgentFlow may have written into a stored flow itself). The removed
// tool and trigger names left the registries but are tolerated by the
// unknown-name checks, and the removed block keys stay in knownKeys, so each
// surface is reported once, by its own code, with its advice.
//
// Order, as in the reference: the signal findings on the orchestrator block,
// the campaign findings (block, lists, triggers), then the step findings by
// step id with loop sub-steps after their parent. The ignored-key warnings are
// separate (warnings are listed apart from errors anyway).
func validateRemovedOrchestratorSurfaces(flow doc, iss *issues, inspector ExonsInspector) {
	sig := spec.RemovedSignalSurfaces
	camp := spec.RemovedCampaignSurfaces
	orch, hasOrch := getRecord(flow, keyOrchestrator)
	allowNames := orchestratorToolsAllow(orch, hasOrch, inspector)

	// The four tool lists, in the reference's order. tools.allow is reported
	// at the orchestrator's exons document, as orchestrator_tool_withheld is.
	type toolList struct {
		field string
		names []string
	}
	var lists []toolList
	if hasOrch {
		for _, key := range []string{keyTools, keyDenyTools, keyRequireApprovalFor} {
			lists = append(lists, toolList{keyOrchestrator + "." + key, stringEntries(orch, key)})
		}
		lists = append(lists, toolList{keyOrchestrator + "." + keyExons, allowNames})
	}

	// B1: signals and DFM on the orchestrator block.
	if hasOrch {
		if on, ok := asBool(get(orch, keyEnableSignals)); ok && on {
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyEnableSignals, Code: codeSignalRemoved,
				Message: sig.EnableSignalsMessage, Suggestion: sig.Suggestion,
			})
		}
		for _, list := range lists {
			if named := namesIn(list.names, sig.Tools); len(named) > 0 {
				iss.error(Issue{
					Field: list.field, Code: codeSignalRemoved,
					Message:    fmt.Sprintf(sig.ToolNamesMessageFormat, list.field, strings.Join(named, ", ")),
					Suggestion: sig.Suggestion,
				})
			}
		}
		if len(namesIn(triggerTypes(orch), sig.Triggers)) > 0 {
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyTriggers, Code: codeFlowModificationRemoved,
				Message: sig.TriggersMessage, Suggestion: sig.Suggestion,
			})
		}
	}

	// B2: campaigns. The block is judged on presence (any non-null value) and
	// refused whole.
	if get(flow, camp.FlowKey) != nil {
		iss.error(Issue{
			Field: camp.FlowKey, Code: codeCampaignRemoved,
			Message: camp.BlockMessage, Suggestion: camp.Suggestion,
		})
	}
	if hasOrch {
		for _, list := range lists {
			if named := namesIn(list.names, camp.Tools); len(named) > 0 {
				iss.error(Issue{
					Field: list.field, Code: codeCampaignRemoved,
					Message:    fmt.Sprintf(camp.ToolNamesMessageFormat, list.field, strings.Join(named, ", ")),
					Suggestion: camp.Suggestion,
				})
			}
		}
		if named := namesIn(triggerTypes(orch), camp.Triggers); len(named) > 0 {
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyTriggers, Code: codeCampaignRemoved,
				Message:    fmt.Sprintf(camp.TriggersMessageFormat, strings.Join(named, ", ")),
				Suggestion: camp.Suggestion,
			})
		}
	}

	// B1: the step keys.
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

	// The inert keys: a WARNING, never an error. Both are *bool in the
	// reference, so a YAML null is absent.
	if hasOrch {
		if on, ok := asBool(get(orch, keyEnableSignals)); ok && !on {
			warnRemovedKeyIgnored(keyOrchestrator+"."+keyEnableSignals, iss)
		}
	}
	if checkpoint, ok := getRecord(flow, keyCheckpoint); ok && get(checkpoint, keyPreDFM) != nil {
		warnRemovedKeyIgnored(keyCheckpoint+"."+keyPreDFM, iss)
	}
}

func warnRemovedKeyIgnored(field string, iss *issues) {
	sig := spec.RemovedSignalSurfaces
	iss.warn(Issue{
		Field: field, Code: codeRemovedKeyIgnored,
		Message:    fmt.Sprintf(sig.IgnoredKeyMessageFormat, field),
		Suggestion: sig.IgnoredKeySuggestion,
	})
}

// orchestratorToolsAllow returns the tools.allow names of the orchestrator's
// inline .exons definition, or nil when there is none or it cannot be read.
func orchestratorToolsAllow(orch doc, hasOrch bool, inspector ExonsInspector) []string {
	if !hasOrch {
		return nil
	}
	// A plain read: a non-string exons value is reported by the orchestrator
	// rules, once.
	source, ok := get(orch, keyExons).(string)
	if !ok || source == "" {
		return nil
	}
	report := inspector.InspectExons(source)
	if report.ParseError != "" || !report.SpecRead || !report.ToolsAllowDeclared {
		return nil
	}
	return report.ToolsAllow
}

// stringEntries returns the string entries of a list-valued key, in order. A
// non-string entry is skipped: its type is another rule's finding.
func stringEntries(m doc, key string) []string {
	raw, ok := getSlice(m, key)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, isStr := v.(string); isStr {
			out = append(out, s)
		}
	}
	return out
}

// triggerTypes returns the string `type` of each orchestrator trigger, in order.
func triggerTypes(orch doc) []string {
	raw, ok := getSlice(orch, keyTriggers)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		trigger, isMap := asRecord(v)
		if !isMap {
			continue
		}
		if s, isStr := get(trigger, keyType).(string); isStr {
			out = append(out, s)
		}
	}
	return out
}

// namesIn returns the entries of names that are in removed, in list order,
// each once. Matching is exact and case-sensitive, as a tool name is.
func namesIn(names, removed []string) []string {
	var found []string
	for _, name := range names {
		if slices.Contains(removed, name) && !slices.Contains(found, name) {
			found = append(found, name)
		}
	}
	return found
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
