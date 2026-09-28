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

	// Exons judges the inline .exons documents a flow carries (a step's
	// `query.exons` on an exons:// executor, and `orchestrator.exons`).
	//
	// nil uses the built-in reader, which has no .exons template engine: it
	// reads the YAML frontmatter only (execution.provider, tools.allow,
	// requirements.resources) and cannot tell whether the document PARSES or
	// whether its tags can RENDER. Those two refusals (exons_attributes and
	// orchestrator_exons_parse_failed) then never fire, so the verdict is
	// looser than the reference's for a document its engine refuses.
	//
	// For the reference's exact verdict, pass the go-exons-backed inspector
	// from the companion module:
	//
	//	import "github.com/itsatony/go-aigentflow-validator/exonsinspect"
	//	opts := aifvalidate.Options{Exons: exonsinspect.New()}
	//
	// It lives in its own module so this one keeps a single dependency and
	// builds for GOOS=js GOARCH=wasm. See PARITY.md, divergence #16.
	Exons ExonsInspector
}

// ExonsInspector judges one .exons document. Implementations must be safe for
// concurrent use and must not panic on malformed input.
type ExonsInspector interface {
	InspectExons(source string) ExonsReport
}

// ExonsReport is what the save-door rules need to know about one .exons
// document. Every field mirrors a value the reference reads from go-exons.
type ExonsReport struct {
	// Engine is true when a real .exons engine produced the report, so that
	// IntakeErrors is authoritative. When it is false IntakeErrors is ignored:
	// "no error" would be a claim nobody checked.
	Engine bool `json:"engine"`
	// ParseError is why the engine's Parse refuses the document, "" when it
	// parses or when the inspector cannot tell. The reference refuses an
	// orchestrator whose document does not parse. An inspector without an
	// engine sets it only where the refusal is certain (the built-in reader:
	// an unclosed frontmatter, or the retired JSON config block).
	ParseError string `json:"parse_error,omitempty"`
	// IntakeErrors are the error-severity issues of the engine's Validate —
	// a tag its own resolver refuses at execute (the reference's
	// ExonsIntakeErrors, v2.777.0). Warnings are not included.
	IntakeErrors []string `json:"intake_errors,omitempty"`

	// SpecRead is true when the frontmatter fields below are known. The
	// built-in reader leaves it false when it cannot read the frontmatter the
	// way the engine would (a template tag inside it, or YAML it cannot
	// decode), and then no rule reads them.
	SpecRead bool `json:"spec_read"`
	// HasSpec is true when the document declares a frontmatter spec. The
	// reference refuses an orchestrator document without one.
	HasSpec bool `json:"has_spec"`
	// Provider is the spec's execution.provider.
	Provider string `json:"provider,omitempty"`
	// ToolsAllowDeclared distinguishes an absent tools.allow (no narrowing)
	// from an empty one (nothing but the exempt tools).
	ToolsAllowDeclared bool     `json:"tools_allow_declared"`
	ToolsAllow         []string `json:"tools_allow,omitempty"`
	// Resources are the spec's requirements.resources entries.
	Resources []ExonsResource `json:"resources,omitempty"`
}

// ExonsResource is one requirements.resources entry.
type ExonsResource struct {
	Ref   string `json:"ref"`
	Kind  string `json:"kind"`
	Scope string `json:"scope,omitempty"`
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
