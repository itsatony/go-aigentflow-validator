package aifvalidate

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document limits. These bound the work an untrusted document can cause, which
// matters because this validator is designed to sit on a publish gate: a caller
// should be able to hand it arbitrary bytes.
const (
	// MaxDocumentBytes is the largest flow document accepted. A flow is a
	// human-authored declaration; anything past this is not one.
	MaxDocumentBytes = 4 << 20 // 4 MiB
	// MaxAliasTokens bounds YAML alias (`*ref`) usage, the "billion laughs"
	// amplification vector. The JS port caps alias EXPANSION at 100 via the yaml
	// package's maxAliasCount; yaml.v3 has its own internal budget but exposes no
	// knob, so this is an explicit pre-parse guard on the same class of input.
	MaxAliasTokens = 100
)

// yamlErrLineRe extracts the line number yaml.v3 prefixes onto its error
// strings ("line 12: …"), so a parse error can carry a position.
var yamlErrLineRe = regexp.MustCompile(`line (\d+):`)

// aliasTokenRe counts YAML alias tokens. It deliberately matches only an alias
// in value position (after ": " or "- " or at the start of a line) so a literal
// "*" inside prose is not counted.
var aliasTokenRe = regexp.MustCompile(`(?m)(^|[:\-]\s)\*[A-Za-z0-9_\-]+`)

// ParseFlow decodes flow YAML into a plain mapping plus structured diagnostics.
// It never panics: malformed input is reported as parseErrors, and a non-empty
// parseErrors means the returned document is nil.
//
// The returned slices are always non-nil.
func ParseFlow(yamlText string) (flow map[string]any, parseErrors, parseWarnings []Issue) {
	parseErrors, parseWarnings = []Issue{}, []Issue{}

	if strings.TrimSpace(yamlText) == "" {
		parseErrors = append(parseErrors, Issue{
			Field: "", Code: codeEmptyDocument, Severity: SeverityError,
			Message: "Flow YAML is empty",
		})
		return nil, parseErrors, parseWarnings
	}
	if len(yamlText) > MaxDocumentBytes {
		parseErrors = append(parseErrors, Issue{
			Field: "", Code: codeYAMLSyntax, Severity: SeverityError,
			Message: fmt.Sprintf("Flow document is %d bytes, over the %d-byte maximum",
				len(yamlText), MaxDocumentBytes),
		})
		return nil, parseErrors, parseWarnings
	}
	if n := len(aliasTokenRe.FindAllString(yamlText, MaxAliasTokens+1)); n > MaxAliasTokens {
		parseErrors = append(parseErrors, Issue{
			Field: "", Code: codeYAMLSyntax, Severity: SeverityError,
			Message: fmt.Sprintf("Flow document uses more than %d YAML aliases", MaxAliasTokens),
		})
		return nil, parseErrors, parseWarnings
	}

	var value any
	if err := yaml.Unmarshal([]byte(yamlText), &value); err != nil {
		parseErrors = append(parseErrors, issuesFromYAMLError(err)...)
		return nil, parseErrors, parseWarnings
	}

	m, ok := normaliseKeys(value).(map[string]any)
	if !ok {
		parseErrors = append(parseErrors, Issue{
			Field: "", Code: codeInvalidRoot, Severity: SeverityError,
			Message: "Flow must be a YAML mapping at the top level",
		})
		return nil, parseErrors, parseWarnings
	}
	return m, parseErrors, parseWarnings
}

// normaliseKeys returns the document with every map[any]any turned into
// map[string]any. yaml.v3 produces map[any]any for a mapping that has a
// non-string key (`steps: {1: …}`), and every validator here narrows to
// map[string]any, so such a mapping read as "not a mapping": a step id of `1`,
// which the reference stores as the string "1", was refused as invalid_type. A
// key is rendered with fmt.Sprint, which equals its source text for every
// ordinary spelling (`1`, `true`).
//
// Copy-on-write: a mapping or list is copied only when something beneath it
// changed, so a caller's document passed to ValidateFlowObject is never mutated.
func normaliseKeys(v any) any {
	out, _ := normaliseKeysChanged(v)
	return out
}

func normaliseKeysChanged(v any) (any, bool) {
	switch x := v.(type) {
	case map[string]any:
		var copied map[string]any
		for k, item := range x {
			n, changed := normaliseKeysChanged(item)
			if !changed {
				continue
			}
			if copied == nil {
				copied = make(map[string]any, len(x))
				for k2, v2 := range x {
					copied[k2] = v2
				}
			}
			copied[k] = n
		}
		if copied == nil {
			return x, false
		}
		return copied, true
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[fmt.Sprint(k)] = normaliseKeys(item)
		}
		return out, true
	case []any:
		var copied []any
		for i, item := range x {
			n, changed := normaliseKeysChanged(item)
			if !changed {
				continue
			}
			if copied == nil {
				copied = append([]any(nil), x...)
			}
			copied[i] = n
		}
		if copied == nil {
			return x, false
		}
		return copied, true
	default:
		return v, false
	}
}

// collectScalarSources records the source text of every plain number and
// boolean scalar, keyed by the field path the validators emit. The reference
// decodes such a scalar into a Go `string` field as that text, so `delay: 0.0`
// is "0.0" there while the decoded value here is the number 0. Aliases are not
// followed: a value reached through one keeps its decoded rendering. A parse
// failure yields no sources (the document is already refused).
func collectScalarSources(yamlText string) scalarSources {
	out := scalarSources{}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil || len(root.Content) == 0 {
		return out
	}
	var walk func(path string, node *yaml.Node)
	walk = func(path string, node *yaml.Node) {
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				if key.Kind != yaml.ScalarNode {
					continue
				}
				child := key.Value
				if path != "" {
					child = path + "." + key.Value
				}
				walk(child, node.Content[i+1])
			}
		case yaml.SequenceNode:
			for i, item := range node.Content {
				walk(fmt.Sprintf("%s[%d]", path, i), item)
			}
		case yaml.ScalarNode:
			switch node.ShortTag() {
			case yamlTagInt, yamlTagFloat, yamlTagBool:
				out[path] = node.Value
			}
		}
	}
	walk("", root.Content[0])
	return out
}

// yaml.v3 short tags of the scalars whose decoded value loses its spelling.
const (
	yamlTagInt   = "!!int"
	yamlTagFloat = "!!float"
	yamlTagBool  = "!!bool"
)

// issuesFromYAMLError converts a yaml.v3 error into one Issue per underlying
// problem, preserving the line number and distinguishing a duplicate key (which
// the JS port surfaces under its own code).
func issuesFromYAMLError(err error) []Issue {
	// yaml.v3 packs several problems (e.g. every duplicate key) into one
	// TypeError; unpacking it turns a single opaque error into one positioned
	// finding per problem.
	messages := []string{err.Error()}
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		messages = typeErr.Errors
	}

	out := make([]Issue, 0, len(messages))
	for _, msg := range messages {
		issue := Issue{
			Field: "", Severity: SeverityError, Code: codeYAMLSyntax,
			Message: strings.TrimPrefix(msg, "yaml: "),
		}
		if strings.Contains(msg, "already defined at line") {
			issue.Code = codeDuplicateKey
		}
		if m := yamlErrLineRe.FindStringSubmatch(msg); m != nil {
			if line, cerr := strconv.Atoi(m[1]); cerr == nil {
				issue.Line = line
			}
		}
		out = append(out, issue)
	}
	return out
}

// positionIndex maps a dotted field path to its 1-based position in the source.
//
// This is an ADDITION over the JS implementation, which can only position parse
// errors. Structural findings name a field path like
// "steps.fetch.next.default"; an editor wants to jump to it. Building the index
// from the yaml.Node tree — in the same path syntax the validators emit — lets
// every finding be resolved to a line without threading node pointers through
// 15 validators.
type positionIndex map[string]position

type position struct{ line, column int }

// buildPositionIndex walks the YAML node tree recording the position of every
// mapping value and sequence element. A parse failure yields an empty index
// (positions are a nicety; their absence must never change a verdict).
func buildPositionIndex(yamlText string) positionIndex {
	idx := positionIndex{}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil {
		return idx
	}
	if len(root.Content) == 0 {
		return idx
	}
	indexNode(idx, "", root.Content[0])
	return idx
}

func indexNode(idx positionIndex, path string, node *yaml.Node) {
	if node == nil {
		return
	}
	if path != "" {
		if _, exists := idx[path]; !exists {
			idx[path] = position{line: node.Line, column: node.Column}
		}
	}
	switch node.Kind {
	case yaml.MappingNode:
		// Content alternates key, value.
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, val := node.Content[i], node.Content[i+1]
			child := key.Value
			if path != "" {
				child = path + "." + key.Value
			}
			// Record the KEY's position for the child path: an author who clicks
			// "steps.fetch.executor" wants the `executor:` line, not the line the
			// value happens to start on (identical for scalars, different for a
			// block mapping or a folded scalar).
			idx[child] = position{line: key.Line, column: key.Column}
			indexNodeChildren(idx, child, val)
		}
	case yaml.SequenceNode:
		for i, item := range node.Content {
			indexNodeChildren(idx, fmt.Sprintf("%s[%d]", path, i), item)
		}
	case yaml.AliasNode:
		// Do not follow aliases: the target is already indexed at its own path,
		// and following them can revisit a node an unbounded number of times.
	}
}

// indexNodeChildren records a child's own position only if not already set by
// its parent mapping (which prefers the key line), then recurses.
func indexNodeChildren(idx positionIndex, path string, node *yaml.Node) {
	if node == nil {
		return
	}
	if _, exists := idx[path]; !exists {
		idx[path] = position{line: node.Line, column: node.Column}
	}
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		indexNode(idx, path, node)
	}
}

// lookup resolves a field path to a position, falling back to the nearest
// existing ancestor. The fallback is the point: a "missing_required_field" names
// a path that by definition does not exist (steps.fetch.executor), and landing
// the author on `fetch:` is right, while landing them on line 1 is useless.
func (idx positionIndex) lookup(path string) (position, bool) {
	if path == "" {
		return position{}, false
	}
	for p := path; p != ""; p = trimLastSegment(p) {
		if pos, ok := idx[p]; ok {
			return pos, true
		}
	}
	return position{}, false
}

// trimLastSegment drops the trailing ".key" or "[i]" from a field path.
func trimLastSegment(path string) string {
	if i := strings.LastIndexAny(path, ".["); i > 0 {
		return path[:i]
	}
	return ""
}

// applyPositions backfills Line/Column onto findings that do not already carry
// them (a parse error keeps the position yaml.v3 reported).
func applyPositions(idx positionIndex, list []Issue) {
	for i := range list {
		if list[i].Line != 0 {
			continue
		}
		if pos, ok := idx.lookup(list[i].Field); ok {
			list[i].Line, list[i].Column = pos.line, pos.column
		}
	}
}
