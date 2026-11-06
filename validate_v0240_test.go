package aifvalidate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// v0.24.0 (AIgentFlow v2.878.0): human_answer_field. A human step's answer is
// committed as `response` (with `comments` and `timed_out`) and nothing else; a
// read under any other key renders "<no value>". The shared manifest
// testdata/conformance/human-answer-field-cases.json pins every finding's exact
// field, step, message and suggestion, as the reference produced them, and the
// JS port asserts the same table (test/v0340-human-answer-field.test.ts).

type humanAnswerFinding struct {
	Field      string `json:"field"`
	StepID     string `json:"stepId"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type humanAnswerManifest struct {
	FlowTemplate string            `json:"flowTemplate"`
	Sites        []string          `json:"sites"`
	Defaults     map[string]string `json:"defaults"`
	Cases        []json.RawMessage `json:"cases"`
}

func loadHumanAnswerManifest(t *testing.T) humanAnswerManifest {
	t.Helper()
	blob, err := os.ReadFile(filepath.Join(fixtureDir, "human-answer-field-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m humanAnswerManifest
	if err := json.Unmarshal(blob, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHumanAnswerFieldManifest(t *testing.T) {
	m := loadHumanAnswerManifest(t)
	if len(m.Cases) == 0 {
		t.Fatal("the manifest has no cases")
	}
	positive, negative := 0, 0
	for _, raw := range m.Cases {
		var sites map[string]any
		if err := json.Unmarshal(raw, &sites); err != nil {
			t.Fatal(err)
		}
		var c struct {
			Name     string               `json:"name"`
			Findings []humanAnswerFinding `json:"findings"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		args := make([]any, 0, len(m.Sites))
		for _, site := range m.Sites {
			v, ok := sites[site].(string)
			if !ok {
				v = m.Defaults[site]
			}
			args = append(args, strings.ReplaceAll(v, "'", "''"))
		}
		yaml := fmt.Sprintf(m.FlowTemplate, args...)
		t.Run(c.Name, func(t *testing.T) {
			res := ValidateFlow(yaml, Options{})
			for _, e := range res.Errors {
				if e.Code == codeHumanAnswerField {
					t.Errorf("human_answer_field is a warning, never an error: %+v", e)
				}
			}
			got := []humanAnswerFinding{}
			for _, w := range res.Warnings {
				if w.Code == codeHumanAnswerField {
					got = append(got, humanAnswerFinding{w.Field, w.StepID, w.Message, w.Suggestion})
				}
			}
			want := c.Findings
			if want == nil {
				want = []humanAnswerFinding{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("findings differ\n got: %+v\nwant: %+v", got, want)
			}
		})
		if len(c.Findings) > 0 {
			positive++
		} else {
			negative++
		}
	}
	if positive == 0 || negative == 0 {
		t.Errorf("the manifest must hold both sides: %d positive, %d negative", positive, negative)
	}
}

// The rule never moves a verdict and is never demoted or promoted by position:
// inside a loop body it is still a warning, and StoredBytes leaves it alone.
func TestHumanAnswerFieldIsAWarningEverywhere(t *testing.T) {
	for _, file := range []string{
		"warn-human-answer-field-post-processing.yaml",
		"warn-human-answer-field-next-condition.yaml",
		"warn-human-answer-field-data-read.yaml",
		"warn-human-answer-field-loop-substep.yaml",
	} {
		raw, err := os.ReadFile(filepath.Join(fixtureDir, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, opts := range []Options{{}, {StoredBytes: true}, {StrictRegistries: true}} {
			res := ValidateFlow(string(raw), opts)
			if !res.Valid {
				t.Errorf("%s %+v: not valid: %+v", file, opts, res.Errors)
			}
			n := 0
			for _, w := range res.Warnings {
				if w.Code == codeHumanAnswerField {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%s %+v: %d human_answer_field warnings, want 1", file, opts, n)
			}
		}
	}
}

func TestHumanAnswerFieldSpecBlock(t *testing.T) {
	h := spec.HumanAnswerField
	if h.Code != codeHumanAnswerField {
		t.Errorf("spec code %q", h.Code)
	}
	if !reflect.DeepEqual(h.AnswerKeys, []string{"response", "comments", "timed_out"}) {
		t.Errorf("answer keys %v", h.AnswerKeys)
	}
	if h.ExecutorPrefix != "async://human/" || !reflect.DeepEqual(h.NonParkingModes, []string{"announce"}) {
		t.Errorf("prefix %q, non-parking modes %v", h.ExecutorPrefix, h.NonParkingModes)
	}
	if got := fmt.Sprintf(h.StepMessageFormat, "a\"b", "content", "content"); got !=
		"human step \"a\\\"b\" reads `.step.response.content`, but its answer is `.step.response.response`; `content` renders \"<no value>\"" {
		t.Errorf("step message %q", got)
	}
}

// humanAnswerSteps is the reference's AsyncHumanParkURL over top-level steps.
func TestHumanAnswerSteps(t *testing.T) {
	flow := doc{"steps": doc{
		"a": doc{"executor": "async://human/prompt/text"},
		"b": doc{"executor": "async://human/announce"},
		"c": doc{"executor": "async://human/announce/x"},
		"d": doc{"executor": "async://human/"},
		"e": doc{"executor": "async://human//x"},
		"f": doc{"executor": "async://external/rest/x"},
		"g": doc{"executor": "ASYNC://human/prompt/text"},
		"h": doc{"loop": doc{"steps": []any{doc{"id": "x", "executor": "async://human/prompt/text"}}}},
	}}
	got := humanAnswerSteps(flow)
	if want := map[string]bool{"a": true, "e": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("human steps %v, want %v", got, want)
	}
}
