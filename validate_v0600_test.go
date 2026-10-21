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
