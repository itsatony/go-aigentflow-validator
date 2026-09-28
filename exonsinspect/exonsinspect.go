// Package exonsinspect is the go-exons-backed aifvalidate.ExonsInspector: it
// judges an inline .exons document with the same engine, the same engine
// options and the same two calls as the AIgentFlow reference, so the verdicts
// aifvalidate derives from it are the reference's.
//
//	opts := aifvalidate.Options{Exons: exonsinspect.New()}
//	result := aifvalidate.ValidateFlow(src, opts)
//
// It is a separate module so that aifvalidate itself keeps a single dependency
// and builds for GOOS=js GOARCH=wasm. Without it aifvalidate reads only an
// .exons document's frontmatter; see its PARITY.md, divergence #16.
//
// The go-exons version this module requires is the one the tracked AIgentFlow
// release pins. An engine upgrade can change what parses and what renders, so
// the pin moves with the reference, never ahead of it.
package exonsinspect

import (
	"fmt"

	aifvalidate "github.com/itsatony/go-aigentflow-validator"
	exons "github.com/itsatony/go-exons"
)

// Inspector implements aifvalidate.ExonsInspector with go-exons. The zero value
// is ready to use and safe for concurrent use: every call builds its own engine,
// as the reference does at the save door.
type Inspector struct{}

// New returns an Inspector.
func New() Inspector { return Inspector{} }

var _ aifvalidate.ExonsInspector = Inspector{}

// newEngine is the reference's NewExonsEngine: environment access disabled.
// Parse and Validate do not read the environment, but a frontmatter is
// EXECUTED before it is decoded, and an {~exons.env~} there must fail as it
// does in the reference rather than read this process's environment.
func newEngine() (*exons.Engine, error) {
	return exons.New(exons.WithEnvDisabled())
}

// InspectExons reports the reference's two judgements of source — Parse
// (grammar, frontmatter decode and spec validation) and Validate's
// error-severity issues (ExonsIntakeErrors) — together with the spec fields the
// save-door rules read. It never panics on malformed input: go-exons folds
// every failure into an error or an issue.
func (Inspector) InspectExons(source string) aifvalidate.ExonsReport {
	engine, err := newEngine()
	if err != nil {
		// An engine that cannot be built judged nothing; the report says so.
		return aifvalidate.ExonsReport{}
	}
	report := aifvalidate.ExonsReport{Engine: true}

	// ExonsIntakeErrors: Validate's error-severity issues. An error from
	// Validate itself is the engine failing, not the document, and the
	// reference ignores it.
	if res, vErr := engine.Validate(source); vErr == nil && res != nil {
		for _, issue := range res.Errors() {
			report.IntakeErrors = append(report.IntakeErrors, formatIssue(issue))
		}
	}

	tmpl, err := engine.Parse(source)
	if err != nil {
		report.ParseError = err.Error()
		return report
	}
	report.SpecRead = true
	spec := tmpl.Spec()
	if spec == nil {
		return report
	}
	report.HasSpec = true
	if spec.Execution != nil {
		report.Provider = spec.Execution.Provider
	}
	if spec.Tools != nil && spec.Tools.Allow != nil {
		report.ToolsAllowDeclared = true
		report.ToolsAllow = append([]string{}, spec.Tools.Allow...)
	}
	if spec.Requirements != nil {
		for _, res := range spec.Requirements.Resources {
			report.Resources = append(report.Resources, aifvalidate.ExonsResource{
				Ref: res.Ref, Kind: res.Kind, Scope: res.Scope,
			})
		}
	}
	return report
}

// formatIssue renders one issue as the reference's FormatExonsIntakeIssues
// does: "line L, column C: message".
func formatIssue(issue exons.ValidationIssue) string {
	return fmt.Sprintf("line %d, column %d: %s", issue.Position.Line, issue.Position.Column, issue.Message)
}
