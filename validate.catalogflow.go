package aifvalidate

import (
	"fmt"
	"strings"
)

// Catalogue sub-flows and partials (v0.26.0, AIgentFlow 2.912.0; the reference's
// validateCatalogFlowSteps in validation.catalogflow.go).
//
// flow://aiv/<org>/<name> runs a published catalogue flow as a sub-flow;
// include://aiv/<org>/<name> is replaced by a published flow's steps before the flow
// runs. Both take exactly one org and one name segment. flow_version pins the
// published version (strict semver) and belongs only on such a step. A templated URL
// is judged when it is rendered, so it is skipped here. Every finding is an ERROR,
// in a loop body too.

const keyFlowVersion = "flow_version"

func validateCatalogFlowSteps(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		checkCatalogSite(step, stepField(stepID), stepID, iss)
		loop, hasLoop := getRecord(step, keyLoop)
		if !hasLoop {
			continue
		}
		subs, hasSubs := getSlice(loop, keySteps)
		if !hasSubs {
			continue
		}
		for i, raw := range subs {
			sub, ok := asRecord(raw)
			if !ok {
				continue
			}
			checkCatalogSite(sub, stepField(stepID, keyLoop, indexed(keySteps, i)), subStepID(stepID, sub, i, iss), iss)
		}
	}
}

func checkCatalogSite(site doc, path, stepID string, iss *issues) {
	executor, _ := getString(site, keyExecutor)
	prefix := catalogPrefix(executor)
	report := func(rule atlasRule, field string, args ...any) {
		iss.error(Issue{
			Field: field, Code: rule.Code, StepID: stepID,
			Message:    fmt.Sprintf(rule.MessageFormat, append([]any{stepID}, args...)...),
			Suggestion: rule.Suggestion,
		})
	}
	if prefix != "" && !strings.Contains(executor, templateOpenDelim) && !validCatalogRef(strings.TrimPrefix(executor, prefix)) {
		scheme, _, _ := strings.Cut(executor, schemeSeparator)
		report(spec.CatalogFlow.RefInvalid, path+"."+keyExecutor, executor, scheme)
	}
	raw, present := site[keyFlowVersion]
	if !present || raw == nil {
		return
	}
	version := scalarString(raw)
	if version == "" {
		return
	}
	if prefix == "" {
		report(spec.CatalogFlow.VersionPlacement, path+"."+keyFlowVersion, executor)
		return
	}
	if !catalogVersionRe.MatchString(version) {
		report(spec.CatalogFlow.VersionInvalid, path+"."+keyFlowVersion, version)
	}
}

func catalogPrefix(executor string) string {
	for _, prefix := range spec.CatalogFlow.RefPrefixes {
		if strings.HasPrefix(executor, prefix) {
			return prefix
		}
	}
	return ""
}

func validCatalogRef(resource string) bool {
	parts := strings.Split(resource, "/")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if !catalogSegmentRe.MatchString(p) {
			return false
		}
	}
	return true
}

// scalarString renders a YAML scalar as its text (a version may decode as a number).
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}
