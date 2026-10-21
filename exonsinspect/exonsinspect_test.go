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

func codes(list []aifvalidate.Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.Code)
	}
	return out
}

// TestTheEngineChangesNoVerdict: since v0.19.0 no aifvalidate rule consults an
// ExonsInspector (AIgentFlow v2.838.0 removed the orchestrator, whose inline
// .exons document was the last one a flow could carry, and its block is refused
// whole). So every shared conformance fixture must get the SAME verdict and the
// same codes with the engine and without it.
func TestTheEngineChangesNoVerdict(t *testing.T) {
	dir := filepath.Join("..", "testdata", "conformance")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	judged := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		builtin := aifvalidate.ValidateFlow(string(src), aifvalidate.Options{})
		engine := aifvalidate.ValidateFlow(string(src), aifvalidate.Options{Exons: New()})
		judged++
		if builtin.Valid != engine.Valid || !slices.Equal(codes(builtin.Errors), codes(engine.Errors)) ||
			!slices.Equal(codes(builtin.Warnings), codes(engine.Warnings)) {
			t.Errorf("%s: without the engine valid=%v %v %v; with it valid=%v %v %v", entry.Name(),
				builtin.Valid, codes(builtin.Errors), codes(builtin.Warnings),
				engine.Valid, codes(engine.Errors), codes(engine.Warnings))
		}
	}
	if judged < 100 {
		t.Fatalf("only %d fixtures judged: the corpus did not load", judged)
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
