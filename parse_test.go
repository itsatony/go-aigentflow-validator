package aifvalidate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFlowDiagnostics(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		wantCode string
	}{
		{"empty", "", codeEmptyDocument},
		{"whitespace only", "   \n\t\n", codeEmptyDocument},
		{"scalar root", "just a string\n", codeInvalidRoot},
		{"sequence root", "- a\n- b\n", codeInvalidRoot},
		{"duplicate key", "aigentflow_version: \"1\"\nname: a\nname: b\n", codeDuplicateKey},
		{"tab indent", "a:\n\tb: 1\n", codeYAMLSyntax},
		{"unclosed bracket", "a: [1, 2\n", codeYAMLSyntax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flow, errs, _ := ParseFlow(tc.source)
			if flow != nil {
				t.Errorf("expected no document on a parse failure, got %v", flow)
			}
			if !hasCode(errs, tc.wantCode) {
				t.Errorf("want code %q, got: %s", tc.wantCode, formatIssues(errs))
			}
			// A parse failure must also produce an invalid Result, not just
			// diagnostics — the two entry points must agree.
			if result := ValidateFlow(tc.source, Options{}); result.Valid {
				t.Errorf("ValidateFlow reported valid for a document that does not parse")
			}
		})
	}
}

func TestParseFlowRejectsOversizeAndAliasBombs(t *testing.T) {
	t.Run("over MaxDocumentBytes", func(t *testing.T) {
		// Not a valid flow either way; the point is that the size guard fires
		// BEFORE the parser is handed the bytes.
		big := "name: x\ndescription: " + strings.Repeat("a", MaxDocumentBytes) + "\n"
		_, errs, _ := ParseFlow(big)
		if !hasCode(errs, codeYAMLSyntax) {
			t.Fatalf("expected the size guard to fire, got: %s", formatIssues(errs))
		}
		if !strings.Contains(errs[0].Message, "maximum") {
			t.Errorf("size-guard message should name the limit, got %q", errs[0].Message)
		}
	})
	t.Run("over MaxAliasTokens", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("anchor: &a value\nlist:\n")
		for range MaxAliasTokens + 5 {
			b.WriteString("  - *a\n")
		}
		_, errs, _ := ParseFlow(b.String())
		if !hasCode(errs, codeYAMLSyntax) {
			t.Fatalf("expected the alias guard to fire, got: %s", formatIssues(errs))
		}
	})
	t.Run("a literal asterisk in prose is not an alias", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "version: 1.0.0",
			"version: 1.0.0\ndescription: \"rated 5 * stars * always * every * time\"", 1)
		if result := ValidateFlow(src, Options{}); !result.Valid {
			t.Errorf("prose asterisks must not trip the alias guard: %s", formatIssues(result.Errors))
		}
	})
}

func TestTemplateSyntaxChecks(t *testing.T) {
	wrap := func(tmpl string) string {
		return "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: ai://chat\n    query:\n      prompt: " +
			quoteYAML(tmpl) + "\n"
	}
	t.Run("unclosed action", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("{{ .query.x "), Options{}),
			codeTemplateSyntax, "steps.a.query.prompt")
	})
	t.Run("empty action", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("{{}}"), Options{}), codeTemplateSyntax, "steps.a.query.prompt")
	})
	t.Run("unbalanced if without end", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("{{ if .a }}yes"), Options{}),
			codeTemplateSyntax, "steps.a.query.prompt")
	})
	t.Run("else outside a block", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("{{ else }}"), Options{}),
			codeTemplateSyntax, "steps.a.query.prompt")
	})
	t.Run("stray closing delimiter is NOT an error", func(t *testing.T) {
		// Go treats it as literal text; the reference does too.
		assertValid(t, ValidateFlow(wrap("all done }} here"), Options{}))
	})
	t.Run("balanced constructs pass", func(t *testing.T) {
		for _, tmpl := range []string{
			"{{ .query.x }}",
			"{{ if .a }}yes{{ else }}no{{ end }}",
			"{{ range .items }}{{ . }}{{ end }}",
			"{{ with .a }}{{ . }}{{ end }}",
			"{{/* a comment */}}",
			"{{ eq .a 1 }}",
			"{{ .a | upper }}",
		} {
			if result := ValidateFlow(wrap(tmpl), Options{}); !result.Valid {
				t.Errorf("template %q must be valid: %s", tmpl, formatIssues(result.Errors))
			}
		}
	})
	t.Run("every spec template function parses in command position", func(t *testing.T) {
		for _, fn := range TemplateFunctionNames() {
			result := ValidateFlow(wrap("{{ "+fn+" }}"), Options{StrictRegistries: true})
			if hasCode(result.Errors, codeTemplateFuncUnkn) {
				t.Errorf("allow-listed function %q was reported unknown", fn)
			}
		}
	})
	t.Run("unknown function is silent by default, an error under StrictRegistries", func(t *testing.T) {
		src := wrap("{{ notARealFunction .a }}")
		if result := ValidateFlow(src, Options{}); !result.Valid {
			t.Errorf("an unknown function must not block by default: %s", formatIssues(result.Errors))
		}
		assertError(t, ValidateFlow(src, Options{StrictRegistries: true}),
			codeTemplateFuncUnkn, "steps.a.query.prompt")
	})
	t.Run("an unknown function does not mask a real syntax error", func(t *testing.T) {
		// The stub-and-retry loop exists for exactly this: a single-pass parse
		// would stop at the unknown name and never see the unclosed action.
		assertError(t, ValidateFlow(wrap("{{ notARealFunction .a }}{{ if .b }}"), Options{}),
			codeTemplateSyntax, "steps.a.query.prompt")
	})
	t.Run("conditions[].if is checked", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      conditions:
        - if: "{{ if .a }}"
          goto_step: a
`
		assertError(t, ValidateFlow(src, Options{}),
			codeTemplateSyntax, "steps.a.next.conditions[0].if")
	})
	t.Run("response_expectation templates are counted but not checked", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    response_expectation:\n      out:\n        type: string\n        required: \"{{ if .a }}\"", 1)
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeTemplateSyntax) {
			t.Errorf("response_expectation templates must not be syntax-checked: %s",
				formatIssues(result.Errors))
		}
		if result.Summary.TemplatesFound == 0 {
			t.Error("response_expectation templates must still be counted")
		}
	})
}

func TestSummaryCounters(t *testing.T) {
	src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: ai://chat
    query:
      good: "{{ .query.x }}"
      bad: "{{ .query.y "
  b:
    executor: function://n
`
	result := ValidateFlow(src, Options{})
	if result.Summary.TotalSteps != 2 {
		t.Errorf("TotalSteps = %d, want 2", result.Summary.TotalSteps)
	}
	// Only step `a` carries an error, so `b` remains valid even though it also
	// carries an unreachable_step WARNING.
	if result.Summary.ValidSteps != 1 {
		t.Errorf("ValidSteps = %d, want 1", result.Summary.ValidSteps)
	}
	if result.Summary.TemplatesFound != 2 {
		t.Errorf("TemplatesFound = %d, want 2", result.Summary.TemplatesFound)
	}
	if result.Summary.TemplatesValid != 1 {
		t.Errorf("TemplatesValid = %d, want 1", result.Summary.TemplatesValid)
	}
	if result.Summary.TemplatesValid > result.Summary.TemplatesFound {
		t.Error("TemplatesValid must never exceed TemplatesFound")
	}
}

func TestFindingsCarrySourcePositions(t *testing.T) {
	// Positions are what make a finding navigable in an editor. Asserted on the
	// LINE, because that is what a consumer jumps to.
	src := "aigentflow_version: \"2.0.0\"\n" + // 1
		"name: f\n" + // 2
		"start: a\n" + // 3
		"steps:\n" + // 4
		"  a:\n" + // 5
		"    executor: function://n\n" + // 6
		"    next:\n" + // 7
		"      default: ghost\n" // 8

	result := ValidateFlow(src, Options{})
	issue, ok := findByCode(result.Errors, codeStepNotFound)
	if !ok {
		t.Fatalf("expected step_not_found, got: %s", formatIssues(result.Errors))
	}
	if issue.Line != 8 {
		t.Errorf("dangling next.default reported on line %d, want 8", issue.Line)
	}
	if issue.Column == 0 {
		t.Error("a positioned finding should carry a column too")
	}
}

func TestMissingFieldPositionFallsBackToTheNearestAncestor(t *testing.T) {
	// A missing field has no node of its own, so the position must resolve to the
	// enclosing step. Landing the author on line 1 would be useless.
	src := "aigentflow_version: \"2.0.0\"\n" + // 1
		"name: f\n" + // 2
		"start: a\n" + // 3
		"steps:\n" + // 4
		"  a:\n" + // 5
		"    query: {}\n" // 6

	result := ValidateFlow(src, Options{})
	issue, ok := findByCode(result.Errors, codeMissingField)
	if !ok {
		t.Fatalf("expected missing_required_field, got: %s", formatIssues(result.Errors))
	}
	if issue.Field != "steps.a.executor" {
		t.Fatalf("field = %q, want steps.a.executor", issue.Field)
	}
	if issue.Line != 5 {
		t.Errorf("missing executor reported on line %d, want 5 (the step key)", issue.Line)
	}
}

// TestResultSlicesMarshalAsArraysNotNull guards the failure mode a typed
// consumer cannot see: a `null` where an array was promised, which only surfaces
// when client code calls an array method on it. Asserted on the WIRE (a JSON
// round-trip), not on the struct — a nil slice is not nil-ness a consumer sees,
// `null` is.
func TestResultSlicesMarshalAsArraysNotNull(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"valid flow", minimalFlow},
		{"invalid flow", strings.Replace(minimalFlow, "function://noop", "notauri", 1)},
		{"unparseable", "- not a mapping\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(ValidateFlow(tc.source, Options{}))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for _, key := range []string{"errors", "warnings"} {
				raw, present := wire[key]
				if !present {
					t.Fatalf("%q missing from the wire shape", key)
				}
				if string(raw) == "null" {
					t.Errorf("%q marshalled as null; it must always be an array", key)
				}
			}
			if _, present := wire["spec_version"]; !present {
				t.Error("spec_version missing: a stored verdict must stay self-describing")
			}
		})
	}
}

func TestValidateFlowObjectMatchesValidateFlow(t *testing.T) {
	// The two entry points must agree on the verdict; only positions differ,
	// because the object form has no source to locate findings in.
	src := strings.Replace(minimalFlow, "function://noop", "notauri", 1)
	fromText := ValidateFlow(src, Options{})
	flow, _, _ := ParseFlow(src)
	fromObject := ValidateFlowObject(flow, Options{})

	if fromText.Valid != fromObject.Valid {
		t.Errorf("verdicts differ: text=%v object=%v", fromText.Valid, fromObject.Valid)
	}
	textCodes, objectCodes := codesOf(fromText.Errors), codesOf(fromObject.Errors)
	if strings.Join(textCodes, ",") != strings.Join(objectCodes, ",") {
		t.Errorf("codes differ: text=%v object=%v", textCodes, objectCodes)
	}
	if fromObject.Errors[0].Line != 0 {
		t.Error("ValidateFlowObject cannot know positions and must not invent them")
	}
}

func TestValidateFlowObjectHandlesNil(t *testing.T) {
	result := ValidateFlowObject(nil, Options{})
	if result.Valid {
		t.Error("a nil document must not validate")
	}
	if !hasCode(result.Errors, codeInvalidRoot) {
		t.Errorf("want invalid_flow_root, got: %s", formatIssues(result.Errors))
	}
}

func TestDeterministicFindingOrder(t *testing.T) {
	// Go map iteration is randomised; every validator walks steps sorted so a
	// consumer can diff or cache a verdict. Ten runs must be byte-identical.
	src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      default: ghost1
  b:
    executor: function://n
    next:
      default: ghost2
  c:
    executor: notauri
`
	first, err := json.Marshal(ValidateFlow(src, Options{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for range 10 {
		again, err := json.Marshal(ValidateFlow(src, Options{}))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("findings are not deterministic:\n%s\n%s", first, again)
		}
	}
}

func TestSpecAccessors(t *testing.T) {
	if SpecVersion() == "" {
		t.Error("SpecVersion must be non-empty: a verdict has to name the schema that produced it")
	}
	if InputSchemaVersion() <= 0 {
		t.Errorf("InputSchemaVersion = %d, want a positive value", InputSchemaVersion())
	}
	if len(ExecutorSchemes()) == 0 || len(TemplateFunctionNames()) == 0 {
		t.Error("the embedded spec produced empty enum surfaces — the embed is not loading")
	}
	// The accessors must hand out copies: a caller mutating the returned slice
	// must not corrupt the package's protocol data.
	schemes := ExecutorSchemes()
	schemes[0] = "corrupted"
	if ExecutorSchemes()[0] == "corrupted" {
		t.Error("ExecutorSchemes leaked its backing array")
	}
}

// quoteYAML renders a string as a YAML double-quoted scalar so a template's
// braces and backslashes survive into the parsed document unchanged.
func quoteYAML(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
