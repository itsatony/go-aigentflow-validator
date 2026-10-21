package aifvalidate

import (
	"slices"
	"strings"
	"testing"
)

// v0.19.0 — AIgentFlow v2.838.0: the orchestrator is removed. The block is
// refused WHOLE as orchestrator_removed and nothing inside it is judged; the
// flow-level tool_discovery / discoverable_tools and `next: orchestrator` are
// orchestrator_removed too. The conformance fixtures pin the verdicts; these
// tests pin fields, step ids, cardinality, presence and the no-false-positive
// edges.

// TestOrchestratorRemovedOnPresence: any non-null value is ONE finding at
// `orchestrator`, never a type or unknown-key finding that would hide the
// advice; a YAML null is absent.
func TestOrchestratorRemovedOnPresence(t *testing.T) {
	for _, body := range []string{
		"orchestrator: {}\n", "orchestrator: []\n", "orchestrator: 1\n", "orchestrator: true\n",
		"orchestrator: monitor\n", "orchestrator:\n  not_a_real_key: 1\n  mode: monitor\n",
	} {
		r := ValidateFlow(minimalFlow+body, Options{})
		if got := fieldsWithCode(r.Errors, codeOrchestratorRemoved); !slices.Equal(got, []string{"orchestrator"}) {
			t.Errorf("%q: orchestrator_removed on %v: %s", body, got, formatIssues(r.Errors))
		}
		if len(r.Errors) != 1 {
			t.Errorf("%q: want exactly one error, got %s", body, formatIssues(r.Errors))
		}
		assertCodesAbsent(t, "warning", r.Warnings, []string{codeUnknownYAMLKey})
		i, _ := findByCode(r.Errors, codeOrchestratorRemoved)
		if i.Suggestion != spec.RemovedOrchestratorSurfaces.Suggestion || i.StepID != "" {
			t.Errorf("%q: suggestion/step id: %+v", body, i)
		}
	}
	for _, body := range []string{"orchestrator:\n", "orchestrator: ~\n", "orchestrator: null\n"} {
		assertValid(t, ValidateFlow(minimalFlow+body, Options{}))
	}
}

// TestOrchestratorRemovedJudgesNothingInside: everything the retired rules (and
// the v0.18.0 signal / campaign arms) used to judge inside the block is silent,
// with StrictRegistries on as well.
func TestOrchestratorRemovedJudgesNothingInside(t *testing.T) {
	inner := "  mode: owner\n  enforce_region: true\n  human_question_timeout: -5m\n" +
		"  enable_signals: true\n  tool_discovery: of\n" +
		"  triggers:\n    - type: timer\n    - type: eclipse\n    - type: dfm_suggestion\n    - type: campaign_child_failed\n" +
		"  tools: [aif_ask_human, aif_signal_complete, aif_campaign_spawn, not_a_tool]\n" +
		"  deny_tools: [aif_signal_delegate]\n  require_approval_for: [aif_campaign_cancel]\n"
	withoutExons := minimalFlow + "orchestrator:\n  agentic: false\n" + inner
	withExons := orchFlow("tools:\n  allow: [aif_signal_need_tool]\nrequirements:\n  resources:\n    - {ref: a, kind: toolset}\n", inner, "")
	for name, src := range map[string]string{"no exons": withoutExons, "exons that the old rules refused": withExons} {
		for _, strict := range []bool{false, true} {
			r := ValidateFlow(src, Options{StrictRegistries: strict})
			if got := issueKeys(r.Errors); !slices.Equal(got, []string{codeOrchestratorRemoved + "@orchestrator#"}) {
				t.Errorf("%s (strict=%v): errors %v", name, strict, got)
			}
			if len(r.Warnings) != 0 {
				t.Errorf("%s (strict=%v): warnings %s", name, strict, formatIssues(r.Warnings))
			}
		}
	}
}

func TestOrchestratorRemovedDiscoveryKeys(t *testing.T) {
	for _, key := range []string{"tool_discovery", "discoverable_tools"} {
		for _, value := range []string{"''", "lazy", "of", "5", "[]", "[search]", "{}"} {
			r := ValidateFlow(minimalFlow+key+": "+value+"\n", Options{})
			if got := issueKeys(r.Errors); !slices.Equal(got, []string{codeOrchestratorRemoved + "@" + key + "#"}) {
				t.Errorf("%s: %s: errors %v", key, value, got)
			}
			i, _ := findByCode(r.Errors, codeOrchestratorRemoved)
			if !strings.Contains(i.Message, `"`+key+`"`) {
				t.Errorf("%s: message names the key: %q", key, i.Message)
			}
		}
		assertValid(t, ValidateFlow(minimalFlow+key+": ~\n", Options{}))
	}
	// One finding each, in the reference's order: block, tool_discovery,
	// discoverable_tools, then the steps.
	r := ValidateFlow(withStep("    executor: function://utils/log\n    next:\n      default: orchestrator\n")+
		"discoverable_tools: []\ntool_discovery: ''\norchestrator: {}\n", Options{})
	want := []string{
		codeOrchestratorRemoved + "@orchestrator#", codeOrchestratorRemoved + "@tool_discovery#",
		codeOrchestratorRemoved + "@discoverable_tools#", codeOrchestratorRemoved + "@steps.only#only",
	}
	if got := issueKeys(r.Errors); !slices.Equal(got, want) {
		t.Errorf("order: %v, want %v", got, want)
	}
}

func TestOrchestratorRemovedNext(t *testing.T) {
	step := func(next string) string {
		return withStep("    executor: function://utils/log\n    next:\n" + next)
	}
	for name, next := range map[string]string{
		"default":                  "      default: orchestrator\n",
		"condition":                "      conditions:\n        - if: '{{ true }}'\n          goto: orchestrator\n      default: 'null'\n",
		"default and two goto":     "      conditions:\n        - if: '{{ true }}'\n          goto: orchestrator\n        - if: '{{ false }}'\n          goto: orchestrator\n      default: orchestrator\n",
		"no block, so not unknown": "      default: orchestrator\n",
	} {
		r := ValidateFlow(step(next), Options{})
		if got := issueKeys(r.Errors); !slices.Equal(got, []string{codeOrchestratorRemoved + "@steps.only#only"}) {
			t.Errorf("%s: errors %v", name, got)
		}
	}
	// The match is exact and case-sensitive: `Orchestrator` is a step reference.
	r := ValidateFlow(step("      default: Orchestrator\n"), Options{})
	assertError(t, r, codeStepNotFound, "")
	assertCodesAbsent(t, "error", r.Errors, []string{codeOrchestratorRemoved})
	// A loop sub-step's next keeps its own refusal, and is not judged here.
	loop := minimalFlow + "  spin:\n    loop:\n      while: '{{ lt .loop.index 2 }}'\n      max_iterations: 2\n      steps:\n" +
		"        - id: a\n          executor: function://utils/log\n          next:\n            default: orchestrator\n"
	r = ValidateFlow(loop, Options{})
	if got := codesOf(r.Errors); !slices.Equal(got, []string{codeLoopSubstepNextSentinel}) {
		t.Errorf("loop sub-step next orchestrator: %v", got)
	}
}

// The step query is never judged by this code: a flow:// child input named
// orchestrator is the child's; tool_discovery on ai:// is agentic_param_removed.
func TestOrchestratorRemovedNeverJudgesAStepQuery(t *testing.T) {
	r := ValidateFlow(atlasProbe("flow://stored/c", "      orchestrator: x\n      tool_discovery: of\n      discoverable_tools: [a]\n"), Options{})
	assertCodesAbsent(t, "error", r.Errors, []string{codeOrchestratorRemoved})
	r = ValidateFlow(atlasProbe("mock://api/x", "      tool_discovery: of\n"), Options{})
	assertValid(t, r)
	r = ValidateFlow(atlasProbe("ai://openai/chat", "      prompt: hi\n      tool_discovery: lazy\n"), Options{})
	if got := codesOf(r.Errors); !slices.Equal(got, []string{codeAgenticParamRemoved}) {
		t.Errorf("ai:// tool_discovery: %v", got)
	}
}

// A flow with both removed blocks gets one finding for each.
func TestCampaignAndOrchestratorBlocksAreOneFindingEach(t *testing.T) {
	r := ValidateFlow(minimalFlow+"campaign:\n  child_flows: []\norchestrator:\n  mode: monitor\n", Options{})
	if got := issueKeys(r.Errors); !slices.Equal(got, []string{codeCampaignRemoved + "@campaign#", codeOrchestratorRemoved + "@orchestrator#"}) {
		t.Errorf("errors %v", got)
	}
}

// recordingInspector fails the test if a rule consults it.
type recordingInspector struct{ calls *int }

func (r recordingInspector) InspectExons(string) ExonsReport {
	*r.calls++
	return ExonsReport{}
}

// No rule reads Options.Exons any more; passing one changes no verdict.
func TestOptionsExonsIsNeverConsulted(t *testing.T) {
	calls := 0
	src := orchFlow("tools:\n  allow: []\n", "  tools: [aif_step_start]\n",
		"compliance:\n  residency: [EU]\n")
	with := ValidateFlow(src, Options{Exons: recordingInspector{&calls}})
	without := ValidateFlow(src, Options{})
	if calls != 0 {
		t.Errorf("the inspector was consulted %d times", calls)
	}
	if !slices.Equal(issueKeys(with.Errors), issueKeys(without.Errors)) ||
		!slices.Equal(issueKeys(with.Warnings), issueKeys(without.Warnings)) {
		t.Errorf("verdicts differ: %v | %v", issueKeys(with.Errors), issueKeys(without.Errors))
	}
}

// exons_uploads is a typed block in the reference: a YAML null is absent.
func TestExonsUploadsNullIsAbsent(t *testing.T) {
	assertValid(t, ValidateFlow(minimalFlow+"exons_uploads: ~\n", Options{}))
	assertError(t, ValidateFlow(minimalFlow+"exons_uploads: {}\n", Options{}), codeExecutorRemoved, "exons_uploads")
}

func TestTheOrchestratorLeftTheSpec(t *testing.T) {
	for _, name := range []string{"OrchestratorDefinition", "OrchestratorTriggerDef"} {
		if _, ok := spec.KnownKeys.Types[name]; ok {
			t.Errorf("knownKeys still types %s", name)
		}
	}
	root := spec.KnownKeys.Types[spec.KnownKeys.Root]
	for _, key := range []string{"orchestrator", "tool_discovery", "discoverable_tools", "campaign"} {
		if root[key] != "any" {
			t.Errorf("root key %s is %q, want any (decoded untyped, never an unknown key)", key, root[key])
		}
	}
	if !nextMarkers.has("orchestrator") {
		t.Error("orchestrator stays a next: marker, so it is never an unknown step")
	}
}

// The advice is the reference's current text for each removal.
func TestRemovalAdviceIsCurrent(t *testing.T) {
	orch := spec.RemovedOrchestratorSurfaces.Suggestion
	if !strings.Contains(orch, "removed in 2.838.0") || !strings.Contains(orch, "nexus://agent/run") ||
		strings.Contains(orch, "atlas://") {
		t.Errorf("orchestrator advice: %q", orch)
	}
	sig := spec.RemovedSignalSurfaces.Suggestion
	if strings.Contains(sig, "aif_signal_") || strings.Contains(sig, "dfm_suggestion") ||
		!strings.HasPrefix(sig, "Signals and dynamic flow modification were removed in 2.834.0. enable_signals: delete it;") {
		t.Errorf("signal advice must name only the step keys it still refuses: %q", sig)
	}
	if camp := spec.RemovedCampaignSurfaces.Suggestion; !strings.HasSuffix(camp, "Delete the campaign: block") {
		t.Errorf("campaign advice: %q", camp)
	}
	if strings.Contains(spec.ResidencyEnforcement.Suggestion, "orchestrator") {
		t.Errorf("residency advice still names the orchestrator: %q", spec.ResidencyEnforcement.Suggestion)
	}
}
