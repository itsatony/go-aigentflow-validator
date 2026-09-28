package exonsinspect

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	aifvalidate "github.com/itsatony/go-aigentflow-validator"
)

// The differential against the AIgentFlow reference.
//
// The reference is closed source, so this test cannot run it. It reads the
// reference's verdicts from a JSON file (AIF_REFERENCE_VERDICTS) that a program
// on the reference's side writes by running its save-door verdict over a
// corpus of flow files, and compares this library's verdict on each file. The
// file maps each flow's absolute path to:
//
//	{"valid": bool, "parse_refusal": bool, "message": "...",
//	 "errors": [{"code": "...", "field": "..."}], "warnings": [...]}
//
// parse_refusal marks a document the reference's strict parse refused. The
// reference then reports ONE error under a generic code (its parse-door
// refusals carry no rule code), so only the verdict is compared; everywhere
// else the error CODE SETS must be identical, and so must the warning code sets
// apart from the scoped divergences below.
//
// The test skips when AIF_REFERENCE_VERDICTS is unset, so CI on a bare clone
// passes. When it is set, any divergence not listed below fails.

// referenceOnlyWarnings are warnings the reference emits and this library does
// not (PARITY.md, divergences #14 and #17).
var referenceOnlyWarnings = map[string]bool{
	"template_missing_field":             true,
	"compliance_catalog_missing":         true,
	"condition_not_boolean":              true,
	"INPUT_SCHEMA_FILE_AFTER_PARAMETRIC": true,
}

// libraryOnlyWarnings are warnings this library emits and the reference does
// not (PARITY.md, divergences #14 and #17).
var libraryOnlyWarnings = map[string]bool{
	"unknown_executor_scheme":            true,
	"unknown_data_type":                  true,
	"input_schema_file_after_parametric": true,
}

type refFinding struct {
	Code  string `json:"code"`
	Field string `json:"field"`
}

type refVerdict struct {
	Valid        bool         `json:"valid"`
	ParseRefusal bool         `json:"parse_refusal"`
	Message      string       `json:"message"`
	Errors       []refFinding `json:"errors"`
	Warnings     []refFinding `json:"warnings"`
}

type divergence struct {
	file    string
	verdict bool // the valid flag differs (else only the code sets do)
	detail  string
}

type diffStats struct {
	files, verdictAgree, codesAgree, refInvalid int
	divergences                                 []divergence
}

func TestDifferentialAgainstReference(t *testing.T) {
	path := os.Getenv("AIF_REFERENCE_VERDICTS")
	if path == "" {
		t.Skip("AIF_REFERENCE_VERDICTS not set: no reference verdicts to compare against")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ref := map[string]refVerdict{}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref) == 0 {
		t.Fatal("the reference verdict file holds no verdicts: a differential over nothing passes vacuously")
	}
	files := make([]string, 0, len(ref))
	for f := range ref {
		files = append(files, f)
	}
	sort.Strings(files)

	engine := runDifferential(t, files, ref, aifvalidate.Options{StrictRegistries: true, Exons: New()})
	builtin := runDifferential(t, files, ref, aifvalidate.Options{StrictRegistries: true})
	t.Logf("[exonsinspect]    files=%d reference-refused=%d verdict-agree=%d code-sets-agree=%d",
		engine.files, engine.refInvalid, engine.verdictAgree, engine.codesAgree)
	t.Logf("[built-in reader] files=%d reference-refused=%d verdict-agree=%d code-sets-agree=%d",
		builtin.files, builtin.refInvalid, builtin.verdictAgree, builtin.codesAgree)
	if engine.refInvalid == 0 || engine.refInvalid == engine.files {
		t.Errorf("the corpus is one-sided (%d of %d refused): it cannot tell a validator that accepts "+
			"everything from one that refuses everything", engine.refInvalid, engine.files)
	}

	// With the engine-backed inspector every verdict must be the reference's.
	for _, d := range engine.divergences {
		t.Errorf("[exonsinspect] %s: %s", d.file, d.detail)
	}
	// Without it the one permitted divergence is PARITY.md #16: a document the
	// reference's .exons ENGINE refuses at its parse door, which the built-in
	// reader cannot judge. It is looser, never stricter, and exonsinspect must
	// agree on the same file (checked above).
	for _, d := range builtin.divergences {
		r := ref[d.file]
		if d.verdict && r.ParseRefusal && strings.Contains(r.Message, "exons") {
			t.Logf("[built-in reader] permitted (divergence #16): %s: %s", d.file, d.detail)
			continue
		}
		t.Errorf("[built-in reader] %s: %s", d.file, d.detail)
	}
}

func runDifferential(t *testing.T, files []string, ref map[string]refVerdict, opts aifvalidate.Options) diffStats {
	t.Helper()
	stats := diffStats{files: len(files)}
	for _, f := range files {
		r := ref[f]
		if !r.Valid {
			stats.refInvalid++
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		got := aifvalidate.ValidateFlow(string(src), opts)
		if got.Valid != r.Valid {
			stats.divergences = append(stats.divergences, divergence{file: f, verdict: true,
				detail: fmt.Sprintf("reference valid=%v, library valid=%v, library errors %v; reference: %s",
					r.Valid, got.Valid, sortedCodes(issueCodes(got.Errors)), firstLine(r.Message))})
			continue
		}
		stats.verdictAgree++
		if r.ParseRefusal {
			stats.codesAgree++
			continue
		}
		refErr, gotErr := findingCodes(r.Errors), issueCodes(got.Errors)
		refWarn, gotWarn := findingCodes(r.Warnings), issueCodes(got.Warnings)
		onlyRef, onlyGot := minus(refErr, gotErr, nil), minus(gotErr, refErr, nil)
		wRef, wGot := minus(refWarn, gotWarn, referenceOnlyWarnings), minus(gotWarn, refWarn, libraryOnlyWarnings)
		if len(onlyRef)+len(onlyGot)+len(wRef)+len(wGot) == 0 {
			stats.codesAgree++
			continue
		}
		stats.divergences = append(stats.divergences, divergence{file: f,
			detail: fmt.Sprintf("errors reference-only=%v library-only=%v; warnings reference-only=%v library-only=%v",
				onlyRef, onlyGot, wRef, wGot)})
	}
	return stats
}

func findingCodes(list []refFinding) map[string]bool {
	out := map[string]bool{}
	for _, f := range list {
		out[f.Code] = true
	}
	return out
}

func issueCodes(list []aifvalidate.Issue) map[string]bool {
	out := map[string]bool{}
	for _, i := range list {
		out[i.Code] = true
	}
	return out
}

func sortedCodes(set map[string]bool) []string {
	return minus(set, nil, nil)
}

// minus returns the codes of a absent from b, skipping the allowed ones.
func minus(a, b, allowed map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] && !allowed[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const maxLen = 200
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}
