package aifvalidate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Cross-implementation parity checks against aigentflow-flow-validator-js.
//
// All of these SKIP cleanly when the sibling JS checkout is absent, so CI on a
// bare clone passes. They fail loudly when it IS present and the two disagree —
// the point is to catch a one-sided change, which is the single most likely way
// these two implementations silently part.
//
// Point envJSRepo at the JS checkout to enable them:
//
//	AIF_JS_VALIDATOR_REPO=~/code/aigentflow-flow-validator-js go test -run Parity ./...
const (
	envJSRepo      = "AIF_JS_VALIDATOR_REPO"
	jsSpecRelPath  = "src/spec/aigentflow-spec.json"
	jsFixtureRel   = "test/conformance/fixtures"
	jsCLIRelPath   = "dist/cli.js"
	jsBuildCommand = "npm run build"
)

// TestParitySpecIsByteIdenticalUpstream is the guard that makes the embedded
// enum surface a shared SSOT rather than a copy that drifts. An upstream enum
// bump must be mirrored here as a byte-for-byte file copy, and this test is what
// notices when it was not.
func TestParitySpecIsByteIdenticalUpstream(t *testing.T) {
	repo := jsRepoOrSkip(t)

	ours, err := os.ReadFile(filepath.Join("spec", "aigentflow-spec.json"))
	if err != nil {
		t.Fatalf("read embedded spec: %v", err)
	}
	theirs, err := os.ReadFile(filepath.Join(repo, jsSpecRelPath))
	if err != nil {
		t.Skipf("upstream spec not readable at %s: %v", filepath.Join(repo, jsSpecRelPath), err)
	}
	if string(ours) != string(theirs) {
		t.Errorf("spec/aigentflow-spec.json has drifted from %s.\n"+
			"Fix by copying upstream over ours (never by hand-editing either):\n"+
			"  cp %s spec/aigentflow-spec.json\n"+
			"then re-run the conformance suite — a rule may need porting for the new version.",
			jsSpecRelPath, filepath.Join(repo, jsSpecRelPath))
	}
}

// TestParityFixtureSetIsIdentical guards the other half of the shared contract:
// the conformance corpus. A fixture added upstream and not here means an
// upstream rule is untested on this side.
func TestParityFixtureSetIsIdentical(t *testing.T) {
	repo := jsRepoOrSkip(t)

	ours := yamlNamesIn(t, fixtureDir)
	theirs := yamlNamesIn(t, filepath.Join(repo, jsFixtureRel))
	if len(theirs) == 0 {
		t.Skipf("no upstream fixtures found at %s", filepath.Join(repo, jsFixtureRel))
	}

	for _, name := range theirs {
		if !contains(ours, name) {
			t.Errorf("fixture %s exists upstream but not here — copy it into %s and add a conformance case",
				name, fixtureDir)
		}
	}
	for _, name := range ours {
		if !contains(theirs, name) {
			t.Errorf("fixture %s exists here but not upstream — add it upstream so both sides assert it", name)
		}
	}

	// Byte-equality too: a fixture edited on one side only would let the two
	// suites be self-consistently green about different documents.
	for _, name := range ours {
		if !contains(theirs, name) {
			continue
		}
		a, errA := os.ReadFile(filepath.Join(fixtureDir, name))
		b, errB := os.ReadFile(filepath.Join(repo, jsFixtureRel, name))
		if errA != nil || errB != nil {
			continue
		}
		if string(a) != string(b) {
			t.Errorf("fixture %s differs between the two repos; they must be byte-identical", name)
		}
	}
}

// jsVerdict is the subset of the JS CLI's --json output this comparison reads.
type jsVerdict struct {
	Valid    bool                    `json:"valid"`
	Errors   []struct{ Code string } `json:"errors"`
	Warnings []struct{ Code string } `json:"warnings"`
}

// TestParityVerdictsMatchTheJSImplementation is the strongest available parity
// net: it runs the JS CLI over every shared fixture and diffs the CODE SETS
// against this implementation's.
//
// Errors must match exactly. Warnings are compared as "every JS warning appears
// here", not equality, because this implementation may legitimately produce more
// (see PARITY.md — the Go template parser is exact where the JS one approximates,
// and RE2 is authoritative for `pattern`).
func TestParityVerdictsMatchTheJSImplementation(t *testing.T) {
	repo := jsRepoOrSkip(t)
	cli := filepath.Join(repo, jsCLIRelPath)
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("JS CLI not built at %s — run `%s` in %s first", cli, jsBuildCommand, repo)
	}

	// A stale build is the failure mode that would make this comparison lie: the
	// committed dist/ can trail src/ by one or more spec versions. Detect that
	// explicitly rather than reporting the resulting differences as parity drift.
	if jsSpec := jsCLISpecVersion(t, cli); jsSpec != SpecVersion() {
		t.Skipf("JS CLI tracks spec %s but this library tracks %s — the JS build is stale.\n"+
			"Run `%s` in %s and re-run.", jsSpec, SpecVersion(), jsBuildCommand, repo)
	}

	for _, name := range yamlNamesIn(t, fixtureDir) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(fixtureDir, name)
			source := readFixture(t, name)

			out, err := exec.Command("node", cli, "--json", path).CombinedOutput()
			// Exit code 1 means "validation failed", which is a normal outcome here;
			// only a usage error (2) or a missing binary is fatal.
			if err != nil && !strings.HasPrefix(strings.TrimSpace(string(out)), "{") {
				t.Fatalf("JS CLI failed: %v\n%s", err, out)
			}
			var theirs jsVerdict
			if uerr := json.Unmarshal(out, &theirs); uerr != nil {
				t.Fatalf("decode JS output: %v\n%s", uerr, out)
			}

			ours := ValidateFlow(source, Options{})

			if ours.Valid != theirs.Valid {
				t.Errorf("verdict differs: go=%v js=%v\ngo errors: %s",
					ours.Valid, theirs.Valid, formatIssues(ours.Errors))
			}
			goErrs, jsErrs := uniqueSorted(codesOf(ours.Errors)), uniqueSorted(jsCodes(theirs.Errors))
			if strings.Join(goErrs, ",") != strings.Join(jsErrs, ",") {
				t.Errorf("error codes differ:\n  go: %v\n  js: %v", goErrs, jsErrs)
			}
			for _, code := range uniqueSorted(jsCodes(theirs.Warnings)) {
				if !hasCode(ours.Warnings, code) {
					t.Errorf("JS warns %q and this implementation does not", code)
				}
			}
		})
	}
}

// jsCLITrackedSpecRe extracts the schema version the built JS CLI reports, e.g.
// "aigentflow-flow-validator (tracks AIgentFlow flow schema v2.485.0)".
var jsCLITrackedSpecRe = regexp.MustCompile(`schema v?(\d+\.\d+\.\d+)`)

// jsCLISpecVersion reads the spec version the BUILT JS CLI tracks.
//
// It deliberately reads the built artifact's own claim and never falls back to
// the repo's src/ spec file: the whole reason this exists is that a committed
// dist/ can trail src/, and a fallback to src would report the version a fresh
// build WOULD track, defeating the staleness check and reporting the resulting
// differences as parity drift instead.
func jsCLISpecVersion(t *testing.T, cli string) string {
	t.Helper()
	out, err := exec.Command("node", cli, "--version").CombinedOutput()
	if err != nil {
		t.Skipf("could not read the JS CLI version: %v\n%s", err, out)
	}
	match := jsCLITrackedSpecRe.FindStringSubmatch(string(out))
	if match == nil {
		t.Skipf("could not parse a tracked schema version from the JS CLI: %q", strings.TrimSpace(string(out)))
	}
	return match[1]
}

func jsRepoOrSkip(t *testing.T) string {
	t.Helper()
	repo := os.Getenv(envJSRepo)
	if repo == "" {
		t.Skipf("set %s to the aigentflow-flow-validator-js checkout to run the parity checks", envJSRepo)
	}
	repo = expandHome(repo)
	if _, err := os.Stat(repo); err != nil {
		t.Skipf("%s=%s is not readable: %v", envJSRepo, repo, err)
	}
	return repo
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func yamlNamesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func jsCodes(list []struct{ Code string }) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.Code)
	}
	return out
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
