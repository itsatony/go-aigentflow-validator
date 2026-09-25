package aifvalidate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// User YAML is arbitrary, so every validator treats the parsed document as
// `any` and narrows explicitly. These are the Go ports of the JS
// isRecord/isString/isArray/isNumber guards.

// doc is a decoded YAML mapping. yaml.v3 decodes nested mappings into
// map[string]any (unlike yaml.v2's map[any]any), so this one type covers the
// whole document.
type doc = map[string]any

func asRecord(v any) (doc, bool) {
	m, ok := v.(doc)
	return m, ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// isNonEmptyString reports a present, string-typed, non-empty value — the guard
// nearly every required-field check needs.
func isNonEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && s != ""
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

// asNumber narrows any YAML numeric scalar to float64. yaml.v3 yields int for
// integers and float64 for floats (and int64/uint64 for values outside int
// range), so all of those must be accepted or an integer constraint read as
// `int` would be silently ignored.
func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// asInteger narrows to a whole-numbered scalar.
func asInteger(v any) (int, bool) {
	f, ok := asNumber(v)
	if !ok || f != float64(int(f)) {
		return 0, false
	}
	return int(f), true
}

func asBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

// get reads a key from a possibly-nil mapping.
func get(m doc, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

// getRecord reads a key expected to hold a mapping.
func getRecord(m doc, key string) (doc, bool) {
	return asRecord(get(m, key))
}

// getString reads a key expected to hold a string.
func getString(m doc, key string) (string, bool) {
	return asString(get(m, key))
}

// getSlice reads a key expected to hold a sequence.
func getSlice(m doc, key string) ([]any, bool) {
	return asSlice(get(m, key))
}

// has reports whether a key is present at all (distinct from present-and-empty:
// several rules are opt-in on presence).
func has(m doc, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// present reports whether a key holds a non-nil value. YAML `key:` with no value
// decodes to nil, which every "is this block declared?" check must treat as
// absent — otherwise an empty `loop:` would suppress the executor requirement.
func present(m doc, key string) bool {
	return get(m, key) != nil
}

// sortedKeys returns a mapping's keys in lexical order.
//
// DIVERGENCE from the JS implementation, deliberate: Go map iteration order is
// randomised, so every validator walks steps in SORTED order. The JS version
// walks YAML insertion order. Finding order therefore differs between the two;
// the parity contract is the SET of codes, not their sequence, so this is
// compatible — and determinism is worth more than insertion order to a consumer
// that diffs or caches verdicts.
func sortedKeys(m doc) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// stepNames is sortedKeys under the name the JS validator uses, for the
// "Available steps: …" context strings.
func stepNames(steps doc) []string { return sortedKeys(steps) }

// sortedSet renders an enum set as a sorted list, for the "Use one of: …"
// suggestion strings. Sorted rather than spec order because Go set iteration is
// randomised and a suggestion that reshuffles between runs looks like a bug.
func sortedSet(s set) []string {
	out := make([]string, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// stepsOf returns the flow's steps mapping, or nil when it is absent or the
// wrong shape. Every validator past basicStructure bails on nil rather than
// re-reporting a shape error basicStructure already raised.
func stepsOf(flow doc) doc {
	m, ok := getRecord(flow, keySteps)
	if !ok {
		return nil
	}
	return m
}

// isValidGoDuration reports whether s parses as a Go time.ParseDuration string.
//
// DIVERGENCE from the JS implementation, in Go's favour: the JS port hand-rolls
// a duration scanner because JS has no equivalent. Here the reference behaviour
// IS time.ParseDuration, so the check is exact rather than approximated.
// time.ParseDuration accepts "0" without a unit, matching the JS special case.
func isValidGoDuration(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.ParseDuration(s)
	return err == nil
}

// scalarText renders a YAML scalar the way yaml.v3 decodes it into a Go
// `string` field: a string as itself, a number or boolean as its text. The
// reference's typed fields (max_duration, a mock delay, tool_discovery, a child
// flow's flow_id) accept any scalar this way, so `max_duration: 90` arrives as
// "90" and is judged as that text. Mappings, sequences and null are not scalars.
//
// A float's original spelling is lost at parse time (`1.50` becomes "1.5"),
// which can change a message, never a verdict: no float text is a Go duration.
func scalarText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case uint64:
		return strconv.FormatUint(x, 10), true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	default:
		return "", false
	}
}

// trimmed returns a string value with surrounding whitespace removed, passing
// through any non-string unchanged. Used where the reference treats a
// whitespace-only value as absent (a rubric of "  " is not a rubric).
func trimmed(v any) any {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return v
}

// joinNames renders a name list for a Context/Suggestion string.
func joinNames(names []string) string { return strings.Join(names, ", ") }

// stepField builds the dotted field path for a step-scoped finding.
func stepField(stepID string, parts ...string) string {
	var b strings.Builder
	b.WriteString(keySteps)
	b.WriteString(".")
	b.WriteString(stepID)
	for _, p := range parts {
		b.WriteString(".")
		b.WriteString(p)
	}
	return b.String()
}

// indexed renders an array element path segment, e.g. "conditions[2]".
func indexed(name string, i int) string { return fmt.Sprintf("%s[%d]", name, i) }
