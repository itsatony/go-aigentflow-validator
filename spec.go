// Package aifvalidate is a static, offline validator for AIgentFlow workflow YAML.
//
// It reproduces the *static* verdicts of the AIgentFlow Go reference validator
// (Validator.ValidateFlowWithDetails + FlowParser.ValidateFlow) and is the Go
// counterpart of github.com/itsatony/aigentflow-flow-validator-js. Error Code
// strings are the cross-implementation parity contract; Message wording may
// differ between the two. See PARITY.md.
//
// Scope: static checks only. Credentials, the model-compliance catalogue, and
// runtime template field-resolution require a live server and are out of scope.
//
// The library performs no network or filesystem access and builds for
// GOOS=js GOARCH=wasm, so it can run in a server, a CLI, and a browser.
package aifvalidate

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
)

// specJSON is the vendored enum surface mirrored from the AIgentFlow Go
// reference implementation. It is embedded VERBATIM — byte-identical to
// aigentflow-flow-validator-js's src/spec/aigentflow-spec.json — so the two
// implementations share one source of truth for protocol names and cannot drift
// on an enum bump. Never transcribe these values into Go literals: an upstream
// enum change must stay a one-file diff that the conformance suite re-verifies.
//
//go:embed spec/aigentflow-spec.json
var specJSON []byte

// rawSpec is the decoded shape of the vendored spec document. Field names track
// the JSON keys exactly.
type rawSpec struct {
	SpecVersion            string   `json:"specVersion"`
	InputSchemaVersion     int      `json:"inputSchemaVersion"`
	ExecutorSchemes        []string `json:"executorSchemes"`
	DataTypes              []string `json:"dataTypes"`
	ErrorStrategyActions   []string `json:"errorStrategyActions"`
	RetryOnCategories      []string `json:"retryOnCategories"`
	NextMarkers            []string `json:"nextMarkers"`
	ParallelResolutions    []string `json:"parallelResolutions"`
	ForEachResolutions     []string `json:"forEachResolutions"`
	LoopMaxIterationsLimit int      `json:"loopMaxIterationsLimit"`
	CredentialRefPrefix    string   `json:"credentialReferencePrefix"`
	EvalJudgeURL           string   `json:"evalJudgeUrl"`
	QualityGate            struct {
		OnFailActions  []string `json:"onFailActions"`
		OnFailRejected []string `json:"onFailRejected"`
		ThresholdMin   float64  `json:"thresholdMin"`
		ThresholdMax   float64  `json:"thresholdMax"`
	} `json:"qualityGate"`
	InputSchema struct {
		Types                []string `json:"types"`
		StringTypes          []string `json:"stringTypes"`
		ParametricTypes      []string `json:"parametricTypes"`
		FieldNamePattern     string   `json:"fieldNamePattern"`
		DatePattern          string   `json:"datePattern"`
		MaxPatternLength     int      `json:"maxPatternLength"`
		MaxConstraintValue   float64  `json:"maxConstraintValue"`
		MaxInputKeyCount     int      `json:"maxInputKeyCount"`
		MaxStringInputLength int      `json:"maxStringInputLength"`
	} `json:"inputSchema"`
	Orchestrator struct {
		Triggers []string `json:"triggers"`
		Tools    []string `json:"tools"`
		Modes    []string `json:"modes"`
	} `json:"orchestrator"`
	TemplateFunctions []string `json:"templateFunctions"`
}

// The decoded enum surface, built once at init. These are package-level because
// they are immutable protocol data; every validator reads them and none writes.
var (
	spec rawSpec

	executorSchemes      set
	dataTypes            set
	errorStrategyActions set
	retryOnCategories    set
	nextMarkers          set
	forEachResolutions   set
	orchestratorTriggers set
	orchestratorTools    set
	orchestratorModes    set
	qualityGateOnFail    set
	qualityGateRejected  set
	inputSchemaTypes     set
	inputStringTypes     set
	inputParametricTypes set

	fieldNameRe *regexp.Regexp
)

// Enum surfaces the vendored spec carries that NO static rule consumes yet —
// `parallelResolutions`, `inputSchema.datePattern`, `inputSchema.maxInputKeyCount`,
// `inputSchema.maxStringInputLength`, and `evalJudgeUrl`. They are unused in the JS
// implementation too (the reference validates them at runtime, not statically), and
// they stay in the embedded document so it remains byte-identical upstream. Do not
// "clean them up" out of spec/aigentflow-spec.json: that would break the
// byte-equality parity check.

// set is a string membership set. A named type (not a bare map) so a validator
// reads `executorSchemes.has(x)` rather than a two-value map index.
type set map[string]struct{}

func newSet(values []string) set {
	s := make(set, len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

func (s set) has(v string) bool {
	_, ok := s[v]
	return ok
}

// init decodes the embedded spec. A malformed embed is a build-time authoring
// error, not a runtime condition a caller could handle, so it panics — the
// alternative is every validator silently passing everything.
func init() {
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		panic(fmt.Sprintf("aifvalidate: embedded aigentflow-spec.json is not decodable: %v", err))
	}
	executorSchemes = newSet(spec.ExecutorSchemes)
	dataTypes = newSet(spec.DataTypes)
	errorStrategyActions = newSet(spec.ErrorStrategyActions)
	retryOnCategories = newSet(spec.RetryOnCategories)
	nextMarkers = newSet(spec.NextMarkers)
	forEachResolutions = newSet(spec.ForEachResolutions)
	orchestratorTriggers = newSet(spec.Orchestrator.Triggers)
	orchestratorTools = newSet(spec.Orchestrator.Tools)
	orchestratorModes = newSet(spec.Orchestrator.Modes)
	qualityGateOnFail = newSet(spec.QualityGate.OnFailActions)
	qualityGateRejected = newSet(spec.QualityGate.OnFailRejected)
	inputSchemaTypes = newSet(spec.InputSchema.Types)
	inputStringTypes = newSet(spec.InputSchema.StringTypes)
	inputParametricTypes = newSet(spec.InputSchema.ParametricTypes)
	fieldNameRe = regexp.MustCompile(spec.InputSchema.FieldNamePattern)
}

// SpecVersion is the AIgentFlow flow-schema version whose static rules this
// validator tracks. Report it alongside any verdict: a consumer that BLOCKS on
// an error needs to tell an author which schema version judged their document,
// because a newer valid flow can legitimately fail an older pinned validator.
func SpecVersion() string { return spec.SpecVersion }

// InputSchemaVersion is the only supported `input_schema.version` value.
func InputSchemaVersion() int { return spec.InputSchemaVersion }

// TemplateFunctionNames returns the recognised Go-template function names (Go
// builtins plus the AIgentFlow registry). Exported because a consumer may want
// to offer them as editor completions.
func TemplateFunctionNames() []string {
	out := make([]string, 0, len(spec.TemplateFunctions))
	out = append(out, spec.TemplateFunctions...)
	return out
}

// ExecutorSchemes returns the known executor URI schemes. Unknown schemes are
// WARNED, never rejected — the vendored list can lag the live registry.
func ExecutorSchemes() []string {
	out := make([]string, 0, len(spec.ExecutorSchemes))
	out = append(out, spec.ExecutorSchemes...)
	return out
}
