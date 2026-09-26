package aifvalidate

// Severity of a validation issue. Info is reserved for future lints.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Issue is a single validation finding. Errors and warnings share this shape;
// a warning simply carries SeverityWarning and never lowers Result.Valid.
//
// The JSON tags mirror the JS validator's ValidationIssue so a consumer can
// serialise either implementation's findings and render them with one type.
type Issue struct {
	// Field is the dotted path to the offending value, e.g.
	// "steps.fetch.query.url". It is the primary way an author locates the
	// problem when Line is unavailable.
	Field string `json:"field"`
	// Message is the human-readable description. NOT part of the parity
	// contract — wording may differ from the JS implementation.
	Message string `json:"message"`
	// Code is the stable, machine-readable identifier. This IS the
	// cross-implementation parity contract; compare on Code, never Message.
	Code string `json:"code"`
	// Severity is "error" or "warning" ("info" reserved).
	Severity Severity `json:"severity"`
	// StepID names the step when the issue is step-specific.
	StepID string `json:"step_id,omitempty"`
	// Line and Column are 1-based positions in the source YAML, set only when
	// the finding could be located (parse errors always; structural findings
	// when the document was parsed with position tracking).
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
	// Context carries extra orientation, e.g. the available step names.
	Context string `json:"context,omitempty"`
	// Suggestion is a concrete proposed fix.
	Suggestion string `json:"suggestion,omitempty"`
}

// Summary is the roll-up for one validation run.
type Summary struct {
	TotalSteps     int `json:"total_steps"`
	ValidSteps     int `json:"valid_steps"`
	ErrorCount     int `json:"error_count"`
	WarningCount   int `json:"warning_count"`
	TemplatesFound int `json:"templates_found"`
	TemplatesValid int `json:"templates_valid"`
}

// Result is the complete verdict for one flow.
//
// Errors and Warnings are ALWAYS non-nil slices, even when empty, so the JSON
// encoding is `[]` and never `null`. A consumer's client code that calls an
// array method on a `null` is a real and expensive failure mode; a library is
// the right place to make it impossible.
type Result struct {
	// Valid is true when there are zero error-severity issues. Warnings never
	// affect it.
	Valid bool `json:"valid"`
	// Errors are the blocking findings.
	Errors []Issue `json:"errors"`
	// Warnings are advisory findings. A consumer that treats these as blocking
	// will reject legitimate flows, because the vendored allow-lists (executor
	// schemes, template functions, orchestrator tools) can lag the live
	// AIgentFlow registries.
	Warnings []Issue `json:"warnings"`
	// Summary is the roll-up.
	Summary Summary `json:"summary"`
	// SpecVersion records which AIgentFlow flow-schema version produced this
	// verdict. Carried on the result (not just available via SpecVersion()) so a
	// verdict remains self-describing after it is serialised and stored.
	SpecVersion string `json:"spec_version"`
}

// Options tunes validation.
type Options struct {
	// StrictRegistries promotes "unrecognised name" findings — an unknown
	// orchestrator tool, an unknown template function — from warning to error.
	//
	// OFF by default, deliberately: the vendored allow-lists can lag the live
	// AIgentFlow registries, and on a publish gate a false-positive error is
	// strictly worse than a missed lint. Turn it on for authoring-time linting,
	// not for admission control.
	StrictRegistries bool
}

// issues accumulates findings during a validation pass. It is the port of the
// JS `Issues` class; the constructor guarantees non-nil slices so Result never
// marshals a null array.
type issues struct {
	errors   []Issue
	warnings []Issue
	// sources is the pass's one piece of read-only context: the SOURCE spelling
	// of every number/boolean scalar, by field path. Only a YAML text has it, so
	// it is nil under ValidateFlowObject. See scalarTextAt.
	sources scalarSources
}

func newIssues() *issues {
	return &issues{errors: []Issue{}, warnings: []Issue{}}
}

func (i *issues) error(issue Issue) {
	issue.Severity = SeverityError
	i.errors = append(i.errors, issue)
}

func (i *issues) warn(issue Issue) {
	issue.Severity = SeverityWarning
	i.warnings = append(i.warnings, issue)
}
