package aifvalidate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The examples: block (v0.7.0).
//
// testdata/conformance/examples-cases.json is the manifest of the examples-*.yaml
// fixtures: one fixture per code (plus extras for the rules that have several
// shapes) and a set of all-correct flows. The sibling JavaScript implementation
// carries byte-identical copies of both. Expected messages are typed LITERALLY in the
// manifest, never formatted from this package's constants: a table that builds its
// expectation from the constant under test passes when the constant is edited.

type examplesManifestEntry struct {
	File    string `json:"file"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
	Clean   bool   `json:"clean"`
}

func loadExamplesManifest(t testing.TB) []examplesManifestEntry {
	t.Helper()
	blob, err := os.ReadFile(filepath.Join(fixtureDir, "examples-cases.json"))
	if err != nil {
		t.Fatalf("read the examples manifest: %v", err)
	}
	var entries []examplesManifestEntry
	if err := json.Unmarshal(blob, &entries); err != nil {
		t.Fatalf("decode the examples manifest: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the examples manifest is empty")
	}
	return entries
}

// exampleCodesBySeverity splits spec.examples.codes. example_ref_blocked_host is a
// reference-only code (PARITY.md): the port never raises it, so it is excluded from
// every "must have a fixture" and "must be forbidden" set.
func exampleCodesBySeverity() (errors, warnings []string) {
	for code, severity := range spec.Examples.Codes {
		if code == "example_ref_blocked_host" {
			continue
		}
		if severity == exSeverityWarning {
			warnings = append(warnings, code)
		} else {
			errors = append(errors, code)
		}
	}
	return errors, warnings
}

func isExampleCode(code string) bool {
	_, ok := spec.Examples.Codes[code]
	return ok
}

// The conformance table gets one case per manifest entry, so TestConformance (and the
// coverage test) run the same fixtures as the rule table below.
func init() {
	blob, err := os.ReadFile(filepath.Join(fixtureDir, "examples-cases.json"))
	if err != nil {
		return // TestExamplesManifestLoads reports it
	}
	var entries []examplesManifestEntry
	if json.Unmarshal(blob, &entries) != nil {
		return
	}
	var errorCodes, warningCodes []string
	for code, severity := range spec.Examples.Codes {
		if code == "example_ref_blocked_host" {
			continue
		}
		if severity == exSeverityWarning {
			warningCodes = append(warningCodes, code)
		} else {
			errorCodes = append(errorCodes, code)
		}
	}
	for _, e := range entries {
		c := conformanceCase{file: e.File, valid: true}
		switch {
		case e.Clean:
			c.forbidErrorCodes = errorCodes
			c.forbidWarningCodes = warningCodes
		case spec.Examples.Codes[e.Code] == exSeverityWarning:
			c.wantWarningCodes = []string{e.Code}
		default:
			c.valid = false
			c.wantErrorCodes = []string{e.Code}
		}
		conformanceCases = append(conformanceCases, c)
	}
}

func TestExamplesManifestLoads(t *testing.T) {
	entries := loadExamplesManifest(t)
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(fixtureDir, e.File)); err != nil {
			t.Errorf("manifest names a missing fixture: %v", err)
		}
		if !e.Clean && !isExampleCode(e.Code) {
			t.Errorf("%s: %q is not a code of spec.examples.codes", e.File, e.Code)
		}
	}
}

// TestExamplesRules is the table: one fixture per code, asserting code, severity,
// field and (where the manifest carries one) the exact message.
func TestExamplesRules(t *testing.T) {
	covered := map[string]bool{}
	for _, e := range loadExamplesManifest(t) {
		if e.Clean {
			continue
		}
		covered[e.Code] = true
		t.Run(e.File, func(t *testing.T) {
			result := ValidateFlow(readFixture(t, e.File), Options{})
			wantSeverity := spec.Examples.Codes[e.Code]
			list := result.Errors
			if wantSeverity == exSeverityWarning {
				list = result.Warnings
			}
			var got *Issue
			for i := range list {
				if list[i].Code == e.Code && list[i].Field == e.Field {
					got = &list[i]
					break
				}
			}
			if got == nil {
				t.Fatalf("want %s %s on %q; errors: %s; warnings: %s", wantSeverity, e.Code, e.Field,
					formatIssues(result.Errors), formatIssues(result.Warnings))
			}
			if got.Severity != Severity(wantSeverity) {
				t.Errorf("severity = %q, want %q", got.Severity, wantSeverity)
			}
			if e.Message != "" && got.Message != e.Message {
				t.Errorf("message =\n  %q\nwant\n  %q", got.Message, e.Message)
			}
			if wantSeverity == exSeverityWarning && !result.Valid && !hasNonExampleOrError(result) {
				t.Errorf("a warning must never block the flow")
			}
			// The fixtures must not trip any OTHER rule of the port: a finding from
			// somewhere else would be a fixture defect hiding behind the subset check.
			for _, issue := range append(append([]Issue{}, result.Errors...), result.Warnings...) {
				if !isExampleCode(issue.Code) {
					t.Errorf("fixture trips a non-examples rule: %s[%s]: %s", issue.Code, issue.Field, issue.Message)
				}
			}
		})
	}
	errorCodes, warningCodes := exampleCodesBySeverity()
	for _, code := range append(errorCodes, warningCodes...) {
		if !covered[code] {
			t.Errorf("code %q has no fixture", code)
		}
	}
}

// hasNonExampleOrError is true when the result carries an error that is not an examples
// code, which would make "invalid" not about the warning under test.
func hasNonExampleOrError(r Result) bool {
	for _, e := range r.Errors {
		if !isExampleCode(e.Code) {
			return true
		}
	}
	return false
}

// TestExamplesCleanFlowsAreClean is the FALSE-POSITIVE half: every all-correct flow
// produces no examples finding at all, because a rule whose entire risk is a false
// positive is invisible to a table of "this must fire" cases.
func TestExamplesCleanFlowsAreClean(t *testing.T) {
	clean := 0
	for _, e := range loadExamplesManifest(t) {
		if !e.Clean {
			continue
		}
		clean++
		t.Run(e.File, func(t *testing.T) {
			result := ValidateFlow(readFixture(t, e.File), Options{})
			for _, issue := range append(append([]Issue{}, result.Errors...), result.Warnings...) {
				if isExampleCode(issue.Code) {
					t.Errorf("an all-correct flow produced %s[%s]: %s", issue.Code, issue.Field, issue.Message)
				}
			}
			if !result.Valid {
				t.Errorf("an all-correct flow must be valid: %s", formatIssues(result.Errors))
			}
		})
	}
	if clean < 6 {
		t.Fatalf("expected the clean flows (base, variants, checkpoints, secret, exact, prose), found %d", clean)
	}
}

// examplesBase returns the base flow's text with `old` replaced by `new`.
func examplesBase(t *testing.T, old, new string) string {
	t.Helper()
	src := readFixture(t, "examples-valid-base.yaml")
	if strings.Count(src, old) != 1 {
		t.Fatalf("fixture drift: %q occurs %d times", old, strings.Count(src, old))
	}
	return strings.Replace(src, old, new, 1)
}

func findIssue(list []Issue, code, field string) (Issue, bool) {
	for _, i := range list {
		if i.Code == code && i.Field == field {
			return i, true
		}
	}
	return Issue{}, false
}

// TestExamplesSecretLikeEachPattern varies the INPUT: one value per credential shape,
// built at run time so no secret-shaped literal sits in the source (gitleaks).
func TestExamplesSecretLikeEachPattern(t *testing.T) {
	shapes := map[string]string{
		"an API key":             "sk-" + strings.Repeat("a", 24),
		"a platform API key":     "crn_" + strings.Repeat("a", 20),
		"an AWS access key":      "AKIA" + strings.Repeat("A", 16),
		"a private key":          "-----BEGIN " + "RSA PRIVATE KEY-----",
		"a bearer token":         "Bearer " + strings.Repeat("a", 24),
		"a Slack token":          "xoxb-" + strings.Repeat("1", 12),
		"a GitHub token":         "ghp_" + strings.Repeat("a", 34),
		"a retired registry key": "aivk_" + strings.Repeat("a", 24),
	}
	if len(shapes) != len(spec.Examples.SecretPatterns) {
		t.Fatalf("this table must name every pattern: have %d shapes, %d patterns", len(shapes), len(spec.Examples.SecretPatterns))
	}
	for name, value := range shapes {
		if got := exampleSecretMatch("prefix " + value + " suffix"); got != name {
			t.Errorf("value for %q matched %q", name, got)
		}
		// And through the whole validator, in a place the walker scans.
		src := examplesBase(t, `rubric: "The vendor is spelled as printed."`, `rubric: "prefix `+value+` suffix"`)
		if _, ok := findIssue(ValidateFlow(src, Options{}).Errors, codeExSecretLikeValue, "examples[0].expected.rubric"); !ok {
			t.Errorf("a %s in the rubric must be refused", name)
		}
	}
	// A false positive is worse than a miss for ordinary prose.
	for _, ok := range []string{
		"risk-assessment-for-the-quarterly-report-2025",
		"desk-reservation-system-for-all-floors-of-the-office",
		"The task-force-meeting-notes-are-attached-below-in-full",
		"bearer of bad news",
		"crn_short",
		"api_key: \"aivk_placeholder\"",
	} {
		if got := exampleSecretMatch(ok); got != "" {
			t.Errorf("%q is ordinary text but matched %q", ok, got)
		}
	}
}

func TestExamplesSecretPatternsCompile(t *testing.T) {
	if len(exSecretRes) != len(spec.Examples.SecretPatterns) || len(exSecretRes) == 0 {
		t.Fatalf("compiled %d of %d patterns", len(exSecretRes), len(spec.Examples.SecretPatterns))
	}
}

// TestExamplesNarrowedInputCheck pins what the narrowed input check does and does
// not judge, one probe per kind. The "does not judge" half matters as much: the port
// may be weaker than the reference, never stricter.
func TestExamplesNarrowedInputCheck(t *testing.T) {
	const in = "      currency_hint: EUR\n"
	refused := map[string]struct{ old, new, field string }{
		"string given a number":    {`invoice_text: "Mueller GmbH Gesamtbetrag 1.190,00 EUR"`, `invoice_text: 7`, "examples[0].input.invoice_text"},
		"string given a list":      {`invoice_text: "Mueller GmbH Gesamtbetrag 1.190,00 EUR"`, `invoice_text: [a]`, "examples[0].input.invoice_text"},
		"enum given a number":      {`currency_hint: EUR`, `currency_hint: 3`, "examples[0].input.currency_hint"},
		"enum outside the list":    {`currency_hint: EUR`, `currency_hint: GBP`, "examples[0].input.currency_hint"},
		"unknown field":            {in, in + "      mystery: 1\n", "examples[0].input.mystery"},
		"file given a bare string": {in, in + "      scan: slot_1\n", "examples[0].input.scan"},
	}
	for name, c := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			result := ValidateFlow(examplesBase(t, c.old, c.new), Options{})
			if _, ok := findIssue(result.Errors, codeExInputInvalid, c.field); !ok {
				if _, ok := findIssue(result.Errors, codeExFileInputNeedsRef, c.field); !ok {
					t.Fatalf("want an input error on %q; got %s", c.field, formatIssues(result.Errors))
				}
			}
		})
	}
	// NOT judged: length, min/max, pattern and date-format constraints. A value the
	// reference would refuse on those grounds is accepted here; the host application
	// refuses the union of both verdicts.
	notJudged := map[string]string{
		"max_length":  strings.Repeat("x", 20001), // invoice_text declares max_length 20000
		"empty value": "",
	}
	for name, value := range notJudged {
		t.Run("not-judged/"+name, func(t *testing.T) {
			src := examplesBase(t, `invoice_text: "Mueller GmbH Gesamtbetrag 1.190,00 EUR"`, `invoice_text: "`+value+`"`)
			result := ValidateFlow(src, Options{})
			if _, ok := findIssue(result.Errors, codeExInputInvalid, "examples[0].input.invoice_text"); ok {
				t.Fatalf("the narrowed check must not judge %s", name)
			}
		})
	}
}

// TestExamplesInputKinds covers number, bool, array_of_strings, date, visible_when and
// the unquoted-date normalisation on a flow built for them.
func TestExamplesInputKinds(t *testing.T) {
	src := readFixture(t, "examples-valid-exact.yaml")
	probe := func(old, new string) Result {
		if strings.Count(src, old) < 1 {
			t.Fatalf("fixture drift: %q", old)
		}
		return ValidateFlow(strings.Replace(src, old, new, 1), Options{})
	}
	if r := ValidateFlow(src, Options{}); !r.Valid {
		t.Fatalf("the exact flow must be valid: %s", formatIssues(r.Errors))
	}
	cases := []struct {
		name, old, new, field string
	}{
		{"number given a string", "      net: 1000\n      rate: 19\n", "      net: \"1000\"\n      rate: 19\n", "examples[0].input.net"},
		{"bool given a string", "reverse_charge: false", "reverse_charge: \"no\"", "examples[0].input.reverse_charge"},
		{"array given a scalar", "labels: [retail, domestic]", "labels: retail", "examples[0].input.labels"},
		{"array item not a string", "labels: [retail, domestic]", "labels: [retail, 5]", "examples[0].input.labels"},
		{"date given a number", "on_date: 2026-01-01", "on_date: 5", "examples[0].input.on_date"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := probe(c.old, c.new)
			if _, ok := findIssue(r.Errors, codeExInputInvalid, c.field); !ok {
				t.Fatalf("want example_input_invalid on %q; got %s", c.field, formatIssues(r.Errors))
			}
		})
	}
	// An unquoted YAML date decodes to a timestamp; a live run receives a string, so
	// it must NOT be a kind error (the false-positive this normalisation exists for).
	if r := probe("on_date: 2026-01-01", "on_date: 2026-01-01"); !r.Valid {
		t.Errorf("an unquoted date is a string at run time: %s", formatIssues(r.Errors))
	}
	// A required field behind a visible_when whose dependency is absent is not judged.
	hidden := strings.Replace(readFixture(t, "examples-valid-base.yaml"),
		`{ name: currency_hint, type: enum, label: "Expected currency", enum: [EUR, USD, CHF], default: EUR }`,
		`{ name: currency_hint, type: enum, label: "Expected currency", enum: [EUR, USD, CHF], required: true, visible_when: { field: scan, equals: x } }`, 1)
	hidden = strings.Replace(hidden, "      currency_hint: EUR\n", "", 1)
	if r := ValidateFlow(hidden, Options{}); hasCode(r.Errors, codeExInputInvalid) {
		t.Errorf("a hidden required field is not required: %s", formatIssues(r.Errors))
	}
}

// TestExamplesVariantRules pins the variant paths the table cannot: a null patch value
// deletes a required secret without a finding, and an input_ref parent leaves the patch
// unchecked rather than wrong.
func TestExamplesVariantRules(t *testing.T) {
	r := ValidateFlow(readFixture(t, "examples-valid-secret-omitted.yaml"), Options{})
	for _, issue := range append(append([]Issue{}, r.Errors...), r.Warnings...) {
		if isExampleCode(issue.Code) {
			t.Errorf("a variant deleting a secret with null must be fine: %s[%s]", issue.Code, issue.Field)
		}
	}
	// The patched input is judged: the patch makes a required field null.
	src := examplesBase(t, "    expected:\n      fields:", "    variants:\n      - { id: drops_text, origin: author, input_patch: { invoice_text: null } }\n    expected:\n      fields:")
	got := ValidateFlow(src, Options{})
	if _, ok := findIssue(got.Errors, codeExVariantInputBad, "examples[0].variants[0].input_patch.invoice_text"); !ok {
		t.Errorf("a patch that deletes a required field must be refused: %s", formatIssues(got.Errors))
	}
}

func TestApplyExamplePatch(t *testing.T) {
	base := doc{"a": 1, "b": doc{"c": 2, "d": 3}, "e": "x"}
	got := applyExamplePatch(base, doc{"b": doc{"c": nil, "f": 4}, "e": nil, "g": []any{1}})
	b := got["b"].(doc)
	if _, has := b["c"]; has || b["d"] != 3 || b["f"] != 4 {
		t.Errorf("nested merge/delete wrong: %+v", got)
	}
	if _, has := got["e"]; has {
		t.Errorf("null must delete a key: %+v", got)
	}
	if got["a"] != 1 || got["g"] == nil {
		t.Errorf("untouched/added keys wrong: %+v", got)
	}
	if _, has := base["e"]; !has || base["b"].(doc)["c"] != 2 {
		t.Errorf("base was mutated: %+v", base)
	}
	b["d"] = 99
	if base["b"].(doc)["d"] != 3 {
		t.Errorf("result shares a nested map with the base")
	}
}

// TestExamplesUnknownKeyNamesTheKey: a typo at any depth is refused through the
// generated key sets, with nothing hand-listed.
func TestExamplesUnknownKeyNamesTheKey(t *testing.T) {
	cases := []struct{ name, old, new, field string }{
		{"example level", "    guidance:", "    guidence:", "examples[0].guidence"},
		{"expected level", "      rubric:", "      rubrik:", "examples[0].expected.rubrik"},
		{"matcher level", "tolerance: 0.005", "tolerence: 0.005", "examples[0].expected.fields.total.tolerence"},
		{"variant level", "    expected:\n", "    variants:\n      - { id: v_one, origin: author, hold_out: true, input_patch: { invoice_text: z } }\n    expected:\n", "examples[0].variants[0].hold_out"},
		{"flow level", "\nexamples:", "\nexample:", "example"},
		{"checkpoint level", "    expected:\n", "    checkpoints:\n      extract: { rubric: r, optionnal: true }\n    expected:\n", "examples[0].checkpoints.extract.optionnal"},
		{"file ref level", "    input:\n", "    input_ref: { url: \"https://docs.example.org/a.json\", sha: x }\n    inputx:\n", "examples[0].input_ref.sha"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ValidateFlow(examplesBase(t, c.old, c.new), Options{})
			if _, ok := findIssue(r.Errors, codeUnknownYAMLKey, c.field); !ok {
				t.Fatalf("want unknown_yaml_key on %q; got %s", c.field, formatIssues(r.Errors))
			}
		})
	}
	// The retired-example-free base is still accepted: a key INSIDE the open maps
	// (input, exact) is the author's own.
	if r := ValidateFlow(readFixture(t, "examples-valid-exact.yaml"), Options{}); hasCode(r.Errors, codeUnknownYAMLKey) {
		t.Errorf("keys inside input/exact are open: %s", formatIssues(r.Errors))
	}
}

// TestExamplesFlowWithoutExamplesIsUnchanged: every flow that predates the grammar.
func TestExamplesFlowWithoutExamplesIsUnchanged(t *testing.T) {
	r := ValidateFlow(readFixture(t, "valid-minimal.yaml"), Options{})
	for _, issue := range append(append([]Issue{}, r.Errors...), r.Warnings...) {
		if isExampleCode(issue.Code) {
			t.Errorf("a flow without examples must carry no examples finding: %s", issue.Code)
		}
	}
}

// TestExamplesCodesMatchTheSpecBothWays holds the walker's code constants against
// spec.examples.codes in both directions, derived from the source rather than listed:
// a code raised but absent from the spec would get no severity (and so be treated as an
// error), and a spec code the walker never raises is a rule that does not exist.
func TestExamplesCodesMatchTheSpecBothWays(t *testing.T) {
	src, err := os.ReadFile("validate.examples.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(examples?_[a-z0-9_]+)"`).FindAllStringSubmatch(string(src), -1) {
		used[m[1]] = true
	}
	for code := range used {
		if _, ok := spec.Examples.Codes[code]; !ok {
			t.Errorf("the walker raises %q, which spec.examples.codes does not list", code)
		}
	}
	for code := range spec.Examples.Codes {
		if code == "example_ref_blocked_host" {
			continue // reference-only, PARITY.md
		}
		if !used[code] {
			t.Errorf("spec.examples.codes lists %q, which the walker never raises", code)
		}
	}
	if used["example_ref_blocked_host"] {
		t.Errorf("example_ref_blocked_host is reference-only and must not be raised by the port")
	}
}

// TestExamplesNoInputFlow: `input: {}` is the documented spelling for a flow that
// takes no input; it is fine (the example just cannot be checked, a warning).
func TestExamplesNoInputFlow(t *testing.T) {
	src := `aigentflow_version: "2.0.0"
name: no_input
description: "Takes no input."
version: "1.0.0"
start: only
output: [answer]
steps:
  only:
    executor: function://text/noop
examples:
  - id: the_one_case
    title: "The only case"
    guidance: "It needs nothing."
    input: {}
    expected:
      rubric: "Says hello."
`
	r := ValidateFlow(src, Options{})
	for _, e := range r.Errors {
		if isExampleCode(e.Code) {
			t.Errorf("input: {} for a flow without input must not be an error: %s[%s]", e.Code, e.Field)
		}
	}
	if !hasCode(r.Warnings, codeExInputUnvalidated) {
		t.Errorf("an input nobody can check is a warning: %s", formatIssues(r.Warnings))
	}
}

// TestExamplesInputObjectForm: ValidateFlowObject (no source text) reaches the same
// walker.
func TestExamplesInputObjectForm(t *testing.T) {
	flow, errs, _ := ParseFlow(examplesBase(t, "        total: { approx", "        totl: { approx"))
	if len(errs) != 0 {
		t.Fatalf("parse: %v", errs)
	}
	r := ValidateFlowObject(flow, Options{})
	if _, ok := findIssue(r.Errors, codeExExpectedFieldUnk, "examples[0].expected.fields.totl"); !ok {
		t.Errorf("the object form must run the walker: %s", formatIssues(r.Errors))
	}
}
