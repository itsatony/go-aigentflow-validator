package aifvalidate

import (
	"fmt"
	"regexp"
	"strings"
)

// validateExpressionFunctions checks `expression_functions` in three layers,
// all mirrored from the reference (parser.go):
//
//  1. validateExpressionFunctions: structural — each entry is a mapping with
//     EXACTLY one key, `package` or `function`, with a non-empty value.
//  2. validateExpressionFunctionCatalog (AIF v2.642.0, DC-FORGE-72): `package:`
//     is refused outright, and a `function:` outside the fixed catalog is
//     refused. The catalog is compiled into AIgentFlow; nothing is loaded at
//     run time, ever.
//  3. validateExpressionFunctionUsage: a template action calling an `fn_` name
//     must name a catalog entry AND be declared by this flow.
//
// Layers 2 and 3 are refused at the SAVE door upstream; a flow stored before
// the rules existed still RUNS. This validator answers "would this save", so
// both are errors. See PARITY.md.
func validateExpressionFunctions(flow doc, iss *issues) {
	validateExpressionFunctionDeclarations(flow, iss)
	// The usage scan runs even when the block is absent: a flow that calls an
	// fn_ function and declares nothing is exactly the case the rule is for.
	validateExpressionFunctionUsage(flow, declaredExpressionFunctions(flow), iss)
}

func validateExpressionFunctionDeclarations(flow doc, iss *issues) {
	raw := get(flow, keyExpressionFunctions)
	if raw == nil {
		return
	}
	list, ok := asSlice(raw)
	if !ok {
		iss.error(Issue{
			Field: keyExpressionFunctions, Code: codeInvalidType,
			Message: "expression_functions must be a list",
		})
		return
	}

	for i, rawEntry := range list {
		field := indexed(keyExpressionFunctions, i)
		entry, isMap := asRecord(rawEntry)
		if !isMap {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "Each expression_functions entry must be a mapping",
			})
			continue
		}
		if len(entry) != 1 {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "Each expression_functions entry must have exactly one key (package OR function)",
			})
			continue
		}
		hasPackage, hasFunction := has(entry, keyPackage), has(entry, keyFunction)
		if !hasPackage && !hasFunction {
			iss.error(Issue{
				Field: field, Code: codeInvalidExprFunction,
				Message: "expression_functions entry must use key 'package' or 'function'",
			})
			continue
		}
		key := keyFunction
		if hasPackage {
			key = keyPackage
		}
		value, isStr := getString(entry, key)
		if !isStr || value == "" {
			iss.error(Issue{
				Field: field + "." + key, Code: codeInvalidExprFunction,
				Message: "expression_functions '" + key + "' must have a non-empty value",
			})
			continue
		}

		// `package:` is refused, not ignored: nothing is loaded at run time, so
		// a flow could never have gotten functions this way.
		if hasPackage {
			iss.error(Issue{
				Field: field + "." + keyPackage, Code: codeExprFnPackageUnsupported,
				Message: "`package:` is not supported: expression functions are compiled into AIgentFlow and " +
					"nothing is loaded at run time. Use `function:` naming a built-in catalog entry instead.",
				Context:    "Available: " + expressionCatalogList(),
				Suggestion: `Replace the entry with { function: "fn_<name>" } from the catalog.`,
			})
			continue
		}
		if !expressionFunctionCatalog.has(value) {
			iss.error(Issue{
				Field: field + "." + keyFunction, Code: codeExprFnUnknown,
				Message: fmt.Sprintf("expression function '%s' is not in the built-in catalog", value),
				Context: "Available: " + expressionCatalogList(),
			})
		}
	}
}

func expressionCatalogList() string { return joinNames(sortedSet(expressionFunctionCatalog)) }

// declaredExpressionFunctions collects the catalog names this flow declares.
func declaredExpressionFunctions(flow doc) set {
	declared := set{}
	list, _ := getSlice(flow, keyExpressionFunctions)
	for _, raw := range list {
		entry, ok := asRecord(raw)
		if !ok {
			continue
		}
		if name, ok := getString(entry, keyFunction); ok && name != "" {
			declared[name] = struct{}{}
		}
	}
	return declared
}

var (
	// templateActionRe spans one template action. (?s) so a MULTI-LINE action,
	// which YAML block scalars produce, is matched whole; non-greedy so two
	// actions on one line stay two.
	templateActionRe = regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	// templateStringLiteralRe spans a double-quoted or backquoted string inside
	// an action. Its contents are DATA (a key name), never an identifier.
	templateStringLiteralRe = regexp.MustCompile("\"[^\"]*\"|`[^`]*`")
)

// expressionFunctionCallRe finds a catalog-namespaced identifier being CALLED.
//
// ⚠ The leading group is not decoration: `\bfn_\w+` also matches the FIELD
// reference {{ .data.fn_total }}, because `\b` matches between the dot and the
// f, and a flow with such a field would be refused for a function it never
// called. RE2 has no lookbehind, so — exactly as the reference does — the
// preceding character is captured and discarded: a call is preceded by `{{`,
// `(`, `|` or whitespace, never by a dot or a word character.
var expressionFunctionCallRe *regexp.Regexp

func init() {
	expressionFunctionCallRe = regexp.MustCompile(
		`(^|[^.\w])(` + regexp.QuoteMeta(spec.ExpressionFunctions.NamePrefix) + `[A-Za-z0-9_]+)`)
}

// validateExpressionFunctionUsage refuses a template that CALLS a catalog
// function this flow did not declare, or an fn_ name that is not in the catalog
// at all. It walks every string leaf of the document (divergence #10: the
// reference re-serialises its typed Flow, so it sees only declared fields; the
// two agree on every flow the reference would accept).
func validateExpressionFunctionUsage(flow doc, declared set, iss *issues) {
	// One finding per distinct name: thirty uses of one undeclared function are
	// one problem.
	reported := set{}
	walkDocStrings("", flow, func(path, text string) {
		if !strings.Contains(text, templateOpenDelim) {
			return
		}
		for _, rawAction := range templateActionRe.FindAllString(text, -1) {
			action := templateStringLiteralRe.ReplaceAllString(rawAction, "")
			for _, match := range expressionFunctionCallRe.FindAllStringSubmatch(action, -1) {
				called := match[2]
				if reported.has(called) {
					continue
				}
				if !expressionFunctionCatalog.has(called) {
					reported[called] = struct{}{}
					iss.error(Issue{
						Field: path, Code: codeExprFnUnknownUse,
						Message: fmt.Sprintf("A template calls '%s', which is not a built-in expression function", called),
						Context: "Available: " + expressionCatalogList(),
					})
					continue
				}
				if !declared.has(called) {
					reported[called] = struct{}{}
					iss.error(Issue{
						Field: path, Code: codeExprFnUndeclaredUse,
						Message:    fmt.Sprintf("A template calls '%s' but the flow does not declare it", called),
						Suggestion: fmt.Sprintf(`Add it: expression_functions: [{ function: "%s" }]`, called),
					})
				}
			}
		}
	})
}

// walkDocStrings visits every string leaf of a decoded document with its
// dotted path, in sorted key order (determinism, divergence #4).
func walkDocStrings(path string, value any, visit func(path, s string)) {
	switch v := value.(type) {
	case string:
		visit(path, v)
	case []any:
		for i, item := range v {
			walkDocStrings(fmt.Sprintf("%s[%d]", path, i), item, visit)
		}
	case doc:
		for _, key := range sortedKeys(v) {
			child := key
			if path != "" {
				child = path + "." + key
			}
			walkDocStrings(child, v[key], visit)
		}
	}
}
