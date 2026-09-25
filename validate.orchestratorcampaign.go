package aifvalidate

import (
	"fmt"
	"math"
	"time"
)

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

		validateHumanQuestionTimeout(orch, iss)

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
	campaign, isMap := getRecord(flow, keyCampaign)
	if !isMap {
		return
	}
	validateCampaignChildFlows(campaign, iss)
	validateMaxCreditsPerChild(campaign, iss)

	// DC-COND-2: on_children_complete, when set, must name a real step — the
	// engine routes into it deterministically once every child is terminal.
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

// validateHumanQuestionTimeout ports the per-question HITL deadline refusal
// (AIF v2.695.0, DC-FORGE-125). The reference REFUSES an unparseable or
// non-positive duration. Note the positivity half: `-5m` is a well-formed Go
// duration, and accepting it would make this oracle looser than its door.
//
// Empty and null mean ABSENT, as in the reference: its field is a Go string,
// null decodes to "", and the check runs only on a non-empty value.
//
// NOT ported: the reference also LOGS (not a validation warning) when the
// timeout exceeds its orchestrator mission clock. That threshold is a
// deployment-side constant this package cannot observe. See PARITY.md.
func validateHumanQuestionTimeout(orch doc, iss *issues) {
	declared := get(orch, keyHumanQuestionTimeout)
	if declared == nil {
		return
	}
	if s, ok := asString(declared); ok {
		if s == "" {
			return
		}
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return
		}
	}
	iss.error(Issue{
		Field: keyOrchestrator + "." + keyHumanQuestionTimeout, Code: codeOrchHumanQTimeoutInvalid,
		Message: fmt.Sprintf("human_question_timeout '%v' is not a valid positive duration", declared),
	})
}

// validateMaxCreditsPerChild ports campaign.max_credits_per_child (AIF
// v2.728.0, DC-FORGE-155). It is a Go int64 and CampaignConfig.Validate
// refuses a negative value. Absent, null and 0 mean "no per-child cap".
//
// ⚠ A FRACTIONAL number is ACCEPTED, because the reference accepts it: yaml.v3
// truncates a float into an int64 field (1.5 decodes as 1, -0.5 as 0), measured
// against the reference's save door. Refusing it here would make this oracle
// stricter than its door. A string or boolean does not decode (invalid_type).
func validateMaxCreditsPerChild(campaign doc, iss *issues) {
	raw := get(campaign, keyMaxCreditsPerChild)
	if raw == nil {
		return
	}
	field := keyCampaign + "." + keyMaxCreditsPerChild
	n, isNum := int64OfYAMLNumber(raw)
	switch {
	case !isNum:
		iss.error(Issue{
			Field: field, Code: codeInvalidType,
			Message: fmt.Sprintf("campaign.max_credits_per_child must be a whole number of credits, got '%v'", raw),
		})
	case n < 0:
		iss.error(Issue{
			Field: field, Code: codeCampaignMaxCreditsChild,
			Message: "campaign: max_credits_per_child must be >= 0",
		})
	}
}

// maxInt64AsFloat is 2^63: a YAML float at or above it cannot decode into an
// int64 field.
const maxInt64AsFloat = float64(1 << 63)

// int64OfYAMLNumber decodes a YAML number the way yaml.v3 decodes it into an
// int64 field: integers exactly (a uint64 beyond int64 fails), floats truncated
// toward zero when in range. Anything else does not decode.
func int64OfYAMLNumber(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case uint64:
		if x > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	case float64:
		if x >= maxInt64AsFloat || x < -maxInt64AsFloat || math.IsNaN(x) {
			return 0, false
		}
		return int64(x), true
	default:
		return 0, false
	}
}

// validateCampaignChildFlows ports the rest of CampaignConfig.Validate: at least
// one child flow, and each entry names a flow_id or a flow_name. (max_concurrent,
// max_depth and max_total_children are defaulted when <= 0 before the reference
// checks them, so their ">= 1" checks can never fire and are not ported.)
//
// Ahead of the JS port, which does not carry this rule yet. See PARITY.md.
func validateCampaignChildFlows(campaign doc, iss *issues) {
	field := keyCampaign + "." + keyChildFlows
	children, ok := getSlice(campaign, keyChildFlows)
	if !ok || len(children) == 0 {
		if raw := get(campaign, keyChildFlows); raw != nil && !ok {
			iss.error(Issue{Field: field, Code: codeInvalidType, Message: "campaign.child_flows must be a list"})
			return
		}
		iss.error(Issue{
			Field: field, Code: codeCampaignNoChildFlows,
			Message: "campaign requires at least one entry in child_flows",
		})
		return
	}
	for i, raw := range children {
		child, isMap := asRecord(raw)
		if !isMap {
			iss.error(Issue{
				Field: indexed(field, i), Code: codeInvalidType,
				Message: "each campaign.child_flows entry must be a mapping",
			})
			continue
		}
		if id, _ := scalarText(get(child, keyFlowID)); id != "" {
			continue
		}
		if name, _ := scalarText(get(child, keyFlowName)); name != "" {
			continue
		}
		iss.error(Issue{
			Field: indexed(field, i), Code: codeCampaignChildFlowNoID,
			Message:    fmt.Sprintf("campaign.child_flows[%d] needs a flow_id or a flow_name", i),
			Suggestion: "Name the child flow with flow_name (resolved in the caller's org) or pin it with flow_id",
		})
	}
}
