package aifvalidate

import (
	"strings"
	"testing"
)

// v0.8.0: the AigentChat executor was removed from AIgentFlow (v2.819.0).
// The removal is pinned from the outside: what a flow that still names it now
// gets, and that no spec table carries the dead protocol.

func TestRemovedNexusSchemesAreOnlyUnknownSchemeWarnings(t *testing.T) {
	for _, scheme := range []string{"nexus", "aigentchat"} {
		src := unpairedHead + "steps:\n  s:\n    executor: " + scheme + "://org/agent\n    query:\n      message: hi\n      base_url: 'https://chat.example.com'\n"
		result := ValidateFlow(src, Options{})
		if !result.Valid {
			t.Errorf("%s://: a removed scheme must stay saveable (the reference never rejects on scheme), errors=%v", scheme, result.Errors)
		}
		found := false
		for _, w := range result.Warnings {
			if w.Code == codeUnknownExecScheme {
				found = true
			}
			if w.Code == codeCredentialEndpointUnpaired {
				t.Errorf("%s://: no longer a credential family, got %s at %s", scheme, w.Code, w.Field)
			}
		}
		if !found {
			t.Errorf("%s://: want an %s warning, got %v", scheme, codeUnknownExecScheme, result.Warnings)
		}
	}
}

func TestRemovedNexusExecutorConfigKeysHaveAnEmptyScope(t *testing.T) {
	for _, key := range []string{"nexus", "aigentchat"} {
		src := unpairedHead + "executor_config:\n  " + key + ":\n    api_key: '${AIGENTFLOW_NEXUS_API_KEY}'\nsteps:\n  s:\n    executor: function://utils/log\n    query:\n      message: hi\n"
		result := ValidateFlow(src, Options{})
		got := false
		for _, e := range result.Errors {
			if e.Code == codeExecutorConfigEnvScope && strings.HasPrefix(e.Field, "executor_config."+key) {
				got = true
			}
		}
		if !got {
			t.Errorf("executor_config.%s: a reference must be refused as out of scope, errors=%v", key, result.Errors)
		}
	}
}

func TestSpecCarriesNoRemovedProtocol(t *testing.T) {
	if executorSchemes.has("nexus") || executorSchemes.has("aigentchat") {
		t.Error("executorSchemes still names a removed protocol")
	}
	for _, key := range []string{"nexus", "aigentchat"} {
		if _, ok := spec.ExecutorConfigEnvScopes.Scopes[key]; ok {
			t.Errorf("env scope %q still declared", key)
		}
		if _, ok := spec.CredentialEndpointPairing.Families[key]; ok {
			t.Errorf("credential family %q still declared", key)
		}
	}
}
