package aifvalidate

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Options.StoredBytes demotes exactly the spec block storedBytesTolerated, and
// changes nothing else about a finding.

func codeSet(issues []Issue) map[string]bool {
	out := map[string]bool{}
	for _, i := range issues {
		out[i.Code] = true
	}
	return out
}

func toleratedSet() map[string]bool {
	out := map[string]bool{}
	for _, c := range spec.StoredBytesTolerated.Codes {
		out[c] = true
	}
	return out
}

func TestStoredBytesDemotesExactlyTheToleratedCodesOnEveryFixture(t *testing.T) {
	files, err := filepath.Glob("testdata/conformance/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no conformance fixtures: %v", err)
	}
	tolerated := toleratedSet()
	seen := map[string]bool{}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		save := ValidateFlow(string(raw), Options{})
		stored := ValidateFlow(string(raw), Options{StoredBytes: true})

		var wantErrors, demoted []Issue
		for _, e := range save.Errors {
			if tolerated[e.Code] {
				demoted = append(demoted, e)
				seen[e.Code] = true
			} else {
				wantErrors = append(wantErrors, e)
			}
		}
		if len(stored.Errors) != len(wantErrors) {
			t.Errorf("%s: stored errors %v, want %v", path, codeList(stored.Errors), codeList(wantErrors))
		}
		if stored.Valid != (len(wantErrors) == 0) {
			t.Errorf("%s: stored Valid=%v with %d remaining errors", path, stored.Valid, len(wantErrors))
		}
		if len(stored.Warnings) != len(save.Warnings)+len(demoted) {
			t.Errorf("%s: stored warnings %d, want %d", path, len(stored.Warnings), len(save.Warnings)+len(demoted))
		}
		warned := map[string]int{}
		for _, w := range stored.Warnings {
			warned[w.Code+"|"+w.Field+"|"+w.Message]++
		}
		for _, d := range demoted {
			key := d.Code + "|" + d.Field + "|" + d.Message
			if warned[key] == 0 {
				t.Errorf("%s: demoted finding %s lost its field or message", path, d.Code)
			}
			warned[key]--
		}
		for _, w := range stored.Warnings {
			if w.Severity != SeverityWarning {
				t.Errorf("%s: warning %s carries severity %q", path, w.Code, w.Severity)
			}
		}
		if stored.Summary.ErrorCount != len(stored.Errors) || stored.Summary.WarningCount != len(stored.Warnings) {
			t.Errorf("%s: summary counts out of step with the findings", path)
		}
	}
	// A code in the spec list that no fixture raises is a typo or a dead entry.
	var missing []string
	for _, c := range spec.StoredBytesTolerated.Codes {
		if !seen[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("tolerated codes no conformance fixture raises: %v", missing)
	}
}

func TestStoredBytesForEachBareItemsIsValidWithTheWarning(t *testing.T) {
	raw, err := os.ReadFile("testdata/conformance/invalid-for-each-items-bare-path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res := ValidateFlow(string(raw), Options{}); res.Valid || !codeSet(res.Errors)[codeForEachItemsUnresolvable] {
		t.Fatalf("the save door must refuse it: valid=%v errors=%v", res.Valid, codeList(res.Errors))
	}
	res := ValidateFlow(string(raw), Options{StoredBytes: true})
	if !res.Valid || !codeSet(res.Warnings)[codeForEachItemsUnresolvable] {
		t.Fatalf("stored bytes must load with the warning: valid=%v warnings=%v errors=%v", res.Valid, codeList(res.Warnings), codeList(res.Errors))
	}
}

func TestStoredBytesKeepsAnUntoleratedErrorAnError(t *testing.T) {
	raw, err := os.ReadFile("testdata/conformance/invalid-flow-data-key-namespace.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res := ValidateFlow(string(raw), Options{StoredBytes: true})
	if res.Valid || !codeSet(res.Errors)[codeFlowDataKeyReserved] {
		t.Fatalf("a rule AIgentFlow enforces on load must stay an error: valid=%v errors=%v", res.Valid, codeList(res.Errors))
	}
}

func TestStoredBytesDoesNotMaskAParseFailure(t *testing.T) {
	res := ValidateFlow("name: [unterminated", Options{StoredBytes: true})
	if res.Valid {
		t.Fatal("an unparseable document is never valid, stored or not")
	}
}

func codeList(issues []Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}
