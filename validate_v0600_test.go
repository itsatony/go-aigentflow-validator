package aifvalidate

import (
	"slices"
	"strings"
	"testing"
)

// v0.6.0: the save-door rules AIgentFlow added after v2.753.0. The conformance
// fixtures pin each verdict against the reference; these tests pin the parts a
// verdict cannot show — fields, counts, the inspector contract.

const orchDoc = "---\nname: orch\ndescription: coordinates\ntype: agent\nexecution: {provider: anthropic, model: m}\n%s---\nhi\n"

func orchFlow(frontmatterExtra, orchExtra, flowExtra string) string {
	doc := strings.Replace(orchDoc, "%s", frontmatterExtra, 1)
	return minimalFlow + flowExtra + "orchestrator:\n  agentic: true\n" + orchExtra +
		"  exons: |\n" + indentLines(doc, "    ")
}

func indentLines(s, prefix string) string {
	lines := strings.SplitAfter(s, "\n")
	var b strings.Builder
	for _, l := range lines {
		if l == "" {
			continue
		}
		b.WriteString(prefix + l)
	}
	return b.String()
}

func TestExecutorConfigEnvScopes(t *testing.T) {
	flow := func(block string) string { return minimalFlow + "executor_config:\n" + block }
	t.Run("every violation is reported on its own field", func(t *testing.T) {
		result := ValidateFlow(flow(`  openai:
    api_key: '${AIGENTFLOW_ANTHROPIC_API_KEY}'
    base_url: '${AIGENTFLOW_OPENAI_BASE_URL}'
    organization: '${X}'
  db:
    extra:
      a: '${AIGENTFLOW_OPENAI_API_KEY}'
      b: '${AIGENTFLOW_DB_REDIS_CONNECTION_STRING}'
`), Options{})
		got := fieldsWithCode(result.Errors, codeExecutorConfigEnvScope)
		want := []string{"executor_config.db.extra.a", "executor_config.openai.api_key", "executor_config.openai.organization"}
		if !slices.Equal(got, want) {
			t.Errorf("fields %v, want %v", got, want)
		}
	})
	t.Run("the message names the permitted set, and an empty one", func(t *testing.T) {
		result := ValidateFlow(flow("  aiv:\n    api_key: '${AIGENTFLOW_OPENAI_API_KEY}'\n  openai:\n    api_key: '${X}'\n"), Options{})
		aiv, _ := findByField(result.Errors, "executor_config.aiv.api_key")
		if !strings.Contains(aiv.Message, "(none") {
			t.Errorf("empty scope message: %q", aiv.Message)
		}
		openai, _ := findByField(result.Errors, "executor_config.openai.api_key")
		if !strings.Contains(openai.Message, "AIGENTFLOW_OPENAI_API_KEY, AIGENTFLOW_OPENAI_BASE_URL") {
			t.Errorf("permitted set missing: %q", openai.Message)
		}
	})
	t.Run("the scope table is the reference's, not a prefix rule", func(t *testing.T) {
		// A computed pair exists only for AI provider keys: a driver key such
		// as `device` gets its protocol's variable and nothing composed.
		assertError(t, ValidateFlow(flow("  device:\n    api_key: '${AIGENTFLOW_DEVICE_API_KEY}'\n"), Options{}),
			codeExecutorConfigEnvScope, "executor_config.device.api_key")
		assertValid(t, ValidateFlow(flow("  device:\n    api_key: '${AIGENTFLOW_SHELLY_AUTH_KEY}'\n"), Options{}))
		if n := len(spec.ExecutorConfigEnvScopes.Scopes); n < 30 {
			t.Fatalf("only %d scoped keys vendored: the table did not load", n)
		}
	})
	t.Run("a reference is exactly ${NAME}", func(t *testing.T) {
		for value, want := range map[string]bool{
			"${A}": true, "${A}${B}": true, "${}": false, "${A": false, "x${A}": false, "": false, "$": false,
		} {
			if _, got := executorConfigEnvReference(value); got != want {
				t.Errorf("%q: reference=%v, want %v", value, got, want)
			}
		}
	})
}

// fakeInspector returns a fixed report, so the rules can be tested apart from
// any engine.
type fakeInspector struct{ report ExonsReport }

func (f fakeInspector) InspectExons(string) ExonsReport { return f.report }

func TestInlineExonsSteps(t *testing.T) {
	src := minimalFlow + `  loopy:
    loop:
      while: '{{ lt .loop.index 2 }}'
      max_iterations: 2
      steps:
        - id: inner
          executor: exons://agent/execute
          query:
            exons: 'doc'
`
	src = strings.Replace(src, "    executor: function://text/noop\n",
		"    executor: exons://agent/execute\n    query:\n      exons: 'doc'\n    next:\n      default: loopy\n", 1)
	engine := Options{Exons: fakeInspector{ExonsReport{Engine: true, IntakeErrors: []string{"line 1, column 1: bad"}}}}
	result := ValidateFlow(src, engine)
	got := fieldsWithCode(result.Errors, codeExonsAttributes)
	want := []string{"steps.loopy.loop.steps[0].query.exons", "steps.only.query.exons"}
	if !slices.Equal(got, want) {
		t.Fatalf("fields %v, want %v", got, want)
	}
	if issue, _ := findByField(result.Errors, "steps.loopy.loop.steps[0].query.exons"); issue.StepID != "loopy.inner" {
		t.Errorf("loop sub-step id %q, want loopy.inner", issue.StepID)
	}
	// The same report from something that is not an engine proves nothing.
	notEngine := Options{Exons: fakeInspector{ExonsReport{IntakeErrors: []string{"x"}}}}
	if got := fieldsWithCode(ValidateFlow(src, notEngine).Errors, codeExonsAttributes); len(got) != 0 {
		t.Errorf("a non-engine report refused %v", got)
	}
}

func TestOrchestratorExons(t *testing.T) {
	t.Run("a parse error is the only refusal reported", func(t *testing.T) {
		opts := Options{Exons: fakeInspector{ExonsReport{Engine: true, ParseError: "boom", SpecRead: true, HasSpec: true,
			Resources: []ExonsResource{{Ref: "r", Kind: "corpus"}}}}}
		result := ValidateFlow(orchFlow("", "", ""), opts)
		if got := codesOf(result.Errors); !slices.Equal(got, []string{codeOrchExonsParseFailed}) {
			t.Errorf("errors %v", got)
		}
	})
	t.Run("the built-in reader reads provider and resources", func(t *testing.T) {
		assertValid(t, ValidateFlow(orchFlow("", "", ""), Options{}))
		result := ValidateFlow(orchFlow("requirements:\n  resources:\n    - {ref: a, kind: toolset}\n", "", ""), Options{})
		assertError(t, result, codeExonsResourcesRefused, "orchestrator.exons")
		noProvider := strings.Replace(orchFlow("", "", ""), "execution: {provider: anthropic, model: m}", "execution: {model: m}", 1)
		assertError(t, ValidateFlow(noProvider, Options{}), codeOrchExonsNoProvider, "orchestrator.exons")
	})
	t.Run("a templated frontmatter is not read", func(t *testing.T) {
		// The engine EXECUTES a frontmatter with a tag in it before decoding
		// it; the built-in reader must not guess what that decodes to.
		src := strings.Replace(orchFlow("", "", ""), "provider: anthropic", `provider: '{~exons.var name="p" /~}'`, 1)
		src = strings.Replace(src, "model: m}", "model: m}\n    requirements: {resources: [{ref: a, kind: toolset}]}", 1)
		result := ValidateFlow(src, Options{})
		assertCodesAbsent(t, "error", result.Errors, []string{codeExonsResourcesRefused, codeOrchExonsNoProvider})
	})
	t.Run("withheld tools: one warning per named tool, on orchestrator.exons", func(t *testing.T) {
		result := ValidateFlow(orchFlow("tools:\n  allow: [aif_ask_human]\n",
			"  tools: [aif_step_start, aif_step_cancel, aif_mission_complete, aif_signal_complete, aif_ask_human]\n", ""), Options{})
		got := fieldsWithCode(result.Warnings, codeOrchToolWithheld)
		if !slices.Equal(got, []string{"orchestrator.exons", "orchestrator.exons"}) {
			t.Errorf("warnings on %v, want two on orchestrator.exons", got)
		}
	})
	t.Run("an empty allow with orchestrator.tools empty warns about aif_ask_human only", func(t *testing.T) {
		result := ValidateFlow(orchFlow("tools:\n  allow: []\n", "", ""), Options{})
		if got := fieldsWithCode(result.Warnings, codeOrchToolWithheld); len(got) != 1 {
			t.Errorf("got %d warnings, want 1: %s", len(got), formatIssues(result.Warnings))
		}
	})
	t.Run("a null campaign is no campaign", func(t *testing.T) {
		result := ValidateFlow(orchFlow("tools:\n  allow: [aif_ask_human]\n", "", "campaign:\n"), Options{})
		assertCodesAbsent(t, "warning", result.Warnings, []string{codeOrchToolWithheld})
	})
}

func TestExtractExonsFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name, src, fm string
		has           bool
		parseErr      bool
	}{
		{name: "plain", src: "---\na: 1\n---\nbody", fm: "a: 1", has: true},
		{name: "crlf", src: "---\r\na: 1\r\n---\r\nbody", fm: "a: 1", has: true},
		{name: "bom and indent", src: "\xef\xbb\xbf  \t---\na: 1\n---\n", fm: "a: 1", has: true},
		{name: "closing at end of input", src: "---\na: 1\n---", fm: "a: 1", has: true},
		{name: "empty", src: "---\n---\n", fm: "", has: true},
		{name: "no frontmatter", src: "hello\n---\na: 1\n---\n"},
		{name: "a newline before the delimiter", src: "\n---\na: 1\n---\n"},
		{name: "delimiter alone", src: "---"},
		{name: "delimiter with text", src: "---x\na: 1\n---\n"},
		{name: "closing needs a line of its own", src: "---\na: 1\n---x\n---\n", fm: "a: 1\n---x", has: true},
		{name: "unclosed", src: "---\na: 1\n", has: true, parseErr: true},
		{name: "legacy config block", src: "{~exons.config~}{}{~/exons.config~}", parseErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fm, has, parseErr := extractExonsFrontmatter(tc.src)
			if fm != tc.fm || has != tc.has || (parseErr != "") != tc.parseErr {
				t.Errorf("got (%q, %v, %q), want (%q, %v, parseErr=%v)", fm, has, parseErr, tc.fm, tc.has, tc.parseErr)
			}
		})
	}
}
