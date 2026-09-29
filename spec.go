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
	ReachabilityMarkers    []string `json:"reachabilityTerminalMarkers"`
	ParallelResolutions    []string `json:"parallelResolutions"`
	ForEachResolutions     []string `json:"forEachResolutions"`
	LoopMaxIterationsLimit int      `json:"loopMaxIterationsLimit"`
	CredentialRefPrefix    string   `json:"credentialReferencePrefix"`
	ExecutorURLPattern     string   `json:"executorUrlPattern"`
	TemplateActionOpen     string   `json:"templateActionOpen"`
	KnownKeys              struct {
		Root  string                       `json:"root"`
		Types map[string]map[string]string `json:"types"`
	} `json:"knownKeys"`
	EvalJudgeURL string `json:"evalJudgeUrl"`
	QualityGate  struct {
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
	ExpressionFunctions struct {
		NamePrefix string   `json:"namePrefix"`
		Catalog    []string `json:"catalog"`
	} `json:"expressionFunctions"`
	ProcessingOperations struct {
		GuardKey         string              `json:"guardKey"`
		StandardTypes    []string            `json:"standardTypes"`
		LoopSubStepTypes []string            `json:"loopSubStepTypes"`
		OpenKeyTypes     []string            `json:"openKeyTypes"`
		ClosedConfigKeys map[string][]string `json:"closedConfigKeys"`
	} `json:"processingOperations"`
	TemplateFunctions []string `json:"templateFunctions"`
	// OrchestratorToolAllow feeds orchestrator_tool_withheld (v2.760.0).
	OrchestratorToolAllow struct {
		WarningField         string   `json:"warningField"`
		AskHumanTool         string   `json:"askHumanTool"`
		EnableSignalsDefault bool     `json:"enableSignalsDefault"`
		LifecycleTools       []string `json:"lifecycleTools"`
		SignalTools          []string `json:"signalTools"`
		CampaignTools        []string `json:"campaignTools"`
	} `json:"orchestratorToolAllow"`
	// Exons names where an inline .exons document arrives (v2.767.0, v2.777.0).
	Exons struct {
		ExecutorPrefix       string `json:"executorPrefix"`
		DocumentParam        string `json:"documentParam"`
		FrontmatterDelimiter string `json:"frontmatterDelimiter"`
	} `json:"exons"`
	// ExecutorConfigEnvScopes feeds executor_config_env_scope (v2.597.0).
	ExecutorConfigEnvScopes struct {
		ReferencePrefix string              `json:"referencePrefix"`
		ReferenceSuffix string              `json:"referenceSuffix"`
		Fields          []string            `json:"fields"`
		ExtraKey        string              `json:"extraKey"`
		Scopes          map[string][]string `json:"scopes"`
	} `json:"executorConfigEnvScopes"`
	// ServerOwnedQueryKeys feeds server_owned_query_key (AIgentFlow CFX-05).
	ServerOwnedQueryKeys struct {
		Keys                []string `json:"keys"`
		StepFieldFormat     string   `json:"stepFieldFormat"`
		LoopStepFieldFormat string   `json:"loopStepFieldFormat"`
	} `json:"serverOwnedQueryKeys"`
	// CredentialEndpointPairing feeds credential_endpoint_unpaired (AIgentFlow
	// DC-FORGE-231 and DC-FORGE-233).
	CredentialEndpointPairing credentialEndpointSpec `json:"credentialEndpointPairing"`
}

// credentialEndpointSpec is the data of credential_endpoint_unpaired: the ai://
// provider rule's names and the per-protocol family table. A new family, a new
// server variable or a new default endpoint is a change to this data only.
type credentialEndpointSpec struct {
	StepFieldFormat                  string            `json:"stepFieldFormat"`
	LoopStepFieldFormat              string            `json:"loopStepFieldFormat"`
	ExecutorConfigBaseURLFieldFormat string            `json:"executorConfigBaseUrlFieldFormat"`
	ExecutorConfigKeyFieldFormat     string            `json:"executorConfigKeyFieldFormat"`
	ExecutorConfigExtraFieldFormat   string            `json:"executorConfigExtraFieldFormat"`
	ExecutorConfigAPIKeyField        string            `json:"executorConfigApiKeyField"`
	ExecutorConfigBaseURLField       string            `json:"executorConfigBaseUrlField"`
	ProtocolSeparator                string            `json:"protocolSeparator"`
	DefaultPorts                     map[string]string `json:"defaultPorts"`
	AI                               struct {
		Protocol                   string   `json:"protocol"`
		GenericKeyParam            string   `json:"genericKeyParam"`
		ProviderKeyParamSuffix     string   `json:"providerKeyParamSuffix"`
		ProviderBaseURLParamSuffix string   `json:"providerBaseUrlParamSuffix"`
		KeylessProviders           []string `json:"keylessProviders"`
		ExecutorConfigProviders    []string `json:"executorConfigProviders"`
	} `json:"ai"`
	FamilyKeySeparator string                              `json:"familyKeySeparator"`
	ProtocolAliases    map[string]string                   `json:"protocolAliases"`
	Families           map[string]credentialEndpointFamily `json:"families"`
}

// credentialEndpointFamily is one row of the reference's per-protocol table
// (credentialEndpointFamilies), with its server-variable set evaluated.
type credentialEndpointFamily struct {
	Protocol                   string   `json:"protocol"`
	ConfigKeys                 []string `json:"configKeys"`
	SecretParams               []string `json:"secretParams"`
	EndpointParams             []string `json:"endpointParams"`
	DefaultEndpoints           []string `json:"defaultEndpoints"`
	ConfigAPIKeyParam          string   `json:"configApiKeyParam"`
	ConfigBaseURLParam         string   `json:"configBaseUrlParam"`
	ImplicitServerCredential   bool     `json:"implicitServerCredential"`
	ExpandsServerEnvReferences bool     `json:"expandsServerEnvReferences"`
	ServerEnv                  []string `json:"serverEnv"`
	// StoredKeyShape marks a family judged like ai:// (nexus): the resolver
	// plans a stored key for every step, so an endpoint without a key of the
	// flow's own can only be paired with a stored key.
	StoredKeyShape *struct {
		OwnKeyParams           []string `json:"ownKeyParams"`
		OwnCredentialsMapParam string   `json:"ownCredentialsMapParam"`
		Provider               string   `json:"provider"`
	} `json:"storedKeyShape"`
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
	reachabilityMarkers  set
	forEachResolutions   set
	orchestratorTriggers set
	orchestratorTools    set
	orchestratorModes    set
	qualityGateOnFail    set
	qualityGateRejected  set
	inputSchemaTypes     set
	inputStringTypes     set
	inputParametricTypes set

	// expressionFunctionCatalog is the fixed `expression_functions:` catalog
	// compiled into the AIgentFlow binary (v2.642.0). Nothing is loaded at run
	// time, so a name outside it is an error, not a lag-prone warning.
	expressionFunctionCatalog set

	// The processing-operation dispatch sets. They are a PARTITION, never a
	// union: loop.set / loop.break are dispatchable only on a loop sub-step.
	processingStandardTypes    set
	processingLoopSubStepTypes set
	processingOpenKeyTypes     set

	// serverOwnedQueryKeys are the executor parameters only the server's
	// credential resolver may set; a step query naming one is refused.
	serverOwnedQueryKeys set

	fieldNameRe *regexp.Regexp
	// executorURLRe is the ONE executor-URL shape, mirrored verbatim from the
	// reference's URL_PATTERN_REGEX.
	executorURLRe *regexp.Regexp
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
	reachabilityMarkers = newSet(spec.ReachabilityMarkers)
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
	executorURLRe = regexp.MustCompile(spec.ExecutorURLPattern)
	expressionFunctionCatalog = newSet(spec.ExpressionFunctions.Catalog)
	processingStandardTypes = newSet(spec.ProcessingOperations.StandardTypes)
	processingLoopSubStepTypes = newSet(spec.ProcessingOperations.LoopSubStepTypes)
	processingOpenKeyTypes = newSet(spec.ProcessingOperations.OpenKeyTypes)
	serverOwnedQueryKeys = newSet(spec.ServerOwnedQueryKeys.Keys)
}

// ExpressionFunctionCatalog returns the fixed expression-function catalog
// (sorted). A flow opts into entries with `expression_functions: [{function: …}]`.
func ExpressionFunctionCatalog() []string {
	return sortedSet(expressionFunctionCatalog)
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
