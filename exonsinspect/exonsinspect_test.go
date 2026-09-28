package exonsinspect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	aifvalidate "github.com/itsatony/go-aigentflow-validator"
)

// The conformance fixtures whose verdict depends on the .exons documents they
// carry, judged with the engine. Each case states the REFERENCE's verdict
// (measured on AIgentFlow v2.788.0's save door): three of them are verdicts the
// built-in reader cannot reach, and the root suite asserts its looser answer.
var engineCases = []struct {
	file        string
	valid       bool
	wantErrors  []string
	wantWarns   []string
	forbidWarns []string
}{
	{file: "invalid-exons-step-attributes.yaml", wantErrors: []string{"exons_attributes"}},
	{file: "valid-exons-step-attributes-skipped.yaml", valid: true},
	{file: "invalid-orchestrator-exons-no-spec.yaml", wantErrors: []string{"orchestrator_exons_parse_failed"}},
	{file: "invalid-orchestrator-exons-unclosed-frontmatter.yaml", wantErrors: []string{"orchestrator_exons_parse_failed"}},
	{file: "invalid-orchestrator-exons-no-provider.yaml", wantErrors: []string{"orchestrator_exons_no_provider"}},
	{file: "invalid-orchestrator-exons-resources.yaml", wantErrors: []string{"exons_resources_unhonoured"}},
	{file: "valid-orchestrator-exons-resources-empty.yaml", valid: true},
	{file: "invalid-orchestrator-exons-attributes.yaml", wantErrors: []string{"exons_attributes"}},
	{file: "invalid-orchestrator-exons-spec-invalid.yaml", wantErrors: []string{"orchestrator_exons_parse_failed"}},
	{file: "warn-orchestrator-tool-withheld-ask-human.yaml", valid: true, wantWarns: []string{"orchestrator_tool_withheld"}},
	{file: "warn-orchestrator-tool-withheld-named.yaml", valid: true, wantWarns: []string{"orchestrator_tool_withheld"}},
	{file: "warn-orchestrator-tool-withheld-signals-off.yaml", valid: true, wantWarns: []string{"orchestrator_tool_withheld"}},
	{file: "warn-orchestrator-tool-withheld-campaign.yaml", valid: true, wantWarns: []string{"orchestrator_tool_withheld"}},
	{file: "valid-orchestrator-tool-allow-consistent.yaml", valid: true, forbidWarns: []string{"orchestrator_tool_withheld"}},
	{file: "valid-orchestrator-tool-allow-campaign.yaml", valid: true, forbidWarns: []string{"orchestrator_tool_withheld"}},
	{file: "valid-orchestrator-tool-allow-absent.yaml", valid: true, forbidWarns: []string{"orchestrator_tool_withheld"}},
	// Every orchestrator fixture the JS port shares must stay valid with the
	// engine too: a false refusal here is the failure that matters most.
	{file: "valid-orchestrator-monitor.yaml", valid: true},
	{file: "valid-campaign-handoff.yaml", valid: true},
	{file: "valid-orchestrator-human-question-timeout.yaml", valid: true},
	{file: "valid-tool-discovery-vocabulary.yaml", valid: true},
}

func codes(list []aifvalidate.Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.Code)
	}
	return out
}

func TestEngineConformance(t *testing.T) {
	for _, tc := range engineCases {
		t.Run(tc.file, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", "testdata", "conformance", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			got := aifvalidate.ValidateFlow(string(src), aifvalidate.Options{Exons: New()})
			if got.Valid != tc.valid {
				t.Fatalf("valid = %v, want %v; errors %v", got.Valid, tc.valid, codes(got.Errors))
			}
			for _, c := range tc.wantErrors {
				if !slices.Contains(codes(got.Errors), c) {
					t.Errorf("missing error %q; got %v", c, codes(got.Errors))
				}
			}
			for _, c := range tc.wantWarns {
				if !slices.Contains(codes(got.Warnings), c) {
					t.Errorf("missing warning %q; got %v", c, codes(got.Warnings))
				}
			}
			for _, c := range tc.forbidWarns {
				if slices.Contains(codes(got.Warnings), c) {
					t.Errorf("forbidden warning %q present", c)
				}
			}
		})
	}
}

const goodDoc = `---
name: helper
description: helps
type: agent
execution:
  provider: anthropic
  model: claude-sonnet-4-6
tools:
  allow: []
requirements:
  resources:
    - ref: docs
      kind: toolset
      scope: user
---
{~exons.message role="system"~}help{~/exons.message~}
`

func TestInspectReportsTheSpec(t *testing.T) {
	r := New().InspectExons(goodDoc)
	if !r.Engine || r.ParseError != "" || len(r.IntakeErrors) != 0 {
		t.Fatalf("a well-formed document: %+v", r)
	}
	if !r.SpecRead || !r.HasSpec || r.Provider != "anthropic" {
		t.Errorf("spec fields: %+v", r)
	}
	if !r.ToolsAllowDeclared || r.ToolsAllow == nil || len(r.ToolsAllow) != 0 {
		t.Errorf("allow: [] must be declared and empty, got declared=%v %#v", r.ToolsAllowDeclared, r.ToolsAllow)
	}
	want := []aifvalidate.ExonsResource{{Ref: "docs", Kind: "toolset", Scope: "user"}}
	if !slices.Equal(r.Resources, want) {
		t.Errorf("resources = %+v, want %+v", r.Resources, want)
	}
}

func TestInspectJudgesParseAndIntake(t *testing.T) {
	t.Run("an absent allow is not declared", func(t *testing.T) {
		r := New().InspectExons("---\nname: a\ndescription: b\n---\nhi\n")
		if r.ToolsAllowDeclared || r.ToolsAllow != nil {
			t.Errorf("%+v", r)
		}
	})
	t.Run("a tag its resolver refuses is an intake error, not a parse error", func(t *testing.T) {
		r := New().InspectExons("{~exons.message~}hi{~/exons.message~}")
		if r.ParseError != "" || len(r.IntakeErrors) == 0 {
			t.Errorf("%+v", r)
		}
	})
	t.Run("an invalid spec is a parse error", func(t *testing.T) {
		r := New().InspectExons("---\nname: a\nexecution:\n  provider: x\n---\nhi\n")
		if r.ParseError == "" || r.SpecRead {
			t.Errorf("%+v", r)
		}
	})
	t.Run("no frontmatter parses to no spec", func(t *testing.T) {
		r := New().InspectExons("just text")
		if r.ParseError != "" || !r.SpecRead || r.HasSpec {
			t.Errorf("%+v", r)
		}
	})
	t.Run("an env tag in the frontmatter fails as in the reference", func(t *testing.T) {
		// go-exons has refused env access by default since v0.35.0, so this
		// cannot tell New() from New(WithEnvDisabled()); the option is kept
		// because it is the reference's constructor, and this pins the outcome.
		r := New().InspectExons("---\nname: a\ndescription: '{~exons.env name=\"HOME\" /~}'\n---\nhi\n")
		if !strings.Contains(r.ParseError, "environment variable access is disabled") {
			t.Errorf("environment access must be refused: %+v", r)
		}
	})
}

// The documented contract: safe for concurrent use. Run under -race.
func TestInspectIsSafeForConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	inspector := New()
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := inspector.InspectExons(goodDoc); r.Provider != "anthropic" {
				t.Errorf("provider = %q", r.Provider)
			}
		}()
	}
	wg.Wait()
}

// The built-in reader follows the engine's frontmatter extraction. Every shape
// here must get the SAME orchestrator verdict and error codes from both, which
// is what makes the built-in reader's refusals safe to trust.
func TestBuiltInReaderAgreesWithTheEngineOnFrontmatterShapes(t *testing.T) {
	const spec = "name: o\ndescription: d\ntype: agent\nexecution:\n  provider: anthropic\n  model: m\n"
	body := `{~exons.message role="system"~}hi{~/exons.message~}`
	shapes := map[string]string{
		"plain":                   "---\n" + spec + "---\n" + body,
		"crlf":                    "---\r\n" + strings.ReplaceAll(spec, "\n", "\r\n") + "---\r\n" + body,
		"bom and indent":          "\xef\xbb\xbf \t---\n" + spec + "---\n" + body,
		"closing at end of input": "---\n" + spec + "---",
		"empty frontmatter":       "---\n---\n" + body,
		"blank frontmatter":       "---\n  \n---\n" + body,
		"no frontmatter":          body,
		"newline first":           "\n---\n" + spec + "---\n" + body,
		"delimiter with text":     "---x\n" + spec + "---\n" + body,
		"closing with text":       "---\n" + spec + "---x\n" + body,
		"unclosed":                "---\n" + spec + body,
		"legacy config block":     "{~exons.config~}{}{~/exons.config~}" + body,
		"no provider":             "---\nname: o\ndescription: d\ntype: agent\n---\n" + body,
		"resources":               "---\n" + spec + "requirements:\n  resources:\n    - {ref: a, kind: corpus}\n---\n" + body,
		"allow empty":             "---\n" + spec + "tools:\n  allow: []\n---\n" + body,
	}
	refused := 0
	for name, doc := range shapes {
		t.Run(name, func(t *testing.T) {
			src := "aigentflow_version: '2.0.0'\nname: f\nversion: '1.0.0'\nstart: s\nsteps:\n  s:\n    executor: function://text/template\n" +
				"orchestrator:\n  agentic: true\n  exons: " + quote(doc) + "\n"
			builtin := aifvalidate.ValidateFlow(src, aifvalidate.Options{})
			engine := aifvalidate.ValidateFlow(src, aifvalidate.Options{Exons: New()})
			if !engine.Valid {
				refused++
			}
			if builtin.Valid != engine.Valid || !slices.Equal(codes(builtin.Errors), codes(engine.Errors)) ||
				!slices.Equal(codes(builtin.Warnings), codes(engine.Warnings)) {
				t.Errorf("built-in valid=%v %v %v; engine valid=%v %v %v", builtin.Valid, codes(builtin.Errors),
					codes(builtin.Warnings), engine.Valid, codes(engine.Errors), codes(engine.Warnings))
			}
		})
	}
	// Vacuity floor: the matrix must exercise both verdicts.
	if refused == 0 || refused == len(shapes) {
		t.Errorf("%d of %d shapes refused: the matrix cannot tell agreement from a constant", refused, len(shapes))
	}
}

// quote renders s as a YAML double-quoted scalar.
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`, "\xef\xbb\xbf", `\uFEFF`)
	return `"` + r.Replace(s) + `"`
}
