package aifvalidate

import (
	"fmt"
	"strings"
)

// validateCredentialBindings checks step credential bindings.
//
// Mirrors validateStepCredentialBindings (parser.go): `credentials` and
// `credential` are mutually exclusive; every source must be
// `stored/{provider}/{name}` with a non-empty provider AND name; an explicit
// binding requires a non-empty `inject_as`. Applies to loop sub-steps too.
//
// This validates the REFERENCE FORM only. It never reads, resolves, or
// transports a secret — a static validator has nothing to resolve against.
func validateCredentialBindings(flow doc, iss *issues) {
	steps := stepsOf(flow)
	if steps == nil {
		return
	}
	for _, stepID := range sortedKeys(steps) {
		step, ok := asRecord(steps[stepID])
		if !ok {
			continue
		}
		checkCredentialBindings(step, stepField(stepID, keyCredential), stepID, iss)

		loop, hasLoop := getRecord(step, keyLoop)
		if !hasLoop {
			continue
		}
		subs, hasSubs := getSlice(loop, keySteps)
		if !hasSubs {
			continue
		}
		for i, raw := range subs {
			sub, isMap := asRecord(raw)
			if !isMap {
				continue
			}
			checkCredentialBindings(sub,
				stepField(stepID, keyLoop, indexed(keySteps, i), keyCredential),
				subStepID(stepID, sub, i), iss)
		}
	}
}

func checkCredentialBindings(owner doc, fieldBase, stepID string, iss *issues) {
	credentialsMap, hasMap := getRecord(owner, keyCredentials)
	shorthand, hasShorthandKey := getString(owner, keyCredential)
	hasCredentialsMap := hasMap && len(credentialsMap) > 0
	hasShorthand := hasShorthandKey && shorthand != ""

	if hasCredentialsMap && hasShorthand {
		iss.error(Issue{
			Field: fieldBase, Code: codeCredMutualExclusive, StepID: stepID,
			Message: fmt.Sprintf("Step '%s' cannot set both 'credentials' and 'credential'", stepID),
		})
	}

	if hasMap {
		// The `credentials` map's field path is the shorthand path plus "s" — the
		// JS implementation composes it the same way, so a consumer highlighting
		// either implementation's findings resolves the same path.
		mapBase := fieldBase + "s"
		for _, bindingName := range sortedKeys(credentialsMap) {
			raw := credentialsMap[bindingName]
			if raw == nil {
				continue
			}
			binding, isMap := asRecord(raw)
			if !isMap {
				iss.error(Issue{
					Field: mapBase + "." + bindingName, Code: codeInvalidType, StepID: stepID,
					Message: fmt.Sprintf("Credential binding '%s' must be a mapping", bindingName),
				})
				continue
			}
			source, _ := getString(binding, keySource)
			switch {
			case !hasCredentialPrefix(source):
				iss.error(Issue{
					Field: mapBase + "." + bindingName + "." + keySource,
					Code:  codeCredInvalidSource, StepID: stepID,
					Message: fmt.Sprintf("Credential binding '%s' source must start with '%s'",
						bindingName, spec.CredentialRefPrefix),
				})
			case !hasValidCredentialFormat(source):
				iss.error(Issue{
					Field: mapBase + "." + bindingName + "." + keySource,
					Code:  codeCredSourceFormat, StepID: stepID,
					Message: fmt.Sprintf("Credential binding '%s' source must be '%s{provider}/{name}'",
						bindingName, spec.CredentialRefPrefix),
				})
			}
			if !isNonEmptyString(get(binding, keyInjectAs)) {
				iss.error(Issue{
					Field: mapBase + "." + bindingName + "." + keyInjectAs,
					Code:  codeCredInjectAsEmpty, StepID: stepID,
					Message: fmt.Sprintf("Credential binding '%s' requires a non-empty 'inject_as'", bindingName),
				})
			}
		}
	}

	if hasShorthand {
		switch {
		case !hasCredentialPrefix(shorthand):
			iss.error(Issue{
				Field: fieldBase, Code: codeCredShorthandSource, StepID: stepID,
				Message: fmt.Sprintf("Shorthand credential must start with '%s'", spec.CredentialRefPrefix),
			})
		case !hasValidCredentialFormat(shorthand):
			iss.error(Issue{
				Field: fieldBase, Code: codeCredShorthandFormat, StepID: stepID,
				Message: fmt.Sprintf("Shorthand credential must be '%s{provider}/{name}'",
					spec.CredentialRefPrefix),
			})
		}
	}
}

func hasCredentialPrefix(source string) bool {
	return len(source) > len(spec.CredentialRefPrefix) &&
		strings.HasPrefix(source, spec.CredentialRefPrefix)
}

// hasValidCredentialFormat mirrors Go's isValidCredentialSourceFormat: the first
// '/' after the prefix must have a non-empty provider before it and a non-empty
// name after it.
func hasValidCredentialFormat(source string) bool {
	rest := source[len(spec.CredentialRefPrefix):]
	idx := strings.Index(rest, "/")
	return idx > 0 && idx < len(rest)-1
}
