package aifvalidate

import (
	"fmt"
	"strings"
	"testing"
)

// v0.6.3: the optional top-level display_name (AIgentFlow #187). The
// conformance fixtures pin the verdict; these pin the boundary, the exact
// field and message, and the type handling a verdict cannot show.

func displayNameFlow(value string) string {
	return "display_name: " + value + "\n" + minimalFlow
}

func TestDisplayNameBoundaryIsCountedInRunes(t *testing.T) {
	cases := []struct {
		name    string
		label   string
		wantErr bool
		wantLen int
	}{
		{"absent is fine", "", false, 0},
		{"80 ASCII", strings.Repeat("a", 80), false, 80},
		{"81 ASCII", strings.Repeat("a", 81), true, 81},
		{"80 runes of 3-byte text (240 bytes)", strings.Repeat("日", 80), false, 80},
		{"81 runes of 3-byte text", strings.Repeat("日", 81), true, 81},
		{"80 runes of 4-byte text", strings.Repeat("😀", 80), false, 80},
		{"81 runes of 4-byte text", strings.Repeat("😀", 81), true, 81},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := minimalFlow
			if tc.name != "absent is fine" {
				src = displayNameFlow(fmt.Sprintf("%q", tc.label))
			}
			result := ValidateFlow(src, Options{})
			if !tc.wantErr {
				assertCodesAbsent(t, tc.name, result.Errors, []string{codeDisplayNameTooLong})
				return
			}
			assertError(t, result, codeDisplayNameTooLong, "display_name")
			e, _ := findByCode(result.Errors, codeDisplayNameTooLong)
			want := fmt.Sprintf("display_name is %d characters; the limit is 80", tc.wantLen)
			if e.Message != want {
				t.Fatalf("message = %q, want %q", e.Message, want)
			}
		})
	}
}

func TestDisplayNameNotAStringIsInvalidType(t *testing.T) {
	for _, v := range []string{"[a, b]", "{k: v}"} {
		result := ValidateFlow(displayNameFlow(v), Options{})
		assertError(t, result, codeInvalidType, "display_name")
	}
}

func TestDisplayNameIsAKnownKey(t *testing.T) {
	result := ValidateFlow(displayNameFlow("Short label"), Options{})
	assertValid(t, result)
	if hasCode(result.Warnings, "unknown_key") {
		t.Fatalf("display_name must be a known flow key, got warnings: %s", formatIssues(result.Warnings))
	}
}
