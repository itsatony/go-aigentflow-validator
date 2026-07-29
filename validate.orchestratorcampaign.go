package aifvalidate

import "fmt"

// validateOrchestratorCampaign checks the orchestrator and campaign blocks.
//
// Mirrors the portable parts of validateOrchestrator /
// validateAndNormalizeCampaign (parser.go). The `exons` spec body is parsed by
// the go-exons engine in the reference and is NOT re-implemented here — its
// PRESENCE is required, its contents are out of scope for a static validator.
// See PARITY.md.
func validateOrchestratorCampaign(flow doc, iss *issues, opts Options) {
	orch, hasOrchestrator := getRecord(flow, keyOrchestrator)

	if hasOrchestrator {
		if !isNonEmptyString(get(orch, keyExons)) {
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyExons, Code: codeOrchExonsRequired,
				Message: "orchestrator requires an exons specification",
			})
		}

		// DC-COND-1 termination authority. An absent/empty mode defaults to
		// monitor, which is valid.
		mode, hasMode := getString(orch, keyMode)
		switch {
		case hasMode && mode != "" && !orchestratorModes.has(mode):
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyMode, Code: codeOrchModeInvalid,
				Message:    fmt.Sprintf("Invalid orchestrator mode '%s'", mode),
				Suggestion: "Use one of: " + joinNames(sortedSet(orchestratorModes)),
			})
		case mode == orchestratorModeOwn && !flowHasOrchestratorYieldEdge(flow):
			// owner mode cedes the lifecycle to the LLM, reachable only via an
			// explicit `next: orchestrator` yield edge. A self-terminating DAG under
			// owner would hand over authority that nothing ever invokes.
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyMode, Code: codeOrchOwnerNeedsYield,
				Message: "orchestrator mode 'owner' requires at least one step with next: 'orchestrator' " +
					"(an explicit yield edge); a self-terminating DAG must use mode 'monitor'",
			})
		}

		if triggers, ok := getSlice(orch, keyTriggers); ok {
			for i, raw := range triggers {
				trigger, isMap := asRecord(raw)
				if !isMap {
					continue
				}
				validateOrchestratorTrigger(trigger, indexed(keyOrchestrator+"."+keyTriggers, i), iss)
			}
		}

		if tools, ok := getSlice(orch, keyTools); ok {
			for i, raw := range tools {
				tool, isStr := asString(raw)
				if !isStr || orchestratorTools.has(tool) {
					continue
				}
				finding := Issue{
					Field:   indexed(keyOrchestrator+"."+keyTools, i),
					Code:    codeOrchToolUnknown,
					Message: fmt.Sprintf("Unrecognised orchestrator tool name '%s'", tool),
				}
				// The vendored tool allow-list can lag the live registry, so this is a
				// warning unless the caller opted into strict registries.
				if opts.StrictRegistries {
					iss.error(finding)
				} else {
					iss.warn(finding)
				}
			}
		}

		if agentic, ok := asBool(get(orch, keyAgentic)); ok && !agentic {
			iss.warn(Issue{
				Field: keyOrchestrator + "." + keyAgentic, Code: codeOrchNotAgentic,
				Message: "orchestrator is most useful with agentic: true",
			})
		}
	}

	if !present(flow, keyCampaign) {
		return
	}
	if !hasOrchestrator {
		iss.error(Issue{
			Field: keyCampaign, Code: codeCampaignNeedsOrch,
			Message: "campaign requires an orchestrator block",
		})
	}
	// DC-COND-2: on_children_complete, when set, must name a real step — the
	// engine routes into it deterministically once every child is terminal.
	campaign, isMap := getRecord(flow, keyCampaign)
	if !isMap {
		return
	}
	target, ok := getString(campaign, keyOnChildrenComplete)
	if !ok || target == "" {
		return
	}
	steps := stepsOf(flow)
	if steps == nil {
		steps = doc{}
	}
	if !has(steps, target) {
		iss.error(Issue{
			Field: keyCampaign + "." + keyOnChildrenComplete, Code: codeCampaignHandoffStep,
			Message: fmt.Sprintf("campaign.on_children_complete references unknown step '%s'", target),
		})
	}
}

func validateOrchestratorTrigger(trigger doc, base string, iss *issues) {
	ttype, isStr := getString(trigger, keyType)
	if !isStr || !orchestratorTriggers.has(ttype) {
		iss.error(Issue{
			Field: base + "." + keyType, Code: codeOrchTriggerUnknown,
			Message:    fmt.Sprintf("Unknown orchestrator trigger type '%v'", get(trigger, keyType)),
			Suggestion: "Use one of: " + joinNames(sortedSet(orchestratorTriggers)),
		})
		return
	}
	if ttype != triggerTimer {
		return
	}
	interval, hasInterval := getString(trigger, keyInterval)
	switch {
	case !hasInterval || interval == "":
		iss.error(Issue{
			Field: base + "." + keyInterval, Code: codeOrchTimerNoInterval,
			Message: "timer trigger requires an interval",
		})
	case !isValidGoDuration(interval):
		iss.error(Issue{
			Field: base + "." + keyInterval, Code: codeOrchTimerBadInterv,
			Message: fmt.Sprintf("timer trigger interval '%s' is not a valid duration", interval),
		})
	}
}

// flowHasOrchestratorYieldEdge mirrors the Go helper: does any step route to the
// orchestrator via next.default or a conditional goto? (DC-COND-1)
func flowHasOrchestratorYieldEdge(flow doc) bool {
	steps := stepsOf(flow)
	if steps == nil {
		return false
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		next, ok := getRecord(step, keyNext)
		if !ok {
			continue
		}
		if routesToOrchestrator(next) {
			return true
		}
	}
	return false
}
