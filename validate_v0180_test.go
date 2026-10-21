package aifvalidate

import (
	"slices"
	"strings"
	"testing"
)

// v0.18.0 — AIgentFlow v2.834.0 … v2.837.0: nexus:// canonical with atlas:// as
// its alias (v2.835.0), signals and DFM removed (v2.834.0), campaigns removed
// (v2.836.0), and the removal advice no longer naming share links (v2.837.0).
// The arms of v0.18.0 that looked inside the orchestrator block went with the
// block in v0.19.0 (validate_v0190_test.go). The conformance fixtures pin the
// verdicts; these tests pin fields, cardinality, messages and the
// no-false-positive edges.

// issueKeys renders findings as code@field#step, in order, so two spellings can
// be compared finding for finding.
func issueKeys(list []Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.Code+"@"+i.Field+"#"+i.StepID)
	}
	return out
}

// Every agent-runtime rule answers the same for nexus:// and atlas://. Equal is
// not enough (both spellings could be equally wrong), so each probe also pins
// the code it must produce.
func TestNexusAndAtlasAnswerIdentically(t *testing.T) {
	for _, tc := range []struct {
		name, path, query, want string
	}{
		{"clean agent run", "agent/run", "      agent: '@a/b'\n      prompt: hi\n", ""},
		{"clean capability", "capability/web_fetch", "      url: https://example.com\n", ""},
		{"agent missing", "agent/run", "      prompt: hi\n", "atlas_agent_missing"},
		{"agent not literal", "agent/run", "      agent: '{{ .query.a }}'\n      prompt: hi\n", "atlas_agent_not_literal"},
		{"prompt missing", "agent/run", "      agent: '@a/b'\n", "atlas_prompt_missing"},
		{"prompt both", "agent/run", "      agent: '@a/b'\n      prompt: hi\n      prompt_template: x\n", "atlas_prompt_both"},
		{"capabilities not literal", "agent/run", "      agent: '@a/b'\n      prompt: hi\n      capabilities: ['{{ .query.c }}']\n", "atlas_capabilities_not_literal"},
		{"model not literal", "agent/run", "      agent: '@a/b'\n      prompt: hi\n      model: '{{ .query.m }}'\n", "atlas_model_not_literal"},
		{"param refused", "agent/run", "      agent: '@a/b'\n      prompt: hi\n      user_id: x\n", "atlas_param_refused"},
		{"capability ref invalid", "capability/a.b", "      x: 1\n", "atlas_capability_ref_invalid"},
		{"operation unknown", "acme/reporter", "      x: 1\n", "atlas_operation_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nexus := ValidateFlow(atlasProbe("nexus://"+tc.path, tc.query), Options{})
			atlas := ValidateFlow(atlasProbe("atlas://"+tc.path, tc.query), Options{})
			if !slices.Equal(issueKeys(nexus.Errors), issueKeys(atlas.Errors)) ||
				!slices.Equal(issueKeys(nexus.Warnings), issueKeys(atlas.Warnings)) {
				t.Fatalf("spellings disagree:\nnexus: %s | %s\natlas: %s | %s", formatIssues(nexus.Errors),
					formatIssues(nexus.Warnings), formatIssues(atlas.Errors), formatIssues(atlas.Warnings))
			}
			assertCodesAbsent(t, "warning", nexus.Warnings, []string{codeUnknownExecScheme})
			if tc.want == "" {
				assertValid(t, nexus)
				return
			}
			assertError(t, nexus, tc.want, "steps.s")
		})
	}
}

func TestNexusIsAKnownSchemeAndAtlasItsAlias(t *testing.T) {
	if !executorSchemes.has("nexus") || !executorSchemes.has("atlas") {
		t.Fatal("both spellings must be known executor schemes")
	}
	if spec.AtlasExecutor.Scheme != "nexus" || !slices.Equal(spec.AtlasExecutor.Aliases, []string{"atlas"}) {
		t.Fatalf("canonical scheme %q, aliases %v", spec.AtlasExecutor.Scheme, spec.AtlasExecutor.Aliases)
	}
	// The scheme match is exact: another scheme that merely starts like it is
	// not judged by the agent-runtime rules.
	r := ValidateFlow(atlasProbe("nexusx://agent/run", "      prompt: hi\n"), Options{})
	assertCodesAbsent(t, "error", r.Errors, []string{"atlas_agent_missing"})
}

func TestNexusMessagesNameNexus(t *testing.T) {
	r := ValidateFlow(atlasProbe("atlas://capability/a.b", "      x: 1\n"), Options{})
	got, _ := findByCode(r.Errors, "atlas_capability_ref_invalid")
	if !strings.Contains(got.Message, "nexus://capability/a.b") {
		t.Errorf("message = %q", got.Message)
	}
	if strings.Contains(spec.RemovedAgentSurfaces.Suggestion, "a share link,") ||
		!strings.Contains(spec.RemovedAgentSurfaces.Suggestion, "share links no longer exist") ||
		strings.Contains(spec.RemovedAgentSurfaces.Suggestion, "atlas://") {
		t.Errorf("removal advice must say share links no longer exist and write nexus://: %q",
			spec.RemovedAgentSurfaces.Suggestion)
	}
}

func TestRemovedStepKeys(t *testing.T) {
	step := func(executor, query string) string {
		return atlasProbe(executor, query)
	}
	t.Run("enable_signals on aiv:// is refused on presence", func(t *testing.T) {
		for _, v := range []string{"~", "false", "true"} {
			r := ValidateFlow(step("aiv://acme/x", "      topic: t\n      enable_signals: "+v+"\n"), Options{})
			assertError(t, r, codeSignalRemoved, "steps.s")
		}
	})
	t.Run("enable_signals on ai:// is agentic_param_removed only", func(t *testing.T) {
		r := ValidateFlow(step("ai://openai/chat", "      prompt: hi\n      enable_signals: true\n"), Options{})
		assertError(t, r, codeAgenticParamRemoved, "steps.s")
		assertCodesAbsent(t, "error", r.Errors, []string{codeSignalRemoved})
	})
	t.Run("enable_signals on any other scheme is not a finding", func(t *testing.T) {
		for _, ex := range []string{"http://example/get", "nexus://agent/run", "flow://stored/c"} {
			r := ValidateFlow(step(ex, "      agent: '@a/b'\n      prompt: hi\n      enable_signals: true\n"), Options{})
			assertCodesAbsent(t, "error", r.Errors, []string{codeSignalRemoved})
		}
	})
	t.Run("flow_modification_allowed on any scheme but flow://", func(t *testing.T) {
		for _, v := range []string{"~", "false"} {
			r := ValidateFlow(step("http://example/get", "      url: https://x\n      flow_modification_allowed: "+v+"\n"), Options{})
			assertError(t, r, codeFlowModificationRemoved, "steps.s")
		}
		for _, ex := range []string{"flow://stored/c", "'http://{{ .query.h }}/get'"} {
			r := ValidateFlow(step(ex, "      flow_modification_allowed: true\n"), Options{})
			assertCodesAbsent(t, "error", r.Errors, []string{codeFlowModificationRemoved})
		}
	})
	t.Run("a removed scheme yields only executor_removed", func(t *testing.T) {
		r := ValidateFlow(step("exons://agent/execute", "      flow_modification_allowed: true\n"), Options{})
		assertError(t, r, codeExecutorRemoved, "steps.s")
		assertCodesAbsent(t, "error", r.Errors, []string{codeFlowModificationRemoved, codeSignalRemoved})
	})
	t.Run("a loop sub-step is addressed by its path and composite id", func(t *testing.T) {
		src := minimalFlow + "  spin:\n    loop:\n      while: '{{ lt .loop.index 2 }}'\n      max_iterations: 2\n      steps:\n" +
			"        - id: ask\n          executor: aiv://acme/x\n          query:\n            enable_signals: true\n            flow_modification_allowed: true\n"
		r := ValidateFlow(src, Options{})
		for _, code := range []string{codeSignalRemoved, codeFlowModificationRemoved} {
			i, ok := findByCode(r.Errors, code)
			if !ok || i.Field != "steps.spin.loop.steps[0]" || i.StepID != "spin.ask" {
				t.Errorf("%s: %+v (found %v)", code, i, ok)
			}
		}
	})
}

func TestRemovedKeyIgnoredWarnings(t *testing.T) {
	r := ValidateFlow(minimalFlow+"checkpoint:\n  pre_dfm: false\n", Options{})
	assertValid(t, r)
	if got := fieldsWithCode(r.Warnings, codeRemovedKeyIgnored); !slices.Equal(got, []string{"checkpoint.pre_dfm"}) {
		t.Fatalf("removed_key_ignored on %v", got)
	}
	assertCodesAbsent(t, "warning", r.Warnings, []string{codeUnknownYAMLKey})
	// A YAML null is absent.
	r = ValidateFlow(minimalFlow+"checkpoint:\n  pre_dfm: ~\n", Options{})
	assertCodesAbsent(t, "warning", r.Warnings, []string{codeRemovedKeyIgnored})
}

func TestCampaignRemoved(t *testing.T) {
	t.Run("the block, on presence", func(t *testing.T) {
		for _, body := range []string{"campaign: {}\n", "campaign: []\n", "campaign: 1\n", "campaign:\n  budget_max_per_child: 1\n"} {
			r := ValidateFlow(minimalFlow+body, Options{})
			if got := fieldsWithCode(r.Errors, codeCampaignRemoved); !slices.Equal(got, []string{"campaign"}) {
				t.Errorf("%q: campaign_removed on %v", body, got)
			}
			assertCodesAbsent(t, "error", r.Errors, []string{codeUnknownYAMLKey, codeInvalidType})
		}
		r := ValidateFlow(minimalFlow+"campaign: ~\n", Options{})
		assertCodesAbsent(t, "error", r.Errors, []string{codeCampaignRemoved})
	})
	t.Run("steps are never judged", func(t *testing.T) {
		r := ValidateFlow(atlasProbe("flow://stored/c", "      campaign: x\n      max_credits_per_child: 3\n"), Options{})
		assertCodesAbsent(t, "error", r.Errors, []string{codeCampaignRemoved})
	})
}
