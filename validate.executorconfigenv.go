package aifvalidate

import (
	"fmt"
	"slices"
	"strings"
)

// validateExecutorConfigEnvScopes refuses an `executor_config:` value that names
// an environment variable outside the scope of the key it is written under.
//
// Port of the reference's ValidateExecutorConfigEnvScopes (v2.597.0), which the
// save door applies to the whole block. `executor_config` is author-written, so
// without it an author could point `base_url` at their own endpoint and have the
// server expand a secret that belongs to another provider into `api_key`.
//
// The checked values are the three Go-`string` fields (`api_key`, `base_url`,
// `organization`, read by source text like every string field) and every YAML
// STRING in `extra` (the reference type-asserts `.(string)` on an `any`, so a
// number there is never a reference). A value is a reference when it is exactly
// `${NAME}` with NAME non-empty; `${A}${B}` is the single name `A}${B` and is
// refused unless a scope names it, as in the reference.
//
// The scopes are the reference's executorConfigEnvScope evaluated for every key
// that has one (spec executorConfigEnvScopes.scopes). Any other key has an EMPTY
// scope and refuses every reference: the reference reads those values raw, so a
// `${...}` there was sent verbatim as a credential.
//
// The reference stops at the first violation (a parse-door refusal); this
// library reports each one. The verdict is the same.
func validateExecutorConfigEnvScopes(flow doc, iss *issues) {
	config, ok := getRecord(flow, keyExecutorConfig)
	if !ok {
		return
	}
	scopes := spec.ExecutorConfigEnvScopes
	for _, configKey := range sortedKeys(config) {
		instance, isMap := asRecord(config[configKey])
		if !isMap {
			// null is skipped by the reference; any other non-mapping is an
			// invalid_type from the unknown-keys rule.
			continue
		}
		base := keyExecutorConfig + "." + configKey
		scope := scopes.Scopes[configKey]
		for _, field := range scopes.Fields {
			value, isStr := iss.stringOf(instance, field, base)
			if !isStr {
				continue
			}
			checkExecutorConfigEnvReference(iss, base+"."+field, configKey, field, value, scope)
		}
		extra, hasExtra := getRecord(instance, scopes.ExtraKey)
		if !hasExtra {
			continue
		}
		for _, extraKey := range sortedKeys(extra) {
			value, isStr := asString(extra[extraKey])
			if !isStr {
				continue
			}
			checkExecutorConfigEnvReference(iss, base+"."+scopes.ExtraKey+"."+extraKey, configKey,
				scopes.ExtraKey+"."+extraKey, value, scope)
		}
	}
}

func checkExecutorConfigEnvReference(iss *issues, path, configKey, fieldName, value string, scope []string) {
	name, isRef := executorConfigEnvReference(value)
	if !isRef || slices.Contains(scope, name) {
		return
	}
	permitted := "(none: this key expands no environment variables)"
	if len(scope) > 0 {
		permitted = joinNames(scope)
	}
	iss.error(Issue{
		Field: path, Code: codeExecutorConfigEnvScope,
		Message: fmt.Sprintf("executor_config key %q sets %s to environment variable %q, which it may not expand; "+
			"a flow may only expand the variables belonging to the provider or protocol it writes them under. "+
			"Permitted here: %s", configKey, fieldName, name, permitted),
		Suggestion: "To use an organisation secret, bind a stored credential on the step instead: " +
			`credentials: {<name>: {source: "stored/<provider>/<credential>", inject_as: "api_key"}}`,
	})
}

// executorConfigEnvReference mirrors the reference's isAuthorEnvReference.
func executorConfigEnvReference(value string) (string, bool) {
	prefix, suffix := spec.ExecutorConfigEnvScopes.ReferencePrefix, spec.ExecutorConfigEnvScopes.ReferenceSuffix
	if len(value) < len(prefix)+len(suffix) || !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return "", false
	}
	name := value[len(prefix) : len(value)-len(suffix)]
	if name == "" {
		return "", false
	}
	return name, true
}
