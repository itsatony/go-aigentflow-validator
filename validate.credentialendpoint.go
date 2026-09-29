package aifvalidate

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// validateCredentialEndpointPairing warns on a step that names an endpoint its
// credential will never be sent to (credential_endpoint_unpaired; reference:
// validateCredentialEndpointPairing, AIgentFlow DC-FORGE-231, and
// validateFamilyCredentialEndpointPairing, DC-FORGE-233).
//
// AIgentFlow sends a credential the SERVER supplied (an org's or the
// platform's stored key, or a value of one of the deployment's environment
// variables) only to the endpoint that came with it: the provider's default,
// the stored credential's own base URL, or a deployment-configured one. An
// endpoint the flow names is honoured only with a credential the flow supplies.
// That run-time refusal is the control; this is the save-time warning about the
// shapes that are statically certain to meet it.
//
// WARNING, never an error, as in the reference: `<provider>_base_url` and a
// family's endpoint parameter have legitimate uses a static reading cannot
// rule out (a key the caller supplies at run time, a keyless self-hosted
// server), and a warning never changes the verdict.
//
// Two arms, both over the same steps (a top-level step and a loop sub-step;
// a parallel branch or a for_each body is a top-level step):
//
//   - ai:// — a step's `<provider>_base_url` with no key of the flow's own
//     (the step query's `api_key` / `<provider>_api_key`, or a literal
//     `executor_config.<provider>.api_key`), and a literal
//     `executor_config.<provider>.base_url` for an AI provider with no literal
//     key beside it, unless every step using that provider brings its own key.
//     Keyless self-hosted providers (ollama, vllm) are exempt.
//   - every other family in the spec's table — a secret that is an
//     `${AIGENTFLOW_…}` reference to one of the family's server variables, on a
//     family whose plugin expands such references, beside an endpoint that is
//     certainly the author's (a literal: not templated, not a variable, not the
//     family's default origin). A family whose client carries an implicit
//     service credential is not judged (that is a property of the deployment),
//     and nexus:// / aigentchat:// get the ai:// shape.
//
// Everything that decides it is data (spec credentialEndpointPairing): the
// ai:// parameter names, the keyless and provider sets, the family table with
// each family's server-variable set evaluated, the default endpoints, and the
// field formats.
//
// Values are read as the reference reads them. A step `query:` and an
// `extra:` block are Go `any` there, so only a YAML string counts (a number is
// never a key or an endpoint). `executor_config.<key>.api_key` and `.base_url`
// are Go `string` fields, read by source text.
func validateCredentialEndpointPairing(flow doc, iss *issues) {
	steps := credentialEndpointSteps(flow, iss)
	validateAICredentialEndpointPairing(flow, steps, iss)
	validateFamilyCredentialEndpointPairing(flow, steps, iss)
}

// credentialEndpointStep is one step the rule judges: its executor, its query
// (nil when it has none) and how a query key of it is addressed.
type credentialEndpointStep struct {
	executor string
	query    doc
	field    func(key string) string
}

// credentialEndpointSteps lists every top-level step and loop sub-step, in the
// reference's order: steps sorted by id, each followed by its loop sub-steps.
func credentialEndpointSteps(flow doc, iss *issues) []credentialEndpointStep {
	cfg := spec.CredentialEndpointPairing
	steps := stepsOf(flow)
	var out []credentialEndpointStep
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		id := stepID
		executor, _ := iss.stringOf(step, keyExecutor, stepField(stepID))
		query, _ := getRecord(step, keyQuery)
		out = append(out, credentialEndpointStep{executor: executor, query: query, field: func(key string) string {
			return fmt.Sprintf(cfg.StepFieldFormat, id, key)
		}})
		for _, sub := range loopSubStepsOf(stepID, step) {
			subID, _ := iss.stringOf(sub.raw, keyID, sub.basePath)
			subExecutor, _ := iss.stringOf(sub.raw, keyExecutor, sub.basePath)
			subQuery, _ := getRecord(sub.raw, keyQuery)
			out = append(out, credentialEndpointStep{executor: subExecutor, query: subQuery, field: func(key string) string {
				return fmt.Sprintf(cfg.LoopStepFieldFormat, id, subID, key)
			}})
		}
	}
	return out
}

// queryNonEmptyString reads a step-query value the way the reference's
// `query[key].(string)` does: a YAML string, and a non-empty one.
func queryNonEmptyString(query doc, key string) (string, bool) {
	v, ok := asString(get(query, key))
	return v, ok && v != ""
}

// executorConfigInstance returns `executor_config.<key>` when it is a mapping.
func executorConfigInstance(flow doc, key string) (doc, bool) {
	config, ok := getRecord(flow, keyExecutorConfig)
	if !ok {
		return nil, false
	}
	return getRecord(config, key)
}

// executorConfigStringField reads a Go-`string` field of an executor_config
// instance (api_key, base_url) by source text; "" when absent or null.
func executorConfigStringField(inst doc, configKey, field string, iss *issues) string {
	v, _ := iss.stringOf(inst, field, keyExecutorConfig+"."+configKey)
	return v
}

// --- ai:// -----------------------------------------------------------------

// aiStepProvider returns the provider of an `ai://<provider>/<op>` executor,
// or "" for any other executor.
func aiStepProvider(executor string) string {
	cfg := spec.CredentialEndpointPairing
	rest, ok := strings.CutPrefix(executor, cfg.AI.Protocol+cfg.ProtocolSeparator)
	if !ok {
		return ""
	}
	provider, _, _ := strings.Cut(rest, "/")
	return provider
}

// aiQueryHasOwnKey reports whether a step query supplies a key of the flow's
// own for a provider (literal or templated: either way it is not a stored key).
func aiQueryHasOwnKey(query doc, provider string) bool {
	ai := spec.CredentialEndpointPairing.AI
	for _, key := range []string{ai.GenericKeyParam, provider + ai.ProviderKeyParamSuffix} {
		if _, ok := queryNonEmptyString(query, key); ok {
			return true
		}
	}
	return false
}

// aiConfigHasOwnKey reports whether `executor_config.<provider>.api_key` is a
// literal of the author's own (not an `${ENV}` reference to the server's key).
func aiConfigHasOwnKey(flow doc, provider string, iss *issues) bool {
	inst, ok := executorConfigInstance(flow, provider)
	if !ok {
		return false
	}
	key := executorConfigStringField(inst, provider, spec.CredentialEndpointPairing.ExecutorConfigAPIKeyField, iss)
	if key == "" {
		return false
	}
	_, isRef := executorConfigEnvReference(key)
	return !isRef
}

func validateAICredentialEndpointPairing(flow doc, steps []credentialEndpointStep, iss *issues) {
	cfg := spec.CredentialEndpointPairing
	keyless := newSet(cfg.AI.KeylessProviders)
	// Per provider: the ai:// steps using it, and those carrying no query key of
	// their own. The executor_config arm reads both, so that a block whose every
	// step brings its own key is not flagged.
	stepsUsing := map[string]int{}
	stepsWithoutOwnKey := map[string]int{}
	for _, step := range steps {
		provider := aiStepProvider(step.executor)
		if provider == "" || keyless.has(provider) {
			continue
		}
		stepsUsing[provider]++
		queryOwn := aiQueryHasOwnKey(step.query, provider)
		if !queryOwn {
			stepsWithoutOwnKey[provider]++
		}
		urlKey := provider + cfg.AI.ProviderBaseURLParamSuffix
		if _, ok := queryNonEmptyString(step.query, urlKey); ok && !queryOwn && !aiConfigHasOwnKey(flow, provider, iss) {
			warnUnpairedAI(iss, step.field(urlKey), provider)
		}
	}

	config, ok := getRecord(flow, keyExecutorConfig)
	if !ok {
		return
	}
	providers := newSet(cfg.AI.ExecutorConfigProviders)
	for _, provider := range sortedKeys(config) {
		inst, isMap := asRecord(config[provider])
		if !isMap || !providers.has(provider) || keyless.has(provider) {
			continue
		}
		baseURL := executorConfigStringField(inst, provider, cfg.ExecutorConfigBaseURLField, iss)
		if baseURL == "" {
			continue
		}
		if _, isRef := executorConfigEnvReference(baseURL); isRef {
			continue // `${AIGENTFLOW_<P>_BASE_URL}` is the deployment's endpoint
		}
		if aiConfigHasOwnKey(flow, provider, iss) {
			continue
		}
		if stepsUsing[provider] > 0 && stepsWithoutOwnKey[provider] == 0 {
			continue // every step brings its own key
		}
		warnUnpairedAI(iss, fmt.Sprintf(cfg.ExecutorConfigBaseURLFieldFormat, provider), provider)
	}
}

func warnUnpairedAI(iss *issues, field, provider string) {
	iss.warn(Issue{
		Field: field, Code: codeCredentialEndpointUnpaired,
		Message: fmt.Sprintf("%s names an endpoint for %q but the flow supplies no %s key of its own. At run time "+
			"the org's (or the platform's) stored %s credential is never sent to a host the flow chooses, so this "+
			"step will be refused unless the run supplies its own key.", field, provider, provider, provider),
		Suggestion: "To use a custom endpoint with the org's credential, store the base_url on the credential; " +
			"to use the provider's default endpoint, remove this field",
	})
}

// --- the executor families -------------------------------------------------

// credentialEndpointFamilyOf returns the family row a step's executor belongs
// to: the protocol (after its alias) with its first path segment as the
// driver, else the protocol alone.
func credentialEndpointFamilyOf(executor string) (string, credentialEndpointFamily, bool) {
	cfg := spec.CredentialEndpointPairing
	sep := strings.Index(executor, cfg.ProtocolSeparator)
	if sep <= 0 {
		return "", credentialEndpointFamily{}, false
	}
	protocol := executor[:sep]
	driver, _, _ := strings.Cut(executor[sep+len(cfg.ProtocolSeparator):], "/")
	if alias, ok := cfg.ProtocolAliases[protocol]; ok {
		protocol = alias
	}
	candidates := []string{protocol}
	if driver != "" {
		candidates = []string{protocol + cfg.FamilyKeySeparator + driver, protocol}
	}
	for _, key := range candidates {
		if family, ok := cfg.Families[key]; ok {
			return key, family, true
		}
	}
	return "", credentialEndpointFamily{}, false
}

// familyServerEnvRef reports whether a value is an `${ENV}` reference to one of
// the family's server variables that its plugin EXPANDS, and which. Elsewhere a
// `${…}` reaches the driver as the literal string, which is no server secret.
func familyServerEnvRef(family credentialEndpointFamily, value string) (string, bool) {
	name, isRef := executorConfigEnvReference(value)
	if !isRef || !family.ExpandsServerEnvReferences || !slices.Contains(family.ServerEnv, name) {
		return "", false
	}
	return name, true
}

// familyListHas is the reference's credentialEndpointListHas: "" is never a
// member.
func familyListHas(list []string, s string) bool {
	return s != "" && slices.Contains(list, s)
}

// familySecret is where a step's credential comes from, as far as the YAML
// shows: whether it is a server variable, which, and the field that holds it.
type familySecret struct {
	server  bool
	envName string
	field   string
}

// staticFamilySecret finds the step's credential: the step query first (the
// plugins' own precedence), then each of the family's executor_config blocks.
func staticFamilySecret(flow doc, family credentialEndpointFamily, step credentialEndpointStep, iss *issues) familySecret {
	cfg := spec.CredentialEndpointPairing
	for _, key := range family.SecretParams {
		if v, ok := queryNonEmptyString(step.query, key); ok {
			name, server := familyServerEnvRef(family, v)
			return familySecret{server: server, envName: name, field: step.field(key)}
		}
	}
	for _, configKey := range family.ConfigKeys {
		inst, ok := executorConfigInstance(flow, configKey)
		if !ok {
			continue
		}
		if apiKey := executorConfigStringField(inst, configKey, cfg.ExecutorConfigAPIKeyField, iss); apiKey != "" &&
			familyListHas(family.SecretParams, family.ConfigAPIKeyParam) {
			name, server := familyServerEnvRef(family, apiKey)
			return familySecret{server: server, envName: name,
				field: fmt.Sprintf(cfg.ExecutorConfigKeyFieldFormat, configKey, cfg.ExecutorConfigAPIKeyField)}
		}
		extra, _ := getRecord(inst, spec.ExecutorConfigEnvScopes.ExtraKey)
		for _, key := range family.SecretParams {
			if v, ok := queryNonEmptyString(extra, key); ok {
				name, server := familyServerEnvRef(family, v)
				return familySecret{server: server, envName: name,
					field: fmt.Sprintf(cfg.ExecutorConfigExtraFieldFormat, configKey, key)}
			}
		}
	}
	return familySecret{}
}

// familyEndpointIsAuthorLiteral reports whether an endpoint is certainly the
// author's: a literal that is neither templated, nor a deployment variable, nor
// of the same origin as one of the family's defaults.
func familyEndpointIsAuthorLiteral(family credentialEndpointFamily, v string) bool {
	if v == "" || strings.Contains(v, spec.TemplateActionOpen) {
		return false
	}
	if _, isRef := executorConfigEnvReference(v); isRef {
		return false
	}
	origin := credentialEndpointOrigin(v)
	for _, d := range family.DefaultEndpoints {
		if credentialEndpointOrigin(d) == origin {
			return false
		}
	}
	return true
}

// staticFamilyAuthorEndpoint returns the field of the endpoint the step will
// use when it is certainly the author's, or "". The first endpoint found
// decides, in the same order as the secret.
func staticFamilyAuthorEndpoint(flow doc, family credentialEndpointFamily, step credentialEndpointStep, iss *issues) string {
	cfg := spec.CredentialEndpointPairing
	decide := func(v, field string) string {
		if familyEndpointIsAuthorLiteral(family, v) {
			return field
		}
		return ""
	}
	for _, key := range family.EndpointParams {
		if v, ok := queryNonEmptyString(step.query, key); ok {
			return decide(v, step.field(key))
		}
	}
	for _, configKey := range family.ConfigKeys {
		inst, ok := executorConfigInstance(flow, configKey)
		if !ok {
			continue
		}
		if baseURL := executorConfigStringField(inst, configKey, cfg.ExecutorConfigBaseURLField, iss); baseURL != "" &&
			familyListHas(family.EndpointParams, family.ConfigBaseURLParam) {
			return decide(baseURL, fmt.Sprintf(cfg.ExecutorConfigBaseURLFieldFormat, configKey))
		}
		extra, _ := getRecord(inst, spec.ExecutorConfigEnvScopes.ExtraKey)
		for _, key := range family.EndpointParams {
			if v, ok := queryNonEmptyString(extra, key); ok {
				return decide(v, fmt.Sprintf(cfg.ExecutorConfigExtraFieldFormat, configKey, key))
			}
		}
	}
	return ""
}

// storedKeyShapeHasOwnKey reports whether a stored-key-shape step (nexus://)
// supplies a key of its own: a non-empty string key, or a credentials mapping.
func storedKeyShapeHasOwnKey(family credentialEndpointFamily, query doc) bool {
	for _, key := range family.StoredKeyShape.OwnKeyParams {
		if _, ok := queryNonEmptyString(query, key); ok {
			return true
		}
	}
	_, hasMap := getRecord(query, family.StoredKeyShape.OwnCredentialsMapParam)
	return hasMap
}

func validateFamilyCredentialEndpointPairing(flow doc, steps []credentialEndpointStep, iss *issues) {
	for _, step := range steps {
		familyKey, family, ok := credentialEndpointFamilyOf(step.executor)
		if !ok || family.ImplicitServerCredential {
			continue
		}
		if family.StoredKeyShape != nil {
			if storedKeyShapeHasOwnKey(family, step.query) {
				continue
			}
			for _, key := range family.EndpointParams {
				if _, ok := queryNonEmptyString(step.query, key); ok {
					warnUnpairedAI(iss, step.field(key), family.StoredKeyShape.Provider)
					break
				}
			}
			continue
		}
		secret := staticFamilySecret(flow, family, step, iss)
		if !secret.server {
			continue
		}
		endpointField := staticFamilyAuthorEndpoint(flow, family, step, iss)
		if endpointField == "" {
			continue
		}
		iss.warn(Issue{
			Field: endpointField, Code: codeCredentialEndpointUnpaired,
			Message: fmt.Sprintf("%s names an endpoint for %s while the step's credential %s comes from the "+
				"server's environment (%s). A server-supplied credential is only sent to the executor's default or "+
				"deployment-configured endpoint, so this step is refused at run time unless that endpoint is the "+
				"deployment's own.", endpointField, familyKey, secret.field, secret.envName),
			Suggestion: "Supply the endpoint's own credential, or remove the endpoint (or name it by its deployment " +
				"variable) to use the deployment's",
		})
	}
}

// credentialEndpointOrigin reduces an endpoint to what decides who receives a
// credential, exactly as the reference does: lower-cased scheme://host:port
// with the scheme's default port filled. A value net/url does not parse, or
// with no scheme or no host (a bare device IP, a hostname), is compared as its
// trimmed, lower-cased self without trailing slashes.
func credentialEndpointOrigin(raw string) string {
	cfg := spec.CredentialEndpointPairing
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(strings.TrimRight(trimmed, "/"))
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if lastColon := strings.LastIndex(host, ":"); lastColon < 0 || lastColon <= strings.LastIndex(host, "]") {
		if port, ok := cfg.DefaultPorts[scheme]; ok {
			host += ":" + port
		}
	}
	return scheme + cfg.ProtocolSeparator + host
}
