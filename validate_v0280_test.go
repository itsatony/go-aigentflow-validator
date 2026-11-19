package aifvalidate

import (
	"fmt"
	"strings"
	"testing"
)

// v0.28.0 (AIgentFlow v2.914.0): the inline size and the asset count are save-door rules.

func TestV0280InlineSizeIsCountedInBytesAfterThePinCheck(t *testing.T) {
	over := strings.Repeat("é", 131073) // 262146 bytes, 131073 characters
	at := strings.Repeat("é", 131072)   // exactly 262144 bytes
	cases := []struct{ asset, want string }{
		{`a: { content: "` + over + `" }`, `asset "a": inline content is larger than 262144 bytes`},
		{`a: { content: "` + at + `" }`, ``},
		{`a: { content: "` + over + `", sha256: "` + v0270Pin + `" }`, `asset "a": inline content takes no sha256 or version`},
	}
	for i, c := range cases {
		errs := v0270Errors(v0270Flow("assets:\n  "+c.asset, "      x: asset://a", ""))
		if c.want == "" {
			if len(errs) != 0 {
				t.Fatalf("case %d: want no errors, got %v", i, errs)
			}
			continue
		}
		if len(errs) != 1 || errs[0].Message != c.want || errs[0].Field != "assets.a" {
			t.Fatalf("case %d: want %q at assets.a, got %v", i, c.want, errs)
		}
	}
}

func TestV0280MoreThan32AssetsIsOneFindingFirst(t *testing.T) {
	block := func(n int) string {
		var b strings.Builder
		b.WriteString("assets:\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "  a%02d: { content: \"x\" }\n", i)
		}
		return b.String()
	}
	if errs := v0270Errors(v0270Flow(block(32), "      x: asset://a00", "")); len(errs) != 0 {
		t.Fatalf("32 assets: want no errors, got %v", errs)
	}
	src := v0270Flow(block(33)+"  A: { content: \"x\" }\n", "      x: asset://a00", "")
	errs := v0270Errors(src)
	if len(errs) != 2 {
		t.Fatalf("want the count finding and the bad name, got %v", errs)
	}
	if errs[0].Field != "assets" || errs[0].StepID != "" || errs[0].Message != "the flow declares 34 assets, more than 32" ||
		errs[0].Code != "asset_declaration_invalid" || errs[0].Suggestion == "" {
		t.Fatalf("the count finding comes first, at field assets: %+v", errs[0])
	}
}
