package aifvalidate

import (
	"strings"
	"testing"
)

// Per-validator table tests. Each asserts the specific CODE on the specific
// FIELD path, because both are the contract a consumer builds on: the code
// drives behaviour, the field path drives the editor jump.

func TestBasicStructureRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		source string
		field  string
	}{
		{"no aigentflow_version", "name: f\nstart: a\nsteps:\n  a:\n    executor: function://n\n", keyAigentflowVersion},
		{"no name", "aigentflow_version: \"2.0.0\"\nstart: a\nsteps:\n  a:\n    executor: function://n\n", keyName},
		{"no start", "aigentflow_version: \"2.0.0\"\nname: f\nsteps:\n  a:\n    executor: function://n\n", keyStart},
		{"no steps", "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\n", keySteps},
		{"empty steps", "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps: {}\n", keySteps},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertError(t, ValidateFlow(tc.source, Options{}), codeMissingField, tc.field)
		})
	}
}

// TestStructuralChecksAreASupersetOfTheFourKeyGate locks the fact aigentverse's
// DC101 adoption depends on: this validator's findings cover — with the same
// field attribution — every check aiv's own four-key gate used to make, so that
// gate can be deleted rather than run alongside and double-report.
func TestStructuralChecksAreASupersetOfTheFourKeyGate(t *testing.T) {
	// start present but not a defined step: must be step_not_found on `start`,
	// NOT a generic missing-field.
	result := ValidateFlow("aigentflow_version: \"2.0.0\"\nname: f\nstart: nope\nsteps:\n  a:\n    executor: function://n\n", Options{})
	assertError(t, result, codeStepNotFound, keyStart)
	if hasCode(result.Errors, codeMissingField) {
		t.Errorf("a defined-but-unknown start must not also report missing_required_field: %s",
			formatIssues(result.Errors))
	}
}

func TestBasicStructureStepShape(t *testing.T) {
	t.Run("step id containing a dot is reserved", func(t *testing.T) {
		src := "aigentflow_version: \"2.0.0\"\nname: f\nstart: a.b\nsteps:\n  a.b:\n    executor: function://n\n"
		assertError(t, ValidateFlow(src, Options{}), codeReservedStepID, "steps.a.b")
	})
	t.Run("step missing an executor", func(t *testing.T) {
		src := "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a: {}\n"
		assertError(t, ValidateFlow(src, Options{}), codeMissingField, "steps.a.executor")
	})
	t.Run("non-string executor", func(t *testing.T) {
		src := "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: 42\n"
		assertError(t, ValidateFlow(src, Options{}), codeInvalidType, "steps.a.executor")
	})
	t.Run("a loop step needs no executor", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    loop:
      while: "{{ lt .index 3 }}"
      max_iterations: 3
      steps:
        - id: inner
          executor: function://n
`
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeMissingField) {
			t.Errorf("a loop step must not require its own executor: %s", formatIssues(result.Errors))
		}
	})
}

func TestExecutorURIs(t *testing.T) {
	t.Run("malformed URI is an error", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "function://noop", "notauri", 1)
		assertError(t, ValidateFlow(src, Options{}), codeInvalidExecutorURL, "steps.only.executor")
	})
	t.Run("empty path is an error", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "function://noop", "function://", 1)
		assertError(t, ValidateFlow(src, Options{}), codeInvalidExecutorURL, "steps.only.executor")
	})
	t.Run("unknown scheme is a WARNING, still valid", func(t *testing.T) {
		// The load-bearing classification: the vendored scheme list lags the live
		// registry, so an unknown scheme must never block a publish.
		src := strings.Replace(minimalFlow, "function://noop", "aif://nope", 1)
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeUnknownExecScheme)
	})
	t.Run("every spec scheme is accepted", func(t *testing.T) {
		for _, scheme := range ExecutorSchemes() {
			src := strings.Replace(minimalFlow, "function://noop", scheme+"://x/y", 1)
			result := ValidateFlow(src, Options{})
			if hasCode(result.Warnings, codeUnknownExecScheme) || hasCode(result.Errors, codeInvalidExecutorURL) {
				t.Errorf("scheme %q from the spec was not accepted: %s / %s",
					scheme, formatIssues(result.Errors), formatIssues(result.Warnings))
			}
		}
	})
}

func TestConnectivity(t *testing.T) {
	t.Run("dangling next.default", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      default: ghost
`
		assertError(t, ValidateFlow(src, Options{}), codeStepNotFound, "steps.a.next.default")
	})
	t.Run("dangling conditional goto", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      conditions:
        - if: "{{ eq .a 1 }}"
          goto: ghost
`
		assertError(t, ValidateFlow(src, Options{}), codeStepNotFound, "steps.a.next.conditions[0].goto")
	})
	t.Run("goto_step under a condition is the wrong key (AIgentFlow refuses it)", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      conditions:
        - if: "{{ eq .a 1 }}"
          goto_step: b
  b:
    executor: function://n
`
		assertError(t, ValidateFlow(src, Options{}), codeUnknownYAMLKey, "steps.a.next.conditions[0].goto_step")
	})
	t.Run("terminal markers are not step references", func(t *testing.T) {
		for _, marker := range spec.NextMarkers {
			src := "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: function://n\n    next:\n      default: " +
				marker + "\n"
			if marker == nextMarkerOrch {
				continue // routing to the orchestrator needs an orchestrator block
			}
			result := ValidateFlow(src, Options{})
			if hasCode(result.Errors, codeStepNotFound) {
				t.Errorf("marker %q must not be treated as a step reference: %s", marker, formatIssues(result.Errors))
			}
		}
	})
	t.Run("unreachable step is a warning", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
  orphan:
    executor: function://n
`
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeUnreachableStep)
	})
	t.Run("cycle is a warning", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      default: b
  b:
    executor: function://n
    next:
      default: a
`
		assertWarningNotError(t, ValidateFlow(src, Options{}), codePotentialInfiniteLop)
	})
}

func TestNextParallel(t *testing.T) {
	base := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      parallel:
%s
  worker:
    executor: function://n
  join:
    executor: function://n
`
	t.Run("missing rendezvous", func(t *testing.T) {
		src := sprintfYAML(base, "        steps: [worker]")
		assertError(t, ValidateFlow(src, Options{}), codeMissingField, "steps.a.next.parallel.rendezvous")
	})
	t.Run("unknown rendezvous", func(t *testing.T) {
		src := sprintfYAML(base, "        rendezvous: ghost\n        steps: [worker]")
		assertError(t, ValidateFlow(src, Options{}), codeStepNotFound, "steps.a.next.parallel.rendezvous")
	})
	t.Run("rendezvous null is allowed", func(t *testing.T) {
		src := sprintfYAML(base, "        rendezvous: \"null\"\n        steps: [worker]")
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeStepNotFound) {
			t.Errorf("rendezvous 'null' must be accepted: %s", formatIssues(result.Errors))
		}
	})
	t.Run("empty steps list", func(t *testing.T) {
		src := sprintfYAML(base, "        rendezvous: join\n        steps: []")
		assertError(t, ValidateFlow(src, Options{}), codeMissingField, "steps.a.next.parallel.steps")
	})
	t.Run("unknown parallel member", func(t *testing.T) {
		src := sprintfYAML(base, "        rendezvous: join\n        steps: [ghost]")
		assertError(t, ValidateFlow(src, Options{}), codeStepNotFound, "steps.a.next.parallel.steps[0]")
	})
}

func TestOrchestratorNextRequiresOrchestratorBlock(t *testing.T) {
	src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      default: orchestrator
`
	assertError(t, ValidateFlow(src, Options{}), codeOrchNextRequiresOrch, "steps.a.next")
}

func TestErrorStrategy(t *testing.T) {
	wrap := func(body string) string {
		return "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: function://n\n    error_strategy:\n" + body
	}
	t.Run("unknown action", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      action: explode\n"), Options{}),
			codeInvalidErrStrategy, "steps.a.error_strategy.action")
	})
	t.Run("goto without goto_step", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      action: goto\n"), Options{}),
			codeGotoStepMissing, "steps.a.error_strategy.goto_step")
	})
	t.Run("goto to an unknown step", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      action: goto\n      goto_step: ghost\n"), Options{}),
			codeStepNotFound, "steps.a.error_strategy.goto_step")
	})
	t.Run("malformed max_delay is an error", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      max_delay: 5 minutes\n"), Options{}),
			codeInvalidDuration, "steps.a.error_strategy.max_delay")
	})
	t.Run("malformed retry_delay is only a warning", func(t *testing.T) {
		// Asymmetric on purpose (mirrors the reference): a bad max_delay disables
		// the backoff ceiling, a bad retry_delay falls back to a default.
		assertWarningNotError(t, ValidateFlow(wrap("      retry_delay: 5 minutes\n"), Options{}),
			codeInvalidDuration)
	})
	t.Run("valid Go durations pass", func(t *testing.T) {
		for _, d := range []string{"0", "1ns", "500ms", "1.5s", "2m", "1h30m", "-5s"} {
			result := ValidateFlow(wrap("      max_delay: \""+d+"\"\n"), Options{})
			if hasCode(result.Errors, codeInvalidDuration) {
				t.Errorf("duration %q must be accepted: %s", d, formatIssues(result.Errors))
			}
		}
	})
	t.Run("non-positive backoff_multiplier", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      backoff_multiplier: 0\n"), Options{}),
			codeInvalidBackoffMult, "steps.a.error_strategy.backoff_multiplier")
	})
	t.Run("unknown retry_on category", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      retry_on: [nonsense]\n"), Options{}),
			codeInvalidRetryOnCategry, "steps.a.error_strategy.retry_on[0]")
	})
}

func TestLoopAndForEach(t *testing.T) {
	t.Run("loop max_iterations over the limit", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    loop:
      while: "{{ true }}"
      max_iterations: 100000
      steps:
        - id: inner
          executor: function://n
`
		assertError(t, ValidateFlow(src, Options{}), codeLoopMaxIterRange, "steps.a.loop.max_iterations")
	})
	t.Run("duplicate loop sub-step id", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    loop:
      while: "{{ true }}"
      max_iterations: 3
      steps:
        - id: inner
          executor: function://n
        - id: inner
          executor: function://n
`
		assertError(t, ValidateFlow(src, Options{}), codeLoopStepIDDuplicate, "steps.a.loop.steps[1].id")
	})
	t.Run("loop and its own executor are mutually exclusive", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    loop:
      while: "{{ true }}"
      max_iterations: 3
      steps:
        - id: inner
          executor: function://n
`
		assertError(t, ValidateFlow(src, Options{}), codeLoopMutualExclExec, "steps.a.executor")
	})
	t.Run("for_each without items", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    for_each:
      as: item
`
		assertError(t, ValidateFlow(src, Options{}), codeForEachItemsRequired, "steps.a.for_each.items")
	})
	t.Run("throttle batch_delay without batch_size", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    for_each:
      items: "{{ .query.list }}"
      throttle:
        batch_delay: 10s
`
		assertError(t, ValidateFlow(src, Options{}),
			codeThrottleBatchNoSize, "steps.a.for_each.throttle.batch_delay")
	})
	t.Run("throttle delay over the 5m ceiling", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    for_each:
      items: "{{ .query.list }}"
      throttle:
        delay: 10m
`
		assertError(t, ValidateFlow(src, Options{}),
			codeThrottleDelayMax, "steps.a.for_each.throttle.delay")
	})
}

func TestCredentialBindings(t *testing.T) {
	t.Run("credential and credentials are mutually exclusive", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    credential: stored/openai/main
    credentials:
      api:
        source: stored/openai/main
        inject_as: Authorization
`
		assertError(t, ValidateFlow(src, Options{}), codeCredMutualExclusive, "steps.a.credential")
	})
	t.Run("shorthand without the stored/ prefix", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    credential: vault/openai/main", 1)
		assertError(t, ValidateFlow(src, Options{}), codeCredShorthandSource, "steps.only.credential")
	})
	t.Run("shorthand missing the name segment", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    credential: stored/openai", 1)
		assertError(t, ValidateFlow(src, Options{}), codeCredShorthandFormat, "steps.only.credential")
	})
	t.Run("binding without inject_as", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    credentials:
      api:
        source: stored/openai/main
`
		assertError(t, ValidateFlow(src, Options{}),
			codeCredInjectAsEmpty, "steps.a.credentials.api.inject_as")
	})
}

func TestInputSchema(t *testing.T) {
	wrap := func(fields string) string {
		return "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: function://n\ninput_schema:\n  version: 1\n  fields:\n" + fields
	}
	t.Run("wrong schema version", func(t *testing.T) {
		src := "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: function://n\ninput_schema:\n  version: 99\n  fields: []\n"
		assertError(t, ValidateFlow(src, Options{}), codeISInvalidVersion, "input_schema.version")
	})
	t.Run("invalid field name", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("    - name: NotSnake\n      type: string\n"), Options{}),
			codeISInvalidFieldName, "input_schema.fields[0]")
	})
	t.Run("duplicate field name", func(t *testing.T) {
		src := wrap("    - name: a\n      type: string\n    - name: a\n      type: string\n")
		assertError(t, ValidateFlow(src, Options{}), codeISDuplicateFieldName, "input_schema.fields[1]")
	})
	t.Run("unknown field type", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("    - name: a\n      type: quaternion\n"), Options{}),
			codeISUnknownType, "input_schema.fields[0].type")
	})
	t.Run("empty enum", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("    - name: a\n      type: enum\n      enum: []\n"), Options{}),
			codeISEnumEmpty, "input_schema.fields[0].enum")
	})
	t.Run("constraint not meaningful for the type", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("    - name: a\n      type: bool\n      min_length: 3\n"), Options{}),
			codeISConstraintMismatch, "input_schema.fields[0].min_length")
	})
	t.Run("min greater than max", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("    - name: a\n      type: number\n      min: 10\n      max: 2\n"), Options{}),
			codeISInvalidRange, "input_schema.fields[0]")
	})
	t.Run("invalid RE2 pattern", func(t *testing.T) {
		// Go's regexp is what the engine uses, so this side is authoritative.
		assertError(t, ValidateFlow(wrap("    - name: a\n      type: string\n      pattern: \"a(\"\n"), Options{}),
			codeISInvalidPattern, "input_schema.fields[0].pattern")
	})
	t.Run("visible_when needing exactly one predicate", func(t *testing.T) {
		src := wrap("    - name: a\n      type: bool\n    - name: b\n      type: string\n      visible_when:\n        field: a\n")
		assertError(t, ValidateFlow(src, Options{}),
			codeISVisibleWhenNoPred, "input_schema.fields[1].visible_when")
	})
	t.Run("visible_when referencing an unknown field", func(t *testing.T) {
		src := wrap("    - name: a\n      type: string\n      visible_when:\n        field: ghost\n        equals: true\n")
		assertError(t, ValidateFlow(src, Options{}),
			codeISVisibleWhenUnknown, "input_schema.fields[0].visible_when.field")
	})
	t.Run("visible_when may reference a LATER field", func(t *testing.T) {
		// Forward references are legal: the index is built before the check.
		src := wrap("    - name: a\n      type: string\n      visible_when:\n        field: b\n        equals: true\n    - name: b\n      type: bool\n")
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeISVisibleWhenUnknown) {
			t.Errorf("a forward visible_when reference must resolve: %s", formatIssues(result.Errors))
		}
	})
	t.Run("file after parametric is a warning", func(t *testing.T) {
		src := wrap("    - name: a\n      type: number\n    - name: b\n      type: file\n")
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeISFileAfterParam)
	})
}

func TestQualityGate(t *testing.T) {
	wrap := func(gate string) string {
		return "aigentflow_version: \"2.0.0\"\nname: f\nstart: a\nsteps:\n  a:\n    executor: ai://chat\n    quality_gate:\n" + gate + "  b:\n    executor: function://n\n"
	}
	t.Run("missing rubric", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      threshold: 0.8\n"), Options{}),
			codeQGMissingRubric, "steps.a.quality_gate.rubric")
	})
	t.Run("whitespace-only rubric counts as missing", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      rubric: \"   \"\n"), Options{}),
			codeQGMissingRubric, "steps.a.quality_gate.rubric")
	})
	t.Run("threshold out of range", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      rubric: good\n      threshold: 1.5\n"), Options{}),
			codeQGThresholdRange, "steps.a.quality_gate.threshold")
	})
	t.Run("on_fail human is rejected under its own code", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      rubric: good\n      on_fail: human\n"), Options{}),
			codeQGOnFailUnsupported, "steps.a.quality_gate.on_fail")
	})
	t.Run("on_fail goto pointing at itself", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      rubric: good\n      on_fail: goto\n      goto_step: a\n"), Options{}),
			codeQGGotoSelf, "steps.a.quality_gate.goto_step")
	})
	t.Run("on_fail goto to an unknown step", func(t *testing.T) {
		assertError(t, ValidateFlow(wrap("      rubric: good\n      on_fail: goto\n      goto_step: ghost\n"), Options{}),
			codeStepNotFound, "steps.a.quality_gate.goto_step")
	})
	t.Run("gate on a composite step", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: ai://chat
    for_each:
      items: "{{ .query.list }}"
    quality_gate:
      rubric: good
`
		assertError(t, ValidateFlow(src, Options{}), codeQGOnComposite, "steps.a.quality_gate")
	})
}

func TestOrchestratorAndCampaign(t *testing.T) {
	t.Run("orchestrator without exons", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  agentic: true\n"
		assertError(t, ValidateFlow(src, Options{}), codeOrchExonsRequired, "orchestrator.exons")
	})
	t.Run("invalid mode", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"---\\nname: o\\n---\\n\"\n  mode: dictator\n"
		assertError(t, ValidateFlow(src, Options{}), codeOrchModeInvalid, "orchestrator.mode")
	})
	t.Run("owner mode without a yield edge", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"---\\nname: o\\n---\\n\"\n  mode: owner\n"
		assertError(t, ValidateFlow(src, Options{}), codeOrchOwnerNeedsYield, "orchestrator.mode")
	})
	t.Run("owner mode WITH a yield edge is valid", func(t *testing.T) {
		src := `aigentflow_version: "2.0.0"
name: f
start: a
steps:
  a:
    executor: function://n
    next:
      default: orchestrator
orchestrator:
  exons: "---\nname: o\n---\n"
  mode: owner
`
		result := ValidateFlow(src, Options{})
		if hasCode(result.Errors, codeOrchOwnerNeedsYield) {
			t.Errorf("an explicit yield edge must satisfy owner mode: %s", formatIssues(result.Errors))
		}
	})
	t.Run("timer trigger without an interval", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"x\"\n  triggers:\n    - type: timer\n"
		assertError(t, ValidateFlow(src, Options{}),
			codeOrchTimerNoInterval, "orchestrator.triggers[0].interval")
	})
	t.Run("unknown trigger type", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"x\"\n  triggers:\n    - type: eclipse\n"
		assertError(t, ValidateFlow(src, Options{}),
			codeOrchTriggerUnknown, "orchestrator.triggers[0].type")
	})
	t.Run("unknown tool warns by default, errors under StrictRegistries", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"x\"\n  tools: [not_a_real_tool]\n"
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeOrchToolUnknown)
		assertError(t, ValidateFlow(src, Options{StrictRegistries: true}),
			codeOrchToolUnknown, "orchestrator.tools[0]")
	})
	t.Run("campaign without an orchestrator", func(t *testing.T) {
		src := minimalFlow + "campaign:\n  spawn: agents\n"
		assertError(t, ValidateFlow(src, Options{}), codeCampaignNeedsOrch, keyCampaign)
	})
	t.Run("campaign handoff to an unknown step", func(t *testing.T) {
		src := minimalFlow + "orchestrator:\n  exons: \"x\"\ncampaign:\n  on_children_complete: ghost\n"
		assertError(t, ValidateFlow(src, Options{}),
			codeCampaignHandoffStep, "campaign.on_children_complete")
	})
}

func TestQueryAndResponseExpectation(t *testing.T) {
	t.Run("query param without a type", func(t *testing.T) {
		src := minimalFlow + "query:\n  topic:\n    description: a topic\n"
		assertError(t, ValidateFlow(src, Options{}), codeQueryParamTypeMissing, "query.topic.type")
	})
	t.Run("unknown top-level query type is a warning", func(t *testing.T) {
		src := minimalFlow + "query:\n  topic:\n    type: quaternion\n"
		assertWarningNotError(t, ValidateFlow(src, Options{}), codeUnknownDataType)
	})
	t.Run("array query param without items", func(t *testing.T) {
		src := minimalFlow + "query:\n  list:\n    type: array\n"
		assertError(t, ValidateFlow(src, Options{}), codeArrayItemsMissing, "query.list")
	})
	t.Run("array item type IS constrained", func(t *testing.T) {
		src := minimalFlow + "query:\n  list:\n    type: array\n    items:\n      type: quaternion\n"
		assertError(t, ValidateFlow(src, Options{}), codeArrayItemsTypeInvalid, "query.list.items.type")
	})
	t.Run("max_items below min_items", func(t *testing.T) {
		src := minimalFlow + "query:\n  list:\n    type: array\n    items:\n      type: string\n    min_items: 5\n    max_items: 2\n"
		assertError(t, ValidateFlow(src, Options{}), codeArrayMaxItemsInvalid, "query.list.max_items")
	})
	t.Run("response_expectation with an unknown type", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    response_expectation:\n      out:\n        type: quaternion", 1)
		assertError(t, ValidateFlow(src, Options{}),
			codeInvalidDataType, "steps.only.response_expectation.out.type")
	})
	t.Run("response_expectation array without items", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    response_expectation:\n      out:\n        type: array", 1)
		assertError(t, ValidateFlow(src, Options{}),
			codeRespExpArrayItems, "steps.only.response_expectation.out.items")
	})
	t.Run("response_expectation required accepts a bool or a template string", func(t *testing.T) {
		for _, value := range []string{"true", "\"{{ .query.flag }}\""} {
			src := strings.Replace(minimalFlow, "    executor: function://noop",
				"    executor: function://noop\n    response_expectation:\n      out:\n        type: string\n        required: "+value, 1)
			result := ValidateFlow(src, Options{})
			if hasCode(result.Errors, codeInvalidValue) {
				t.Errorf("required: %s must be accepted: %s", value, formatIssues(result.Errors))
			}
		}
	})
	t.Run("response_expectation required rejects other shapes", func(t *testing.T) {
		src := strings.Replace(minimalFlow, "    executor: function://noop",
			"    executor: function://noop\n    response_expectation:\n      out:\n        type: string\n        required: [1]", 1)
		assertError(t, ValidateFlow(src, Options{}),
			codeInvalidValue, "steps.only.response_expectation.out.required")
	})
}

func TestExpressionFunctions(t *testing.T) {
	t.Run("not a list", func(t *testing.T) {
		assertError(t, ValidateFlow(minimalFlow+"expression_functions: nope\n", Options{}),
			codeInvalidType, keyExpressionFunctions)
	})
	t.Run("two keys in one entry", func(t *testing.T) {
		src := minimalFlow + "expression_functions:\n  - package: p\n    function: f\n"
		assertError(t, ValidateFlow(src, Options{}), codeInvalidExprFunction, "expression_functions[0]")
	})
	t.Run("wrong key", func(t *testing.T) {
		src := minimalFlow + "expression_functions:\n  - module: p\n"
		assertError(t, ValidateFlow(src, Options{}), codeInvalidExprFunction, "expression_functions[0]")
	})
	t.Run("empty value", func(t *testing.T) {
		src := minimalFlow + "expression_functions:\n  - package: \"\"\n"
		assertError(t, ValidateFlow(src, Options{}),
			codeInvalidExprFunction, "expression_functions[0].package")
	})
	t.Run("valid entries", func(t *testing.T) {
		src := minimalFlow + "expression_functions:\n  - package: sprig\n  - function: upper\n"
		assertValid(t, ValidateFlow(src, Options{}))
	})
}

// sprintfYAML substitutes a single %s in a YAML template. A helper rather than
// fmt.Sprintf directly so the test bodies read as YAML, not as format strings.
func sprintfYAML(template, body string) string {
	return strings.Replace(template, "%s", body, 1)
}

// aigentflow#149: a parallel fan-out, its rendezvous, a conditional branch and
// both error redirects are REACHABLE. v0.1.0 reported every one of them "not
// reachable from start step" (the walk followed next.default and a condition
// key AIgentFlow does not have). Each step here is reachable by exactly one edge
// kind, so removing any kind from the walk turns exactly one warning on.
func TestReachabilityFollowsEveryEdgeKind(t *testing.T) {
	src := `aigentflow_version: "2.0.0"
name: f
start: fan
error_strategy:
  action: goto
  goto_step: flow_handler
steps:
  fan:
    executor: function://n
    next:
      parallel:
        steps: [v1, v2]
        rendezvous: quorum
  v1:
    executor: function://n
  v2:
    executor: function://n
  quorum:
    executor: function://n
    error_strategy:
      action: goto
      goto_step: step_handler
    next:
      conditions:
        - if: "{{ eq .data.quorum.ok true }}"
          goto: conditional
      default: "end"
  conditional:
    executor: function://n
  step_handler:
    executor: function://n
  flow_handler:
    executor: function://n
`
	res := ValidateFlow(src, Options{})
	for _, w := range res.Warnings {
		if w.Code == codeUnreachableStep {
			t.Errorf("step %q reported unreachable: %s", w.StepID, w.Message)
		}
	}
	// Control: an orphan IS reported, so the check still runs.
	orphan := ValidateFlow(src+`  orphan:
    executor: function://n
`, Options{})
	found := false
	for _, w := range orphan.Warnings {
		if w.Code == codeUnreachableStep && w.StepID == "orphan" {
			found = true
		}
	}
	if !found {
		t.Error("control: a step with no edge into it must still be reported unreachable")
	}
}

