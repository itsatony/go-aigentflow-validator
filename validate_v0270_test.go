package aifvalidate

import (
	"reflect"
	"strings"
	"testing"
)

// v0.27.0 (AIgentFlow v2.913.0): assets — long step bodies kept outside the flow.

const v0270Pin = "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"

func v0270Flow(assets, query, extra string) string {
	return "aigentflow_version: \"2.0.0\"\nname: test-flow\nversion: 1.0.0\nstart: only\n" + assets +
		"\nsteps:\n  only:\n    executor: function://data/create\n" + extra + "    query:\n" + query +
		"\n    next:\n      default: 'null'\n"
}

func v0270Errors(src string) []Issue {
	var out []Issue
	for _, e := range ValidateFlow(src, Options{}).Errors {
		if strings.HasPrefix(e.Code, "asset_") {
			out = append(out, e)
		}
	}
	return out
}

func TestV0270AcceptsSourcesSitesAndProse(t *testing.T) {
	src := v0270Flow("assets:\n  a: { content: \"hello\" }\n  b: { trove_key: \"k\", sha256: \""+v0270Pin+"\" }\n"+
		"  c: { aiv_ref: \"acme/p\", version: \"1.2.0-rc.1\", sha256: \""+strings.ToUpper(v0270Pin)+"\" }",
		"      x: asset://a\n      y: [asset://b, { z: asset://c }]\n      p: \"asset://nope \"\n      q: see asset://nope", "")
	res := ValidateFlow(src, Options{})
	if len(res.Errors) != 0 {
		t.Fatalf("want no errors, got %v", res.Errors)
	}
	for _, w := range res.Warnings {
		if w.Code == codeUnknownYAMLKey {
			t.Fatalf("assets must be a known key: %v", w)
		}
	}
}

func TestV0270DeclarationProblemsInReferenceOrder(t *testing.T) {
	cases := [][2]string{
		{`A: { content: "x" }`, `asset "A": the name "A" must be lowercase letters, digits, - and _, starting with a letter (at most 64)`},
		{`a: {}`, `asset "a": it names no source: set exactly one of content, trove_key or aiv_ref`},
		{`a:`, `asset "a": it names no source: set exactly one of content, trove_key or aiv_ref`},
		{`a: { content: "x", aiv_ref: "acme/p", version: "1.0.0", sha256: "` + v0270Pin + `" }`, `asset "a": it names more than one source: set exactly one of content, trove_key or aiv_ref`},
		{`a: { content: "x", sha256: "` + v0270Pin + `" }`, `asset "a": inline content takes no sha256 or version`},
		{`a: { trove_key: "k", sha256: "abc" }`, `asset "a": a trove_key or aiv_ref asset needs its sha256 (64 hex characters) so the run reads exactly the text the flow was written for`},
		{`a: { trove_key: "k", version: "1.0.0", sha256: "` + v0270Pin + `" }`, `asset "a": a trove_key asset takes no version; its sha256 pins it`},
		{`a: { aiv_ref: "acme/p", version: "1.0", sha256: "` + v0270Pin + `" }`, `asset "a": an aiv_ref asset needs its published version (MAJOR.MINOR.PATCH)`},
		{`a: { aiv_ref: "acme/team/p", version: "1.0.0", sha256: "` + v0270Pin + `" }`, `asset "a": aiv_ref must be <org>/<name> (the aigentverse org handle and the entity's name)`},
	}
	for _, c := range cases {
		got := v0270Errors(v0270Flow("assets:\n  "+c[0], "      x: plain", ""))
		if len(got) != 1 || got[0].Message != c[1] || !strings.HasPrefix(got[0].Field, "assets.") {
			t.Errorf("%s: got %v, want %q", c[0], got, c[1])
		}
	}
}

func TestV0270UnknownAndPlacement(t *testing.T) {
	got := v0270Errors(v0270Flow("assets:\n  a:", "      x: asset://a\n      y: asset://b", ""))
	want := [][4]string{
		{"asset_declaration_invalid", `asset "a": it names no source: set exactly one of content, trove_key or aiv_ref`, "assets.a", ""},
		{"asset_reference_unknown", "step only uses asset://a, which the flow's assets: block does not declare", "steps.only", "only"},
		{"asset_reference_unknown", "step only uses asset://b, which the flow's assets: block does not declare", "steps.only", "only"},
	}
	var have [][4]string
	for _, e := range got {
		have = append(have, [4]string{e.Code, e.Message, e.Field, e.StepID})
	}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("got %v\nwant %v", have, want)
	}

	got = v0270Errors(v0270Flow(`assets: { a: { content: "x" } }`, "      x: plain", "    description: asset://a\n    condition: asset://zz\n"))
	wantCodes := []string{"asset_reference_placement", "asset_reference_placement", "asset_reference_unknown"}
	if len(got) != 3 || got[0].Code != wantCodes[0] || got[1].Code != wantCodes[1] || got[2].Code != wantCodes[2] ||
		!strings.Contains(got[0].Message, "asset://a outside") || !strings.Contains(got[1].Message, "asset://zz outside") ||
		!strings.Contains(got[2].Message, "asset://zz, which") {
		t.Errorf("placement before unknown, sorted: got %v", got)
	}

	if got := v0270Errors(v0270Flow(`assets: { a: { content: "x" } }`, "      x: plain", "    quality_gate:\n      rubric: asset://a\n")); len(got) != 0 {
		t.Errorf("a rubric is a site: %v", got)
	}
	got = v0270Errors(v0270Flow(`assets: { a: { content: "x" } }`, "      x: plain", "    quality_gate:\n      rubric: r\n      model: asset://a\n"))
	if len(got) != 1 || got[0].Code != "asset_reference_placement" {
		t.Errorf("a gate's model is not a site: %v", got)
	}
}

func TestV0270UnknownKeyInsideAnAsset(t *testing.T) {
	res := ValidateFlow(v0270Flow(`assets: { a: { content: "x", sha: "y" } }`, "      x: asset://a", ""), Options{})
	for _, i := range append(res.Errors, res.Warnings...) {
		if i.Code == codeUnknownYAMLKey {
			return
		}
	}
	t.Errorf("an unknown key inside an asset is refused: %v %v", res.Errors, res.Warnings)
}
