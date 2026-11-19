package aifvalidate

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Assets (v0.27.0, AIgentFlow 2.913.0; the reference's validateAssets in
// validation.assets.go and AssetDeclarationProblem in flow.assets.go).
//
// A flow-level assets: block names long step bodies kept outside the flow. Each
// asset sets exactly one source with its pin; a step string value that is exactly
// asset://<name> (one token, no whitespace) is replaced by that text before the flow
// runs, and only the step's query, its quality_gate rubric and a loop sub-step's
// query may hold one. Whether a remote asset exists and matches its sha256 is the
// reference server's check, not a save-door rule.

const (
	keyAssets   = "assets"
	keyContent  = "content"
	keyTroveKey = "trove_key"
	keyAivRef   = "aiv_ref"
	keySHA256   = "sha256"
)

// assetReferenceName returns the name a string value references, or false.
func assetReferenceName(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, spec.AssetGrammar.ReferencePrefix) {
		return "", false
	}
	name := strings.TrimPrefix(s, spec.AssetGrammar.ReferencePrefix)
	if name == "" || strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return "", false
	}
	return name, true
}

// collectAssetRefs adds every reference among v's string values (never map keys).
func collectAssetRefs(v any, into map[string]bool) {
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			collectAssetRefs(item, into)
		}
	default:
		if m, ok := asRecord(v); ok {
			for _, value := range m {
				collectAssetRefs(value, into)
			}
			return
		}
		if name, ok := assetReferenceName(v); ok {
			into[name] = true
		}
	}
}

// assetRefsOutsideSites lists the references a step makes outside its query, its
// quality_gate rubric and its loop sub-steps' queries.
func assetRefsOutsideSites(step doc) map[string]bool {
	out := map[string]bool{}
	for key, value := range step {
		switch key {
		case keyQuery:
			continue
		case keyQualityGate:
			if gate, ok := asRecord(value); ok {
				for gk, gv := range gate {
					if gk != keyRubric {
						collectAssetRefs(gv, out)
					}
				}
				continue
			}
		case keyLoop:
			if loop, ok := asRecord(value); ok {
				for lk, lv := range loop {
					subs, isList := lv.([]any)
					if lk != keySteps || !isList {
						collectAssetRefs(lv, out)
						continue
					}
					for _, raw := range subs {
						sub, ok := asRecord(raw)
						if !ok {
							collectAssetRefs(raw, out)
							continue
						}
						for sk, sv := range sub {
							if sk != keyQuery {
								collectAssetRefs(sv, out)
							}
						}
					}
				}
				continue
			}
		}
		collectAssetRefs(value, out)
	}
	return out
}

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// assetDeclarationProblem mirrors the reference's AssetDeclarationProblem, in its order.
func assetDeclarationProblem(name string, raw any, path string, iss *issues) string {
	p := spec.AssetGrammar.DeclarationInvalid.Problems
	if !assetNameRe.MatchString(name) {
		return fmt.Sprintf(p.Name, name)
	}
	asset, ok := asRecord(raw)
	if !ok {
		return p.NoSource
	}
	field := func(k string) string {
		s, _ := iss.stringOf(asset, k, path)
		return s
	}
	content, trove, aiv := field(keyContent), field(keyTroveKey), field(keyAivRef)
	version, sha := field(keyVersion), field(keySHA256)
	sources := 0
	for _, s := range []string{content, trove, aiv} {
		if s != "" {
			sources++
		}
	}
	switch {
	case sources == 0:
		return p.NoSource
	case sources > 1:
		return p.ManySources
	case content != "" && (sha != "" || version != ""):
		return p.InlinePin
	case len(content) > spec.AssetGrammar.MaxBytes:
		// Go string length is the UTF-8 byte length, as the reference measures it.
		return fmt.Sprintf(p.InlineTooLarge, spec.AssetGrammar.MaxBytes)
	case content == "" && !assetSHA256Re.MatchString(sha):
		return p.SHA256
	case trove != "" && version != "":
		return p.TroveVersion
	case aiv != "" && !catalogVersionRe.MatchString(version):
		return p.AivVersion
	}
	if aiv != "" && !validCatalogRef(aiv) {
		return p.AivRef
	}
	return ""
}

func validateAssets(flow doc, iss *issues) {
	declared, _ := getRecord(flow, keyAssets)
	rule := spec.AssetGrammar.DeclarationInvalid
	if len(declared) > spec.AssetGrammar.MaxCount {
		iss.error(Issue{Field: keyAssets, Code: rule.Code,
			Message: fmt.Sprintf(rule.TooManyFormat, len(declared), spec.AssetGrammar.MaxCount), Suggestion: rule.Suggestion})
	}
	for _, name := range sortedKeys(declared) {
		path := keyAssets + "." + name
		if problem := assetDeclarationProblem(name, declared[name], path, iss); problem != "" {
			iss.error(Issue{Field: path, Code: rule.Code, Message: fmt.Sprintf(rule.MessageFormat, name, problem), Suggestion: rule.Suggestion})
		}
	}
	steps := stepsOf(flow)
	for _, id := range sortedKeys(steps) {
		step, ok := asRecord(steps[id])
		if !ok {
			continue
		}
		placement := spec.AssetGrammar.PlacementOutside
		for _, name := range sortedNames(assetRefsOutsideSites(step)) {
			iss.error(Issue{Field: stepField(id), Code: placement.Code, StepID: id,
				Message: fmt.Sprintf(placement.MessageFormat, id, name), Suggestion: placement.Suggestion})
		}
		all := map[string]bool{}
		collectAssetRefs(step, all)
		unknown := spec.AssetGrammar.ReferenceUnknown
		for _, name := range sortedNames(all) {
			if declared[name] != nil {
				continue
			}
			iss.error(Issue{Field: stepField(id), Code: unknown.Code, StepID: id,
				Message: fmt.Sprintf(unknown.MessageFormat, id, name), Suggestion: unknown.Suggestion})
		}
	}
}
