package aifvalidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An AIgentFlow checkout's example flows are EU-clean by construction: the demo
// series pins every call it makes. Point AIF_CORPUS_DIR at a checkout root to
// run compliance_residency_unenforced over its example_flows and
// skills/*/examples; any warning there is a defect in the rule or in the flow.
// Skips when unset.
func TestResidencyCorpusIsSilent(t *testing.T) {
	root := os.Getenv("AIF_CORPUS_DIR")
	if root == "" {
		t.Skip("AIF_CORPUS_DIR not set")
	}
	files, _ := filepath.Glob(filepath.Join(root, "example_flows", "*.yaml"))
	more, _ := filepath.Glob(filepath.Join(root, "skills", "*", "examples", "*.yaml"))
	files = append(files, more...)
	if len(files) == 0 {
		t.Fatal("vacuity guard: no flows found under " + root)
	}
	demos, declaring := 0, 0
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(filepath.Base(file), ".demo.") {
			demos++
		}
		if strings.Contains(string(raw), "residency") {
			declaring++
		}
		for _, w := range ValidateFlow(string(raw), Options{}).Warnings {
			if w.Code == codeComplianceResidencyUnenforced {
				t.Errorf("%s: %s %s", filepath.Base(file), w.Field, w.Message)
			}
		}
	}
	t.Logf("%d flows, %d demos, %d declaring residency", len(files), demos, declaring)
}
