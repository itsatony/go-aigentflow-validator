package aifvalidate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// v0.23.0 (AIgentFlow #322): the unknown_yaml_key finding offers the key the author
// most likely meant — the reference's closeKeys: the alias table first, then the
// accepted keys within an edit distance of max(2, len/3), nearest first, ties in
// sorted order. testdata/conformance/unknown-key-hints-cases.json pins the exact
// messages; the JS port carries a byte-identical copy and asserts the same table.
// Expected messages are typed LITERALLY, never built from the constants under test.

const unknownKeyBase = " AIgentFlow refuses to save a flow with a key its grammar does not declare, " +
	"because the key would be silently dropped."

type keyHintManifestEntry struct {
	File    string `json:"file"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type fieldMessage struct{ field, message string }

func sortFieldMessages(list []fieldMessage) {
	sort.Slice(list, func(i, j int) bool { return list[i].field < list[j].field })
}

// Exactly these findings: one more (or one fewer) unknown key is a failure too.
func TestUnknownKeyHintsManifest(t *testing.T) {
	blob, err := os.ReadFile(filepath.Join(fixtureDir, "unknown-key-hints-cases.json"))
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	var entries []keyHintManifestEntry
	if err := json.Unmarshal(blob, &entries); err != nil {
		t.Fatalf("decode the manifest: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the manifest is empty")
	}
	want := map[string][]fieldMessage{}
	for _, e := range entries {
		if e.Code != codeUnknownYAMLKey {
			t.Fatalf("manifest entry %s carries code %q", e.Field, e.Code)
		}
		want[e.File] = append(want[e.File], fieldMessage{e.Field, e.Message})
	}
	for file, expected := range want {
		t.Run(file, func(t *testing.T) {
			result := ValidateFlow(readFixture(t, file), Options{})
			var got []fieldMessage
			for _, e := range result.Errors {
				if e.Code == codeUnknownYAMLKey {
					got = append(got, fieldMessage{e.Field, e.Message})
				}
			}
			sortFieldMessages(got)
			sortFieldMessages(expected)
			if !reflect.DeepEqual(got, expected) {
				t.Errorf("findings differ\n got: %q\nwant: %q", got, expected)
			}
		})
	}
}

func stepFlow(extra string) string {
	return strings.Join([]string{
		"aigentflow_version: '2.0.0'", "name: t", "version: '1.0.0'", "start: a", "steps:",
		"  a:", "    executor: mock://api/a", extra, "    next:", "      default: 'null'", "",
	}, "\n")
}

func unknownKeyIssue(t *testing.T, yaml, field string) Issue {
	t.Helper()
	for _, e := range ValidateFlow(yaml, Options{}).Errors {
		if e.Code == codeUnknownYAMLKey && e.Field == field {
			return e
		}
	}
	t.Fatalf("no %s at %s", codeUnknownYAMLKey, field)
	return Issue{}
}

func TestDidYouMeanAliasInputSchemaField(t *testing.T) {
	yaml := strings.Join([]string{
		"aigentflow_version: '2.0.0'", "name: t", "version: '1.0.0'",
		"input_schema:", "  version: 1", "  fields:", "    - name: topic", "      type: string", "      key: topic",
		"start: a", "steps:", "  a:", "    executor: mock://api/a", "    next:", "      default: 'null'", "",
	}, "\n")
	issue := unknownKeyIssue(t, yaml, "input_schema.fields[0].key")
	if want := "Unknown key 'key' in 'input_schema.fields[0]'." + unknownKeyBase + " Did you mean `name`?"; issue.Message != want {
		t.Errorf("message\n got: %s\nwant: %s", issue.Message, want)
	}
	// The existing suggestion is kept.
	if !strings.HasPrefix(issue.Suggestion, "Known keys here: accept, default, description, ") {
		t.Errorf("suggestion: %s", issue.Suggestion)
	}
}

func TestDidYouMeanStepPaths(t *testing.T) {
	cases := []struct{ name, extra, field, want string }{
		{"alias params", "    params: {}", "steps.a.params",
			"Unknown key 'params' in 'steps.a'." + unknownKeyBase + " Did you mean `query`?"},
		{"distance excutor", "    excutor: mock://api/b", "steps.a.excutor",
			"Unknown key 'excutor' in 'steps.a'." + unknownKeyBase + " Did you mean `executor`?"},
		{"no match", "    zzzzzz: 1", "steps.a.zzzzzz",
			"Unknown key 'zzzzzz' in 'steps.a'." + unknownKeyBase},
		{"alias target not accepted, nothing close", "    language: go", "steps.a.language",
			"Unknown key 'language' in 'steps.a'." + unknownKeyBase},
		{"alias with one target not accepted", "    input: {}", "steps.a.input",
			"Unknown key 'input' in 'steps.a'." + unknownKeyBase + " Did you mean `query`?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unknownKeyIssue(t, stepFlow(c.extra), c.field).Message; got != c.want {
				t.Errorf("message\n got: %s\nwant: %s", got, c.want)
			}
		})
	}
}

func TestDidYouMeanNotOfferedForARetiredKey(t *testing.T) {
	yaml := strings.Join([]string{
		"aigentflow_version: '2.0.0'", "name: t", "version: '1.0.0'", "start: fetch", "steps:",
		"  fetch:", "    executor: mock://api/a", "    next:", "      default: 'null'",
		"mock_scenarios:", "  happy:", "    fetch:", "      binary_results: []", "",
	}, "\n")
	msg := unknownKeyIssue(t, yaml, "mock_scenarios.happy.fetch.binary_results").Message
	if !strings.Contains(msg, "It was removed from the grammar") || strings.Contains(msg, "Did you mean") {
		t.Errorf("message: %s", msg)
	}
	// Without the retired rule it WOULD have drawn a hint: the decision is load-bearing.
	assertCloseKeys(t, "binary_results", []string{"binary_assets", "content"}, []string{"binary_assets"})
}

func assertCloseKeys(t *testing.T, guess string, accepted, want []string) {
	t.Helper()
	got := closeKeys(guess, accepted)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("closeKeys(%q, %v) = %v, want %v", guess, accepted, got, want)
	}
}

func TestCloseKeys(t *testing.T) {
	cases := []struct {
		name     string
		guess    string
		accepted []string
		want     []string
	}{
		{"alias targets first, in TABLE order", "input", []string{"input_schema", "name", "query"}, []string{"query", "input_schema"}},
		{"alias with two accepted targets", "title", []string{"label", "name"}, []string{"label", "name"}},
		{"alias target not accepted", "title", []string{"description", "name"}, []string{"name"}},
		{"nearest first, ties in the order given", "nam", []string{"enum", "max", "min", "name"}, []string{"name", "enum", "max"}},
		{"nearest first", "mx_items", []string{"max_items", "min_items"}, []string{"max_items", "min_items"}},
		{"alias first, then distance", "id", []string{"max", "min", "name"}, []string{"name", "min"}},
		{"deduplicated", "nam", []string{"name", "name"}, []string{"name"}},
		{"case-insensitive distance", "EXCUTOR", []string{"executor"}, []string{"executor"}},
		{"case-insensitive alias", "Params", []string{"query"}, []string{"query"}},
		{"U+0130 folds to i", "İD", []string{"max", "min", "name"}, []string{"name", "min"}},
		{"threshold in bytes: 13/3 = 4", "mäx_ïtëmß", []string{"max_items"}, []string{"max_items"}},
		{"floor 2, inclusive", "ab", []string{"abcd"}, []string{"abcd"}},
		{"floor 2, exceeded", "ab", []string{"abcde"}, nil},
		{"len/3, inclusive", "abcdefghi", []string{"abcdefxyz"}, []string{"abcdefxyz"}},
		{"len/3, exceeded", "abcdefghi", []string{"abcdewxyz"}, nil},
		{"no match", "zzzzzz", []string{"query"}, nil},
		{"a prototype member name is not an alias", "constructor", []string{"name"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assertCloseKeys(t, c.guess, c.accepted, c.want) })
	}
	if d := editDistance("mäx_ïtëmß", "max_items"); d != 4 {
		t.Errorf("editDistance counts runes: got %d, want 4", d)
	}
}

func TestDidYouMeanFormat(t *testing.T) {
	if got, want := didYouMean("input", []string{"input_schema", "query"}), " Did you mean `query` or `input_schema`?"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := didYouMean("zzzzzz", []string{"query"}); got != "" {
		t.Errorf("got %q, want none", got)
	}
}

func TestKeyAliasesAreTheReferenceTable(t *testing.T) {
	want := map[string][]string{
		"key": {"name"}, "id": {"name"}, "title": {"label", "name"}, "desc": {"description"},
		"url": {"executor"}, "uri": {"executor"}, "type": {"executor"}, "params": {"query"},
		"parameters": {"query"}, "input": {"query", "input_schema"}, "inputs": {"query", "input_schema"},
		"script": {"query"}, "code": {"expression"}, "expr": {"expression"}, "language": {"lang"},
		"retries": {"max_retries"}, "retry": {"error_strategy"}, "timeout": {"max_duration"},
		"next_step": {"next"}, "goto": {"next"},
	}
	if !reflect.DeepEqual(spec.KeyAliases, want) {
		t.Errorf("keyAliases = %v\nwant %v", spec.KeyAliases, want)
	}
	for alias := range spec.KeyAliases {
		if alias != strings.ToLower(alias) {
			t.Errorf("alias %q is not lower-case, so the lower-cased lookup cannot reach it", alias)
		}
	}
}

func TestKeyAliasesIsADeepCopy(t *testing.T) {
	got := KeyAliases()
	if !reflect.DeepEqual(got, spec.KeyAliases) {
		t.Fatalf("KeyAliases() = %v, want the spec table %v", got, spec.KeyAliases)
	}
	got["key"][0] = "mutated"
	got["title"] = append(got["title"], "extra")
	got["new"] = []string{"x"}
	delete(got, "id")
	again := KeyAliases()
	if again["key"][0] != "name" || len(again["title"]) != 2 || again["new"] != nil || again["id"] == nil {
		t.Errorf("a caller's change reached the table: %v", again)
	}
	if closeKeys("key", []string{"name"})[0] != "name" {
		t.Error("the validator's own lookup changed")
	}
}
