package aifvalidate

import (
	"fmt"
	"slices"
	"strings"
)

// The .exons documents a flow carries inline, judged where the reference's save
// door judges them. Everything the rules read about a document comes from the
// ExonsInspector (Options.Exons, or the built-in frontmatter reader).

func exonsInspectorOf(opts Options) ExonsInspector {
	if opts.Exons != nil {
		return opts.Exons
	}
	return frontmatterReader{}
}

// validateInlineExonsSteps is the reference's validateInlineExonsStep
// (v2.777.0, DC-FORGE-214), applied to every top-level step and loop sub-step:
// an inline `query.exons` document on an exons:// executor must have no tag the
// engine's own resolver refuses at execute (`{~exons.message~}` without role=,
// `{~exons.include~}` without template=, and anything that does not lex or
// parse). Before this rule the first parse was at run time.
//
// A document containing a Go template action is SKIPPED, as in the reference:
// the engine renders step params before the exons plugin parses them, so the
// text here is not the text the engine will judge.
//
// Only an engine-backed inspector can judge this; the built-in reader cannot.
func validateInlineExonsSteps(flow doc, iss *issues, inspector ExonsInspector) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		checkInlineExonsStep(step, stepField(stepID), stepID, iss, inspector)
		for i, sub := range loopSubStepsOf(stepID, step) {
			checkInlineExonsStep(sub.raw, sub.basePath, subStepID(stepID, sub.raw, i, iss), iss, inspector)
		}
	}
}

func checkInlineExonsStep(step doc, base, stepID string, iss *issues, inspector ExonsInspector) {
	executor, ok := iss.stringOf(step, keyExecutor, base)
	if !ok || !strings.HasPrefix(executor, spec.Exons.ExecutorPrefix) {
		return
	}
	query, ok := getRecord(step, keyQuery)
	if !ok {
		return
	}
	// The reference type-asserts `.(string)` on the query's `any` value, so
	// only a YAML string is a document.
	content, ok := asString(get(query, spec.Exons.DocumentParam))
	if !ok || content == "" || strings.Contains(content, spec.TemplateActionOpen) {
		return
	}
	report := inspector.InspectExons(content)
	if !report.Engine || len(report.IntakeErrors) == 0 {
		return
	}
	iss.error(Issue{
		Field: base + "." + keyQuery + "." + spec.Exons.DocumentParam, Code: codeExonsAttributes, StepID: stepID,
		Message: fmt.Sprintf("step '%s': exons document has tags that cannot render: %s",
			stepID, strings.Join(report.IntakeErrors, "; ")),
		Suggestion: "Fix each tag named: a built-in tag needs the attributes its resolver reads " +
			"(exons.message needs role=, exons.include needs template=)",
	})
}

// validateOrchestratorExons is the exons half of the reference's
// validateOrchestrator (parser.go) plus the orchestrator_tool_withheld warning
// (validateOrchestratorToolAllowWithholds, v2.760.0). The document's presence is
// judged by validateOrchestratorCampaign (orchestrator_exons_required).
//
// Refusals, in the reference's order:
//   - the document does not parse (engine-backed inspector; the built-in
//     reader only for an unclosed frontmatter or the retired config block);
//   - a tag its resolver refuses at execute (exons_attributes, engine only);
//   - no frontmatter spec, which the reference reports as a parse failure;
//   - no execution.provider;
//   - any requirements.resources entry (v2.767.0, DC-FORGE-205): AIgentFlow
//     can honour no declared resource today, so every one is refused rather
//     than run without the narrowing it asks for.
func validateOrchestratorExons(flow doc, iss *issues, inspector ExonsInspector) {
	orch, ok := getRecord(flow, keyOrchestrator)
	if !ok {
		return
	}
	source, ok := iss.stringOf(orch, keyExons, keyOrchestrator)
	if !ok || source == "" {
		return
	}
	field := keyOrchestrator + "." + keyExons
	report := inspector.InspectExons(source)

	if report.ParseError != "" {
		iss.error(Issue{
			Field: field, Code: codeOrchExonsParseFailed,
			Message: "orchestrator: failed to parse exons spec: " + report.ParseError,
		})
		return
	}
	if report.Engine && len(report.IntakeErrors) > 0 {
		iss.error(Issue{
			Field: field, Code: codeExonsAttributes,
			Message: "orchestrator: exons spec has tags that cannot render: " + strings.Join(report.IntakeErrors, "; "),
		})
	}
	if !report.SpecRead {
		return
	}
	if !report.HasSpec {
		iss.error(Issue{
			Field: field, Code: codeOrchExonsParseFailed,
			Message:    "orchestrator: failed to parse exons spec: the document has no frontmatter spec",
			Suggestion: "Start the document with a --- frontmatter block that sets execution.provider",
		})
		return
	}
	if report.Provider == "" {
		iss.error(Issue{
			Field: field, Code: codeOrchExonsNoProvider,
			Message: "orchestrator: exons spec must have execution.provider set",
		})
	}
	if len(report.Resources) > 0 {
		refs := make([]string, 0, len(report.Resources))
		for _, res := range report.Resources {
			refs = append(refs, fmt.Sprintf("%q (kind %s)", res.Ref, res.Kind))
		}
		iss.error(Issue{
			Field: field, Code: codeExonsResourcesRefused,
			Message: "the orchestrator's exons spec declares requirements.resources that AIgentFlow cannot honour, " +
				"so it is refused rather than run without the narrowing it asks for: " + strings.Join(refs, "; "),
			Suggestion: "Remove requirements.resources, and narrow the agent with tools.mcp_servers and tools.allow",
		})
	}
	warnOrchestratorToolWithheld(flow, orch, report, iss)
}

// warnOrchestratorToolWithheld: an orchestrator tool is offered only if it
// passes BOTH orchestrator.tools (empty = every tool) AND the definition's
// tools.allow (absent = no narrowing). Two lists that each read as complete can
// disagree, and the disagreement is silent at run time. Reported: a tool named
// in orchestrator.tools that tools.allow omits; with orchestrator.tools empty,
// a tools.allow without aif_ask_human (human intervention off), and one that
// names no campaign tool when the flow declares a campaign. The lifecycle tools,
// and the signal tools when signals are enabled, are exempt from tools.allow.
//
// A WARNING, never an error: an unattended orchestrator is a legitimate design.
func warnOrchestratorToolWithheld(flow, orch doc, report ExonsReport, iss *issues) {
	if !report.ToolsAllowDeclared {
		return
	}
	cfg := spec.OrchestratorToolAllow
	exempt := slices.Clone(cfg.LifecycleTools)
	signals := cfg.EnableSignalsDefault
	if v, ok := asBool(get(orch, keyEnableSignals)); ok {
		signals = v
	}
	if signals {
		exempt = append(exempt, cfg.SignalTools...)
	}
	withheld := func(name string) bool {
		return !slices.Contains(report.ToolsAllow, name) && !slices.Contains(exempt, name)
	}

	var named []string
	if tools, ok := getSlice(orch, keyTools); ok {
		for i, raw := range tools {
			if name, isStr := iss.stringAt(raw, indexed(keyOrchestrator+"."+keyTools, i)); isStr {
				named = append(named, name)
			}
		}
	}
	if len(named) > 0 {
		for _, name := range named {
			if withheld(name) {
				iss.warn(Issue{
					Field: cfg.WarningField, Code: codeOrchToolWithheld,
					Message: fmt.Sprintf("orchestrator tool %q is named in orchestrator.tools but the orchestrator's "+
						"exons definition's tools.allow does not name it, so it is never offered: a tool must pass "+
						"both lists. Add %q to tools.allow, or remove it from orchestrator.tools.", name, name),
				})
			}
		}
		return
	}
	if withheld(cfg.AskHumanTool) {
		iss.warn(Issue{
			Field: cfg.WarningField, Code: codeOrchToolWithheld,
			Message: fmt.Sprintf("the orchestrator's exons definition has a tools.allow list that does not name %s, "+
				"so this orchestrator can never ask a person a question: human intervention is off. Add %s to "+
				"tools.allow if a human should be reachable, or ignore this if the orchestrator is meant to run "+
				"unattended.", cfg.AskHumanTool, cfg.AskHumanTool),
		})
	}
	if _, hasCampaign := getRecord(flow, keyCampaign); hasCampaign &&
		!slices.ContainsFunc(cfg.CampaignTools, func(name string) bool { return !withheld(name) }) {
		iss.warn(Issue{
			Field: cfg.WarningField, Code: codeOrchToolWithheld,
			Message: fmt.Sprintf("this flow declares a campaign, but the orchestrator's exons definition has a "+
				"tools.allow list that names none of the campaign tools (%s), so no child mission can be spawned "+
				"or tracked. Add the campaign tools the orchestrator needs to tools.allow.", joinNames(cfg.CampaignTools)),
		})
	}
}
