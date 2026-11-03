package aifvalidate

import (
	"sort"
	"strings"
	"testing"
)

// v0.23.0: the spec is the JS port's byte for byte, so the flow_data_key_reserved,
// unknown_function_operation and template_default_piped texts are read from it.
// These tests pin that the MESSAGES did not move: literal text, and the
// reference's %q for a key or step id holding a quote or a non-printable.

func TestSpecCodesAreTheContractCodes(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{spec.FlowDataReservedKeys.Code, codeFlowDataKeyReserved},
		{spec.FunctionOperations.Code, codeUnknownFunctionOperation},
		{spec.TemplateDefaultPiped.Code, codeTemplateDefaultPiped},
	} {
		if c.spec != c.want {
			t.Errorf("spec code %q, contract code %q", c.spec, c.want)
		}
	}
	if spec.FunctionOperations.Scheme != "function" || spec.TemplateDefaultPiped.Function != "default" {
		t.Errorf("scheme %q, function %q", spec.FunctionOperations.Scheme, spec.TemplateDefaultPiped.Function)
	}
}

func dataKeyFlow(quotedKey string) string {
	return strings.Join([]string{
		"aigentflow_version: '2.0.0'", "name: t", "version: '1.0.0'", "data:", "  " + quotedKey + ": 1",
		"start: a", "steps:", "  a:", "    executor: mock://api/a", "    next:", "      default: 'null'", "",
	}, "\n")
}

func TestFlowDataKeyReservedQuotesAsPercentQ(t *testing.T) {
	cases := []struct{ yamlKey, quoted string }{
		{"_memory", `"_memory"`},
		{`"_a\"b"`, `"_a\"b"`},
		{`"_é\u00a0x"`, `"_é\u00a0x"`},
		{`"_\x07\x7f"`, `"_\a\x7f"`},
	}
	for _, c := range cases {
		issue, ok := findByCode(ValidateFlow(dataKeyFlow(c.yamlKey), Options{}).Errors, codeFlowDataKeyReserved)
		if !ok {
			t.Fatalf("%s: no finding", c.yamlKey)
		}
		if want := "data: key " + c.quoted + " is reserved: "; !strings.HasPrefix(issue.Message, want) {
			t.Errorf("%s:\n got: %s\nwant prefix: %s", c.yamlKey, issue.Message, want)
		}
		if !strings.HasSuffix(issue.Message, ". It would write into the engine's own state, so it is not seeded.") {
			t.Errorf("%s: message tail moved: %s", c.yamlKey, issue.Message)
		}
	}
}

func functionFlow(stepID, executor string) string {
	return strings.Join([]string{
		"aigentflow_version: '2.0.0'", "name: t", "version: '1.0.0'", "start: " + stepID, "steps:",
		"  " + stepID + ":", "    executor: " + executor, "    next:", "      default: 'null'", "",
	}, "\n")
}

func TestUnknownFunctionOperationText(t *testing.T) {
	r := ValidateFlow(functionFlow(`'a"é'`, "function://nope/nothing"), Options{})
	issue, ok := findByCode(r.Errors, codeUnknownFunctionOperation)
	if !ok {
		t.Fatalf("no finding: %s", formatIssues(r.Errors))
	}
	list := strings.Join(sortedStrings(spec.FunctionOperations.Operations), ", ")
	if want := `step "a\"é" uses function://nope/nothing, which is not a built-in function. Available: ` + list; issue.Message != want {
		t.Errorf("message\n got: %s\nwant: %s", issue.Message, want)
	}
	// The suggestion is the spec's (it changed in v0.23.0 with the shared spec).
	if want := "Use one of the built-in function operations: " + list +
		". For custom logic use a script://, http:// or ai:// executor instead."; issue.Suggestion != want {
		t.Errorf("suggestion\n got: %s\nwant: %s", issue.Suggestion, want)
	}
}

// The reference keeps the scheme as written (ParseExecutorURLString); only the
// exact `function` scheme is a function:// step. Before v0.23.0 this port
// lower-cased it and refused FUNCTION://nope/nothing, which the reference saves.
func TestUnknownFunctionOperationSchemeIsCaseSensitive(t *testing.T) {
	r := ValidateFlow(functionFlow("a", "FUNCTION://nope/nothing"), Options{})
	if hasCode(r.Errors, codeUnknownFunctionOperation) {
		t.Errorf("FUNCTION:// judged as function://: %s", formatIssues(r.Errors))
	}
}

func TestTemplateDefaultPipedText(t *testing.T) {
	got := templateDefaultPipedMessage(`.x | default "w"`)
	want := "`default` takes the value FIRST, and a pipe passes the piped value LAST: `.x | default \"w\"` is `default <fallback> <value>`, so the fallback is always the result"
	if got != want {
		t.Errorf("message\n got: %s\nwant: %s", got, want)
	}
	if want := "write the value first and the fallback second, as a call: `{{ default .x \"fallback\" }}` (the pipe form of other template libraries does not work here)"; spec.TemplateDefaultPiped.Suggestion != want {
		t.Errorf("suggestion %q", spec.TemplateDefaultPiped.Suggestion)
	}
	if strings.Count(spec.TemplateDefaultPiped.MessageFormat, "%s") != 1 {
		t.Errorf("messageFormat must take exactly one %%s: %q", spec.TemplateDefaultPiped.MessageFormat)
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
