package aifvalidate

import (
	"fmt"
	"strings"
	"time"
)

// validateOrchestratorCampaign checks the orchestrator block. (The campaign block
// was removed in AIgentFlow 2.836.0: it is refused whole as campaign_removed by
// validateRemovedOrchestratorSurfaces, and none of its old rules run.)
//
// Mirrors the portable parts of validateOrchestrator (parser.go). The `exons` spec body is parsed by
// the go-exons engine in the reference and is NOT re-implemented here — its
// PRESENCE is required, its contents are out of scope for a static validator.
// See PARITY.md.
func validateOrchestratorCampaign(flow doc, iss *issues, opts Options) {
	orch, hasOrchestrator := getRecord(flow, keyOrchestrator)

	if hasOrchestrator {
		if !iss.nonEmptyStringOf(orch, keyExons, keyOrchestrator) {
			iss.error(Issue{
				Field: keyOrchestrator + "." + keyExons, Code: codeOrchExonsRequired,
				Message: "orchestrator requires an exons specification",
			})
		}

		validateHumanQuestionTimeout(orch, iss)
		validateEnforceRegion(orch, iss)

		// DC-COND-1 termination authority. An absent/empty mode defaults to
		// monitor, which is valid.
		mode, hasMode := iss.stringOf(orch, keyMode, keyOrchestrator)
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
				tool, isStr := iss.stringAt(raw, indexed(keyOrchestrator+"."+keyTools, i))
				// A removed signal / campaign name is refused by its own code
				// (validateRemovedOrchestratorSurfaces), never as unknown.
				if !isStr || orchestratorTools.has(tool) || removedOrchestratorTools.has(tool) {
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

}

func validateOrchestratorTrigger(trigger doc, base string, iss *issues) {
	ttype, isStr := iss.stringOf(trigger, keyType, base)
	// A removed trigger type (dfm_suggestion, campaign_child_*) is refused by
	// its own code, never as unknown.
	if isStr && removedOrchestratorTriggers.has(ttype) {
		return
	}
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
	// A Go `string` field, filled from any scalar by its source text: `interval:
	// 0` is "0" and saves, `interval: 100` is "100" (no unit). A number used to
	// read as "no interval" here.
	interval, hasInterval := iss.stringOf(trigger, keyInterval, base)
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
	if s, ok := iss.stringAt(declared, keyOrchestrator+"."+keyHumanQuestionTimeout); ok {
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

// validateEnforceRegion ports the reference's orchestrator region rule (AIF
// v2.820.0): `enforce_region: true` makes the provider refuse a call that
// carries no region, so declaring it without naming `region` would fail every
// orchestrator turn closed. Whitespace-only counts as unnamed (the reference
// trims). `enforce_region: false` or absent needs nothing, and `region` alone
// is a pin, not an error. The reference runs this in its parser, so it is an
// error at the save door.
func validateEnforceRegion(orch doc, iss *issues) {
	enforce, ok := asBool(get(orch, keyEnforceRegion))
	if !ok || !enforce {
		return
	}
	region, _ := iss.stringOf(orch, keyRegion, keyOrchestrator)
	if strings.TrimSpace(region) != "" {
		return
	}
	iss.error(Issue{
		Field: keyOrchestrator + "." + keyEnforceRegion, Code: codeOrchEnforceRegionNeeds,
		Message:    "orchestrator: enforce_region requires region (name the region the model must run in, e.g. region: eu)",
		Suggestion: "Add orchestrator.region (for example region: eu), or remove enforce_region",
	})
}
