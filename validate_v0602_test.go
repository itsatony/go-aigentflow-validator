package aifvalidate

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// v0.6.2: credential_endpoint_unpaired (AIgentFlow DC-FORGE-231 / DC-FORGE-233).
// The conformance fixtures pin the code per fixture; these pin what a code set
// cannot show — the exact fields (so a finding on the wrong key, a duplicate or
// a missing arm goes red), the severity, and the family data.

func unpairedFields(t *testing.T, src string) []string {
	t.Helper()
	result := ValidateFlow(src, Options{})
	for _, e := range result.Errors {
		if e.Code == codeCredentialEndpointUnpaired {
			t.Fatalf("credential_endpoint_unpaired must be a WARNING, got an error at %s", e.Field)
		}
	}
	var out []string
	for _, w := range result.Warnings {
		if w.Code == codeCredentialEndpointUnpaired {
			out = append(out, w.Field)
		}
	}
	sort.Strings(out)
	return out
}

// TestCredentialEndpointFixtureFields pins every credential-endpoint fixture's
// exact field list. These are the fields AIgentFlow's own validator reports on
// the same files (measured at the reference's DC-FORGE-233 commit).
func TestCredentialEndpointFixtureFields(t *testing.T) {
	want := map[string][]string{
		"warn-credential-endpoint-ai-step-query.yaml": {
			"steps.literal.query.openai_base_url",
			"steps.other_key.query.mistral_base_url",
			"steps.templated.query.anthropic_base_url",
		},
		"warn-credential-endpoint-ai-loop-sub-step.yaml": {
			"steps.rounds.loop.steps.stored.query.openai_base_url",
		},
		"warn-credential-endpoint-ai-executor-config.yaml": {
			"executor_config.anthropic.base_url",
			"executor_config.mistral.base_url",
			"executor_config.openai.base_url",
		},
		"warn-credential-endpoint-family-step-query.yaml": {
			"steps.broker.query.broker_url",
			"steps.rounds.loop.steps.map.query.base_url",
			"steps.vectors.query.base_url",
		},
		"warn-credential-endpoint-family-executor-config.yaml": {
			"executor_config.api.base_url",
			"executor_config.s3.extra.endpoint",
			"steps.research.query.base_url",
		},
		"warn-credential-endpoint-nexus.yaml": {
			"steps.agent.query.base_url",
			"steps.alias.query.aigentchat_base_url",
		},
		"valid-credential-endpoint-own-key-own-url.yaml":   nil,
		"valid-credential-endpoint-server-endpoints.yaml":  nil,
		"valid-credential-endpoint-keyless-providers.yaml": nil,
		"valid-credential-endpoint-not-judged.yaml":        nil,
	}
	files, err := filepath.Glob(filepath.Join("testdata", "conformance", "*credential-endpoint*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(want) {
		t.Fatalf("found %d credential-endpoint fixtures, this table pins %d", len(files), len(want))
	}
	for _, path := range files {
		name := filepath.Base(path)
		wantFields, ok := want[name]
		if !ok {
			t.Errorf("%s: fixture not pinned here", name)
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := unpairedFields(t, string(raw)); !slices.Equal(got, wantFields) {
			t.Errorf("%s: fields = %v, want %v", name, got, wantFields)
		}
	}
}

const unpairedHead = "aigentflow_version: '2.0.0'\nname: p\nversion: '1.0.0'\ndescription: d\nstart: s\n"

// TestCredentialEndpointShapes pins the boundaries one shape at a time.
func TestCredentialEndpointShapes(t *testing.T) {
	step := func(executor, query string) string {
		return unpairedHead + "steps:\n  s:\n    executor: " + executor + "\n    query:\n" + query
	}
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"ai: a numeric key is no key (the reference reads query[k].(string))",
			step("ai://openai/chat", "      api_key: 12345\n      openai_base_url: 'https://x.example.com'\n"),
			[]string{"steps.s.query.openai_base_url"}},
		{"ai: a numeric endpoint is no endpoint",
			step("ai://openai/chat", "      openai_base_url: 8080\n"), nil},
		{"ai: a timestamp is not a string to yaml.v3",
			step("ai://openai/chat", "      openai_base_url: 2024-01-01\n"), nil},
		{"ai: an empty endpoint is none",
			step("ai://openai/chat", "      openai_base_url: ''\n"), nil},
		{"ai: the provider is case-sensitive, as written",
			step("ai://OpenAI/chat", "      OpenAI_base_url: 'https://x.example.com'\n"),
			[]string{"steps.s.query.OpenAI_base_url"}},
		{"ai: a literal executor_config key covers a step endpoint",
			unpairedHead + "executor_config:\n  openai:\n    api_key: 12345\nsteps:\n  s:\n    executor: ai://openai/chat\n    query:\n      openai_base_url: 'https://x.example.com'\n",
			nil},
		{"ai: an env-reference executor_config key does not",
			unpairedHead + "executor_config:\n  openai:\n    api_key: '${AIGENTFLOW_OPENAI_API_KEY}'\nsteps:\n  s:\n    executor: ai://openai/chat\n    query:\n      openai_base_url: 'https://x.example.com'\n",
			[]string{"steps.s.query.openai_base_url"}},
		{"ai: a null executor_config block is no key",
			unpairedHead + "executor_config:\n  openai: ~\nsteps:\n  s:\n    executor: ai://openai/chat\n    query:\n      openai_base_url: 'https://x.example.com'\n",
			[]string{"steps.s.query.openai_base_url"}},
		{"ai: executor_config base_url warns with no step using the provider",
			unpairedHead + "executor_config:\n  cohere:\n    base_url: 8080\n  not_a_provider:\n    base_url: 'https://x.example.com'\nsteps:\n  s:\n    executor: function://text/template\n    query:\n      message: hi\n",
			[]string{"executor_config.cohere.base_url"}},
		{"ai: a templated executor_config base_url is still a literal to the ai arm",
			unpairedHead + "executor_config:\n  openai:\n    base_url: '{{ .query.u }}'\nsteps:\n  s:\n    executor: ai://openai/chat\n    query:\n      prompt: hi\n",
			[]string{"executor_config.openai.base_url"}},
		{"family: the step query's secret decides before executor_config's",
			unpairedHead + "executor_config:\n  deepr:\n    api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\nsteps:\n  s:\n    executor: deepr://deepr/stats\n    query:\n      api_key: 'mine'\n      base_url: 'https://evil.example.com'\n",
			nil},
		{"family: a templated executor_config endpoint is not certainly the author's",
			unpairedHead + "executor_config:\n  deepr:\n    api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\n    base_url: '{{ .query.u }}'\nsteps:\n  s:\n    executor: deepr://deepr/stats\n    query:\n      q: 1\n",
			nil},
		{"family: executor_config api_key is read only when the family copies it into a secret (s3 copies it into access_key_id)",
			unpairedHead + "executor_config:\n  s3:\n    api_key: '${AIGENTFLOW_STORAGE_S3_ACCESS_KEY}'\n    base_url: 'https://objects.example.com'\nsteps:\n  s:\n    executor: storage://s3/upload\n    query:\n      key: a\n",
			nil},
		{"family: ... and its base_url is read as the endpoint",
			unpairedHead + "executor_config:\n  s3:\n    base_url: 'https://objects.example.com'\nsteps:\n  s:\n    executor: storage://s3/upload\n    query:\n      secret_access_key: '${AIGENTFLOW_STORAGE_S3_SECRET_KEY}'\n",
			[]string{"executor_config.s3.base_url"}},
		{"family: an empty or numeric first endpoint falls through to the next",
			step("storage://s3/upload", "      secret_access_key: '${AIGENTFLOW_STORAGE_S3_SECRET_KEY}'\n      endpoint: 9\n      public_endpoint: 'https://pub.example.com'\n"),
			[]string{"steps.s.query.public_endpoint"}},
		{"family: the default by origin — userinfo, path, query, fragment and case do not matter",
			step("deepr://deepr/stats", "      api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\n      base_url: 'http://u:p@LOCALHOST:28081/x?y=1#z'\n"),
			nil},
		{"family: the default origin survives surrounding space",
			step("staticmap://geoapify/render", "      api_key: '${AIGENTFLOW_GEOAPIFY_API_KEY}'\n      base_url: '  https://MAPS.geoapify.com/  '\n"),
			nil},
		{"family: the default port is filled (http 80 is not the default's 8123)",
			step("homeassistant://api/get_states", "      token: '${AIGENTFLOW_HOMEASSISTANT_TOKEN}'\n      base_url: 'http://homeassistant.local'\n"),
			[]string{"steps.s.query.base_url"}},
		{"family: another scheme is another origin",
			step("homeassistant://api/get_states", "      token: '${AIGENTFLOW_HOMEASSISTANT_TOKEN}'\n      base_url: 'https://homeassistant.local:8123'\n"),
			[]string{"steps.s.query.base_url"}},
		{"family: a URL net/url refuses is compared as text, so it is no default",
			step("deepr://deepr/stats", "      api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\n      base_url: 'http://localhost:28081/%zz'\n"),
			[]string{"steps.s.query.base_url"}},
		{"family: a host:port with no scheme is no default",
			step("deepr://deepr/stats", "      api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\n      base_url: 'localhost:28081'\n"),
			[]string{"steps.s.query.base_url"}},
		{"family: an empty port is kept as written, so it is no default",
			step("deepr://deepr/stats", "      api_key: '${AIGENTFLOW_DEEPR_API_KEY}'\n      base_url: 'http://localhost:28081:'\n"),
			[]string{"steps.s.query.base_url"}},
		{"family: a reference to ANOTHER family's variable is no server secret here",
			step("db://weaviate/search", "      api_key: '${AIGENTFLOW_OPENAI_API_KEY}'\n      base_url: 'https://v.example.com'\n"),
			nil},
		{"family: the driver picks the row (storage/trove is implicit, never judged)",
			step("storage://trove/upload", "      trove_auth_token: '${AIGENTFLOW_TROVE_AUTH_TOKEN}'\n      trove_base_url: 'https://t.example.com'\n"),
			nil},
		{"family: an extra-block secret and endpoint on a loop sub-step",
			unpairedHead + "executor_config:\n  broker:\n    extra:\n      password: '${AIGENTFLOW_MQTT_PASSWORD}'\n      broker_url: 'tcp://evil.example.com:1883'\nsteps:\n  s:\n    loop:\n      while: '{{ false }}'\n      max_iterations: 1\n      steps:\n        - id: 7\n          executor: mqtt://broker/publish\n          query:\n            topic: t\n",
			[]string{"executor_config.broker.extra.broker_url"}},
		{"nexus: a null credentials entry and a numeric key are no key of its own",
			step("nexus://org/agent", "      credentials: ~\n      api_key: 42\n      base_url: 'https://chat.example.com'\n"),
			[]string{"steps.s.query.base_url"}},
		{"nexus: an empty credentials mapping is a key of its own",
			step("nexus://org/agent", "      credentials: {}\n      base_url: 'https://chat.example.com'\n"),
			nil},
		{"nexus: only the first endpoint warns",
			step("nexus://org/agent", "      aigentchat_base_url: 'https://a.example.com'\n      base_url: 'https://b.example.com'\n"),
			[]string{"steps.s.query.aigentchat_base_url"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unpairedFields(t, tc.src); !slices.Equal(got, tc.want) {
				t.Errorf("fields = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCredentialEndpointFamilyTableIsTheSpecData pins the shape of the family
// table: a row per family the reference declares, the two ai:// exemptions, and
// every row's parameter lists non-empty where the rule reads them.
func TestCredentialEndpointFamilyTableIsTheSpecData(t *testing.T) {
	cfg := spec.CredentialEndpointPairing
	var keys []string
	for k := range cfg.Families {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wantKeys := []string{"db", "deepr", "getmd", "homeassistant", "hosting", "mcp/http", "mqtt", "nexus",
		"opencorporates", "shelly", "staticmap", "storage/s3", "storage/trove"}
	if !slices.Equal(keys, wantKeys) {
		t.Fatalf("family rows = %v, want %v (a new family is a spec change)", keys, wantKeys)
	}
	if got := cfg.AI.KeylessProviders; !slices.Equal(got, []string{"ollama", "vllm"}) {
		t.Fatalf("keyless providers = %v", got)
	}
	for key, family := range cfg.Families {
		if len(family.EndpointParams) == 0 {
			t.Errorf("%s: no endpoint parameter", key)
		}
		if !strings.HasPrefix(key, family.Protocol) {
			t.Errorf("%s: row key does not start with its protocol %q", key, family.Protocol)
		}
		if family.ExpandsServerEnvReferences && len(family.ServerEnv) == 0 {
			t.Errorf("%s: expands references but names no server variable", key)
		}
	}
	if cfg.Families["nexus"].StoredKeyShape == nil {
		t.Error("nexus must carry the stored-key shape")
	}
}

// TestCredentialEndpointOrigin pins the origin reduction the default-endpoint
// comparison uses, which is the reference's credentialEndpointOrigin.
func TestCredentialEndpointOrigin(t *testing.T) {
	cases := map[string]string{
		"https://Example.COM/v1":                "https://example.com:443",
		"http://example.com":                    "http://example.com:80",
		"HTTP://example.com:8080/x":             "http://example.com:8080",
		"tcp://localhost:1883":                  "tcp://localhost:1883",
		"tcp://localhost":                       "tcp://localhost",
		"http://[::1]/x":                        "http://[::1]:80",
		"http://[::1]:9/x":                      "http://[::1]:9",
		" https://u:p@example.com/ ":            "https://example.com:443",
		"10.0.0.9":                              "10.0.0.9",
		"Host.Example//":                        "host.example",
		"http://example.com/%zz":                "http://example.com/%zz",
		"http://example.com:8080:":              "http://example.com:8080:",
		"mailto:someone@example.com":            "mailto:someone@example.com",
		"https://example.com#fragment":          "https://example.com:443",
		"https://example.com#%zz":               "https://example.com#%zz",
		"http://ex ample.com":                   "http://ex ample.com",
		"http://ex%41mple.com":                  "http://ex%41mple.com",
		"http://u ser@example.com":              "http://u ser@example.com",
		"://example.com":                        "://example.com",
		"http://example.com\u0001":              "http://example.com\u0001",
		"http://homeass\u0130stant.local:8123":  "http://homeassistant.local:8123",
		"http://homeass%C4%B0stant.local:8123":  "http://homeassistant.local:8123",
		"http://homeassistant.local:8123\u0085": "http://homeassistant.local:8123",
		"\ufeffhttp://homeassistant.local:8123": "\ufeffhttp://homeassistant.local:8123",
	}
	for in, want := range cases {
		if got := credentialEndpointOrigin(in); got != want {
			t.Errorf("credentialEndpointOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}
