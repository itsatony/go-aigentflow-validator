package aifvalidate

import (
	"regexp"
	"strings"
	"text/template"
)

// templateOpenDelim marks a Go-template action. A string containing it is
// treated as a template and syntax-checked.
const templateOpenDelim = "{{"

// templateName is the name given to every parsed template; it appears in Go's
// error prefix, which is stripped before the message is surfaced.
const templateName = "flow"

// undefinedFuncRe extracts the function name from Go's parse error
// `function "foo" not defined`. Depending on that wording is the one
// text-coupling in this package, and it is bounded: the name is only used to
// register a stub and retry, so a wording change degrades to "reported as a
// syntax error" rather than to a wrong verdict.
var undefinedFuncRe = regexp.MustCompile(`function "([^"]+)" not defined`)

// templateErrPrefixRe strips Go's `template: flow:12:` position prefix so the
// surfaced message reads as a problem rather than as a stack frame.
var templateErrPrefixRe = regexp.MustCompile(`^template: [^:]*:(\d+:)?\s*`)

// templateIssue is one problem found in a template string.
type templateIssue struct {
	message string
	// isFunctionError distinguishes "this names a function I do not recognise"
	// from "this is not valid template syntax". The two have different codes and
	// different severities, and conflating them would make an unknown function —
	// which is frequently just a newer registry entry — look like broken syntax.
	isFunctionError bool
}

// knownFuncMap is the FuncMap used for parsing: every allow-listed AIgentFlow
// template function registered as a stub. text/template's parser rejects an
// unregistered function name, so registering the allow-list is what lets the
// real Go parser be used for syntax checking without false "not defined" errors
// on legitimate flows.
var knownFuncMap template.FuncMap

func init() {
	knownFuncMap = make(template.FuncMap, len(spec.TemplateFunctions))
	for _, name := range spec.TemplateFunctions {
		knownFuncMap[name] = stubFunc
	}
}

// stubFunc is a no-op with a signature text/template accepts anywhere. It is
// never called: only Parse runs, never Execute.
func stubFunc(...any) any { return nil }

// checkGoTemplateSyntax reports Go text/template problems in src.
//
// DIVERGENCE from aigentflow-flow-validator-js, in this implementation's favour
// and recorded in PARITY.md: the JS port hand-rolls an approximate lexer
// ("NOT a renderer and NOT a full parser… deliberate bias against false
// positives") because JS has no Go template parser. Here the AIgentFlow engine's
// actual parser — text/template — is available, so this check is exact rather
// than approximate: delimiters, quoting, comments, if/range/with/block/define
// nesting, else placement, and empty actions are all judged by the same code the
// engine uses. It is therefore a SUPERSET of the JS checker on
// template_syntax_error; the conformance contract is a code SUBSET assertion,
// which tolerates exactly this direction.
//
// Unknown function names are discovered by registering a stub and re-parsing,
// so a document with an unknown function still has the REST of its syntax
// checked (a single-pass parse would stop at the first unknown name).
func checkGoTemplateSyntax(src string, strictFunctions bool) []templateIssue {
	funcs := make(template.FuncMap, len(knownFuncMap)+4)
	for name, fn := range knownFuncMap {
		funcs[name] = fn
	}

	var unknown []string
	for range maxUnknownFuncLoop {
		_, err := template.New(templateName).Funcs(funcs).Parse(src)
		if err == nil {
			break
		}
		match := undefinedFuncRe.FindStringSubmatch(err.Error())
		if match == nil {
			// A genuine syntax problem. Go's parser reports one at a time, and the
			// first is the one an author should fix, so this is the whole result.
			return []templateIssue{{message: cleanTemplateError(err)}}
		}
		name := match[1]
		if _, already := funcs[name]; already {
			// Defensive: the stub is registered yet Go still reports it undefined.
			// Report as syntax rather than loop forever.
			return []templateIssue{{message: cleanTemplateError(err)}}
		}
		funcs[name] = stubFunc
		unknown = append(unknown, name)
	}

	if !strictFunctions || len(unknown) == 0 {
		// Matching the JS contract: unknown-function findings are produced ONLY
		// under strict registries, because the vendored allow-list can lag the live
		// AIgentFlow function registry and a false positive here is worse than a
		// missed lint.
		return nil
	}
	out := make([]templateIssue, 0, len(unknown))
	for _, name := range unknown {
		out = append(out, templateIssue{
			message:         "function \"" + name + "\" is not a recognised AIgentFlow template function",
			isFunctionError: true,
		})
	}
	return out
}

// cleanTemplateError renders a Go parse error without its position prefix.
func cleanTemplateError(err error) string {
	return strings.TrimSpace(templateErrPrefixRe.ReplaceAllString(err.Error(), ""))
}

// isTemplate reports whether a string contains a template action.
func isTemplate(value string) bool { return strings.Contains(value, templateOpenDelim) }
