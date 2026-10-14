package aifvalidate

import (
	"fmt"
	"slices"
	"strings"
)

// validateResidencyEnforcement ports the reference's catalogue-free warning
// compliance_residency_unenforced (AIgentFlow v2.821.0, #217).
//
// `compliance.residency` is a catalogue ELIGIBILITY check in the reference; it
// says nothing about the host a call dials. This rule is its honest counterpart
// and needs no catalogue, which is why a static port can reproduce it exactly:
//
//	if compliance.residency is non-empty and every code (trimmed, upper-cased)
//	is EU or an EU member state
//	  for each top-level step / loop sub-step whose executor starts "ai://", and
//	  the orchestrator:
//	    provider := the literal ai://<provider>/<op> segment
//	                (orchestrator: its exons frontmatter execution.provider)
//	    skip when provider is empty/templated or not a region-lane provider
//	    pinned := region is a non-empty string
//	           OR enforce_region is true (bool, or "true"/"1"/"yes" on a step)
//	           OR the call names its endpoint: query <provider>_base_url, or
//	              executor_config.<provider>.base_url, is non-empty
//	    if not pinned: warn
//
// A warning, never an error: the call may be correct (the key is EU-confined, a
// run input supplies the region) and a static check cannot see that. Not
// checked: exons:// and aiv:// steps (the provider lives in the agent), a
// quality_gate judge (it has no region key), a failover chain, a run-level
// `region` input. The region-lane provider set is a hand copy of go-vaibstract's
// RuntimeRegionProviders carried in the spec (PARITY.md).
func validateResidencyEnforcement(flow doc, iss *issues, inspector ExonsInspector) {
	cfg := spec.ResidencyEnforcement
	if !euOnlyResidency(flow, cfg.EUCodes) {
		return
	}
	residency := residencyDisplay(flow)

	warn := func(field, stepID, address, provider string) {
		iss.warn(Issue{
			Field: field, StepID: stepID, Code: codeComplianceResidencyUnenforced,
			Message:    fmt.Sprintf(cfg.MessageFormat, address, provider, residency),
			Suggestion: cfg.Suggestion,
		})
	}

	steps := stepsOf(flow)
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		if provider, query, ok := residencyCall(step); ok && residencyUnpinned(flow, provider, query, cfg) {
			warn(fmt.Sprintf(cfg.StepFieldFormat, stepID), stepID, fmt.Sprintf(cfg.SiteStepFormat, stepID), provider)
		}
		for _, sub := range loopSubStepsOf(stepID, step) {
			provider, query, ok := residencyCall(sub.raw)
			if !ok || !residencyUnpinned(flow, provider, query, cfg) {
				continue
			}
			subID, _ := getString(sub.raw, keyID)
			warn(fmt.Sprintf(cfg.LoopStepFieldFormat, stepID, subID), stepID,
				fmt.Sprintf(cfg.SiteLoopStepFormat, subID, stepID), provider)
		}
	}

	orch, ok := getRecord(flow, keyOrchestrator)
	if !ok {
		return
	}
	provider := orchestratorProvider(orch, inspector)
	if !residencyRegionLane(provider, cfg) {
		return
	}
	if region, isStr := getString(orch, keyRegion); isStr && strings.TrimSpace(region) != "" {
		return
	}
	if enforce, _ := asBool(get(orch, keyEnforceRegion)); enforce {
		return
	}
	if flowDirectsEndpoint(flow, provider, cfg) {
		return
	}
	warn(cfg.OrchestratorField, "", cfg.SiteOrchestrator, provider)
}

// euOnlyResidency reports whether compliance.residency is non-empty and every
// code (trimmed, upper-cased) is in the EU set. ["EU","US"] is NOT EU-only:
// residency means "at least one of the listed codes".
func euOnlyResidency(flow doc, euCodes []string) bool {
	compliance, ok := getRecord(flow, keyCompliance)
	if !ok {
		return false
	}
	codes, ok := getSlice(compliance, keyResidency)
	if !ok || len(codes) == 0 {
		return false
	}
	for _, raw := range codes {
		code, isStr := asString(raw)
		if !isStr || !slices.Contains(euCodes, strings.ToUpper(strings.TrimSpace(code))) {
			return false
		}
	}
	return true
}

// residencyDisplay returns the declared codes as written, for the message's
// `%v` (the reference prints a []string, "[EU DE]").
func residencyDisplay(flow doc) []string {
	compliance, _ := getRecord(flow, keyCompliance)
	codes, _ := getSlice(compliance, keyResidency)
	parts := make([]string, 0, len(codes))
	for _, raw := range codes {
		s, _ := asString(raw)
		parts = append(parts, s)
	}
	return parts
}

// residencyCall reads an `ai://<provider>/<op>` site. ok is false for any other
// executor. The provider is segment one and must be followed by a slash, as the
// reference's extractAIProviderAndModel reads it; a templated provider is "".
func residencyCall(site doc) (provider string, query doc, ok bool) {
	executor, isStr := getString(site, keyExecutor)
	if !isStr {
		return "", nil, false
	}
	rest, isAI := strings.CutPrefix(executor, spec.ResidencyEnforcement.AIProtocol+spec.ResidencyEnforcement.ProtocolSeparator)
	if !isAI {
		return "", nil, false
	}
	if slash := strings.Index(rest, "/"); slash > 0 {
		provider = rest[:slash]
	}
	query, _ = getRecord(site, keyQuery)
	return provider, query, true
}

// residencyRegionLane reports whether provider is a literal name of a provider
// whose adapter routes on `region`.
func residencyRegionLane(provider string, cfg residencyEnforcementSpec) bool {
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.Contains(provider, templateOpenDelim) {
		return false
	}
	return slices.Contains(cfg.RegionLaneProviders, strings.ToLower(provider))
}

// residencyUnpinned reports whether a region-lane call pins nothing.
func residencyUnpinned(flow doc, provider string, query doc, cfg residencyEnforcementSpec) bool {
	if !residencyRegionLane(provider, cfg) {
		return false
	}
	if region, ok := getString(query, cfg.RegionParam); ok && strings.TrimSpace(region) != "" {
		return false
	}
	switch v := get(query, cfg.EnforceRegionParam).(type) {
	case bool:
		if v {
			return false
		}
	case string:
		if slices.Contains(cfg.EnforceRegionTruthyStrings, v) {
			return false
		}
	}
	if base, ok := getString(query, strings.ToLower(provider)+cfg.BaseURLParamSuffix); ok && strings.TrimSpace(base) != "" {
		return false
	}
	return !flowDirectsEndpoint(flow, provider, cfg)
}

// flowDirectsEndpoint reports whether flow-level executor_config names a
// base_url for the provider (the author chose the host).
func flowDirectsEndpoint(flow doc, provider string, _ residencyEnforcementSpec) bool {
	inst, ok := executorConfigInstance(flow, strings.ToLower(provider))
	if !ok {
		return false
	}
	base, ok := getString(inst, "base_url")
	return ok && strings.TrimSpace(base) != ""
}

// orchestratorProvider reads the orchestrator's exons frontmatter
// `execution.provider`; "" when the document cannot be read statically.
func orchestratorProvider(orch doc, inspector ExonsInspector) string {
	source, ok := getString(orch, keyExons)
	if !ok || source == "" {
		return ""
	}
	report := inspector.InspectExons(source)
	if report.ParseError != "" || !report.SpecRead || !report.HasSpec {
		return ""
	}
	return report.Provider
}
