package aifvalidate

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// The examples: block (v0.7.0).
//
// A flow may carry concrete cases — an input, what a good result looks like, why the
// case matters and what must never happen. The block is read by tooling, never by the
// flow engine; this file judges it statically and offline: it never fetches a
// reference, resolves a host or reads a registry.
//
// Every limit, pattern, vocabulary and each code's severity is read from the vendored
// spec (spec.Examples) — never typed here — so a change to any of them is a one-file
// diff to the spec. The strict key sets (a typo at example, expected, matcher, variant
// depth) come from spec.knownKeys through validateUnknownKeys, not from this file.
//
// Two deliberate divergences from the reference, both recorded in PARITY.md:
//   - the reference judges an example's inline input with its full runtime
//     input-schema walker; this port judges a NARROWED subset (see checkInput);
//   - example_ref_blocked_host (a literal private/loopback/metadata host) is
//     reference-only and is not raised here.

// Keys of the examples block.
const (
	keyExamples       = "examples"
	keyExTitle        = "title"
	keyExGuidance     = "guidance"
	keyExNotes        = "notes"
	keyExTags         = "tags"
	keyExWeight       = "weight"
	keyExHoldOut      = "hold_out"
	keyExInput        = "input"
	keyExInputRef     = "input_ref"
	keyExExpected     = "expected"
	keyExCheckpoints  = "checkpoints"
	keyExVariants     = "variants"
	keyExMinScore     = "min_score"
	keyExSideEffects  = "side_effects"
	keyExOrigin       = "origin"
	keyExInputPatch   = "input_patch"
	keyExProvenance   = "provenance"
	keyExRubric       = "rubric"
	keyExReference    = "reference"
	keyExFields       = "fields"
	keyExExact        = "exact"
	keyExMustNot      = "must_not"
	keyExMustNotCont  = "must_not_contain"
	keyExStatus       = "status"
	keyExText         = "text"
	keyExRef          = "ref"
	keyExURL          = "url"
	keyExSHA256       = "sha256"
	keyExMediaType    = "media_type"
	keyExNote         = "note"
	keyExContains     = "contains"
	keyExNotContains  = "not_contains"
	keyExMatches      = "matches"
	keyExOneOf        = "one_of"
	keyExApprox       = "approx"
	keyExTolerance    = "tolerance"
	keyExPresent      = "present"
	keyVisibility     = "visibility"
	valueVisibilityPb = "public"

	exSeverityWarning = "warning"
	exStatusCompleted = "completed"
	exStatusFailed    = "failed"
	exStatusPaused    = "paused_for_human"
	exOriginAuthor    = "author"
	exSchemeHTTPS     = "https"
	exFileRefKey      = "ref" // the shape a file-typed input takes after a FileRef is resolved
	exMaxDepth        = 32

	exTypeString    = "string"
	exTypeMultiline = "multiline"
	exTypeSecret    = "secret"
	exTypeBool      = "bool"
	exTypeDate      = "date"
	exDateLayout    = "2006-01-02"

	exPathFormat        = "examples[%d]"
	exVariantPathFormat = "examples[%d].variants[%d]"
)

// Codes of the examples block. Severities live in spec.examples.codes.
const (
	codeExTooMany            = "examples_too_many"
	codeExBlockTooLarge      = "examples_block_too_large"
	codeExIDMissing          = "example_id_missing"
	codeExIDInvalid          = "example_id_invalid"
	codeExIDDuplicate        = "example_id_duplicate"
	codeExTitleMissing       = "example_title_missing"
	codeExTitleTooLong       = "example_title_too_long"
	codeExGuidanceTooLong    = "example_guidance_too_long"
	codeExGuidanceMissing    = "example_guidance_missing"
	codeExNotesTooLong       = "example_notes_too_long"
	codeExTagInvalid         = "example_tag_invalid"
	codeExWeightRange        = "example_weight_range"
	codeExHoldoutInPublic    = "example_holdout_in_public_flow"
	codeExInputConflict      = "example_input_conflict"
	codeExInputMissing       = "example_input_missing"
	codeExInputTooLarge      = "example_input_too_large"
	codeExInputInvalid       = "example_input_invalid"
	codeExInputUnvalidated   = "example_input_unvalidated"
	codeExFileInputNeedsRef  = "example_file_input_needs_ref"
	codeExSecretValue        = "example_secret_value"
	codeExSecretLikeValue    = "example_secret_like_value"
	codeExRefInvalid         = "example_ref_invalid"
	codeExRefScheme          = "example_ref_scheme"
	codeExRefSchemeReserved  = "example_ref_scheme_reserved"
	codeExRefSHA256Invalid   = "example_ref_sha256_invalid"
	codeExRefMediaInvalid    = "example_ref_media_type_invalid"
	codeExRefUnpinned        = "example_ref_unpinned"
	codeExRefSignedURL       = "example_ref_signed_url"
	codeExExpectedMissing    = "example_expected_missing"
	codeExExpectedEmpty      = "example_expected_empty"
	codeExExpectedConflict   = "example_expected_conflict"
	codeExExpectedFieldUnk   = "example_expected_field_unknown"
	codeExExpectedFieldUnchk = "example_expected_field_unchecked"
	codeExMatcherInvalid     = "example_field_matcher_invalid"
	codeExRubricEmpty        = "example_rubric_empty"
	codeExRubricTooLong      = "example_rubric_too_long"
	codeExReferenceInvalid   = "example_reference_invalid"
	codeExReferenceBinary    = "example_reference_binary"
	codeExMustNotInvalid     = "example_must_not_invalid"
	codeExMustNotContainBad  = "example_must_not_contain_invalid"
	codeExContradiction      = "example_expectation_contradiction"
	codeExStatusInvalid      = "example_status_invalid"
	codeExMinScoreRange      = "example_min_score_range"
	codeExSideEffectsInvalid = "example_side_effects_invalid"
	codeExCheckpointUnknown  = "example_checkpoint_step_unknown"
	codeExCheckpointComp     = "example_checkpoint_step_composite"
	codeExCheckpointStatus   = "example_checkpoint_status"
	codeExVariantsTooMany    = "example_variants_too_many"
	codeExVariantIDInvalid   = "example_variant_id_invalid"
	codeExVariantIDDup       = "example_variant_id_duplicate"
	codeExVariantOriginBad   = "example_variant_origin_invalid"
	codeExVariantEmpty       = "example_variant_empty"
	codeExVariantConflict    = "example_variant_input_conflict"
	codeExVariantInputBad    = "example_variant_input_invalid"
	codeExVariantUnchecked   = "example_variant_unchecked"
	codeExAllHeldOut         = "examples_all_held_out"
	codeExPublishedPublic    = "examples_published_with_public_flow"
	codeExUnreadableByJudge  = "examples_unreadable_by_judge"
)

// Messages. Wording is not part of the parity contract; it is kept identical to the
// reference where practical.
const (
	exMsgTooMany          = "flow declares %d examples; the limit is %d"
	exMsgBlockTooLarge    = "the examples block is %d bytes; the limit is %d. Move large inputs or target outputs to a file reference (https URL)."
	exMsgIDMissing        = "example #%d has no id"
	exMsgIDInvalid        = "example id %q is not valid: use lower-case letters, digits and underscores, starting with a letter (2 to 48 characters)"
	exMsgIDDuplicate      = "example id %q is used by examples #%d and #%d"
	exMsgTitleMissing     = "example %q has no title; the title is what a person sees in the list of examples"
	exMsgTitleTooLong     = "example %q title is %d characters; the limit is %d"
	exMsgGuidanceLong     = "example %q guidance is %d characters; the limit is %d"
	exMsgGuidanceMissing  = "example %q has no guidance. One sentence on why this case matters helps the judge and the next author."
	exMsgNotesLong        = "example %q notes are %d characters; the limit is %d"
	exMsgTagInvalid       = "example %q has an invalid tag %q"
	exMsgTagsTooMany      = "example %q has %d tags; the limit is %d"
	exMsgTagDuplicate     = "example %q lists the tag %q more than once"
	exMsgWeightRange      = "example %q has weight %v; use a number above 0 and at most 10"
	exMsgHoldoutPublic    = "example %q is held out but this flow is public: a held-out example must stay private, and a public flow's definition is readable by everyone. Make the flow private or remove hold_out."
	exMsgInputConflict    = "example %q sets both input and input_ref; use exactly one"
	exMsgInputMissing     = "example %q has neither input nor input_ref; use input: {} for a flow that takes no input"
	exMsgInputTooLarge    = "example %q input is %d bytes; the limit is %d. Put the large part behind an https file reference."
	exMsgInputInvalid     = "example %q: %s"
	exMsgInputUndeclared  = "example %q: %q is not a declared input of this flow (declared: %s)"
	exMsgInputUnchecked   = "example %q cannot be checked: the flow declares no input_schema and no query parameters."
	exMsgFileNeedsRef     = "example %q: field %q is a file; give it as {url: \"https://…\"}. A pasted upload id would stop working as soon as the session ends."
	exMsgSecretValue      = "example %q gives a value for %q, which is a secret field. Examples are saved inside the flow and are never allowed to hold secrets; remove it. The person running the evaluation is asked for it at run time."
	exMsgSecretLike       = "%s looks like a credential (%s). Examples are saved in the flow definition and must never contain keys or tokens. Replace it with a made-up value."
	exMsgRefInvalid       = "the file reference needs a url of at most %d characters"
	exMsgRefInvalidAuth   = "a reference must not carry a login or a fragment"
	exMsgRefScheme        = "only https:// references are allowed (got %q)"
	exMsgRefReserved      = "%s: references are not available yet. Use an https URL, or paste the text into the example."
	exMsgRefSHA256        = "sha256 must be 64 lowercase hexadecimal characters"
	exMsgRefMediaType     = "media_type %q must look like type/subtype"
	exMsgRefNoteLong      = "the note is %d characters; the limit is %d"
	exMsgRefUnpinned      = "this reference is not pinned. If the file at that address changes, your scores change with it. Add sha256 to freeze it."
	exMsgRefSigned        = "this address looks like a signed link. Anyone who can read the flow can use it until it expires, and it will stop working. Prefer a stable public address."
	exMsgExpectedMissing  = "example %q does not say what a good result is: add a rubric, fields, exact or reference"
	exMsgExpectedEmpty    = "this expectation checks nothing; add a rubric, fields, exact, reference, must_not or must_not_contain"
	exMsgExpectedBoth     = "use fields (some fields) or exact (the whole result), not both"
	exMsgExpectedNoOut    = "a case that ends in %s has no output to match; remove fields and exact, or expect status completed"
	exMsgFieldUnknown     = "%q is not an output of this flow (outputs: %s)"
	exMsgStepFieldUnknown = "%q is not a field this step declares in its output_schema (declared: %s)"
	exMsgFieldUnchecked   = "this flow declares no output list, so %q cannot be checked against it"
	exMsgStepUnchecked    = "this step declares no output_schema, so %q cannot be checked against it"
	exMsgMatcherOne       = "write exactly one of equals, contains, not_contains, matches, one_of, approx, present"
	exMsgMatcherRegex     = "the pattern in matches must be a valid regular expression of at most %d characters"
	exMsgMatcherApprox    = "approx needs a tolerance (a number, zero or more) beside it, and tolerance is only for approx"
	exMsgRubricEmpty      = "the rubric is empty; write what a good result looks like, or remove it"
	exMsgRubricLong       = "the rubric is %d characters; the limit is %d"
	exMsgReferenceOne     = "a reference needs exactly one of text or ref"
	exMsgReferenceLong    = "the reference text is %d bytes; the limit is %d. Put it behind an https file reference."
	exMsgReferenceBinary  = "this reference is not text, so it cannot be compared in this version; the example will be skipped, not failed."
	exMsgMustNotBad       = "must_not takes up to %d statements of at most %d characters each, none empty"
	exMsgMustNotContain   = "must_not_contain takes up to %d literals of 1 to %d characters each"
	exMsgContradiction    = "%q is required by %q and forbidden by must_not_contain"
	exMsgStatusInvalid    = "status must be completed, failed or paused_for_human"
	exMsgMinScoreRange    = "example %q has min_score %v; use a number from 0 to 1 (0.7 is 7.3 out of 10)"
	exMsgScoreRangeAt     = "min_score %v is out of range; use a number from 0 to 1"
	exMsgSideEffects      = "example %q has side_effects %q; use refuse or allow"
	exMsgCheckpointStep   = "example %q: checkpoint %q is not a step of this flow (steps: %s)"
	exMsgCheckpointComp   = "example %q: step %q repeats or branches, so there is no single result to check; put the checkpoint on a plain step or check the final output"
	exMsgCheckpointStatus = "example %q: a checkpoint cannot set status; status describes how the whole run ends"
	exMsgVariantsMany     = "example %q has %d variants; the limit is %d"
	exMsgVariantIDBad     = "variant id %q of example %q is not valid: use lower-case letters, digits and underscores, starting with a letter (2 to 48 characters)"
	exMsgVariantIDDup     = "variant id %q is used twice in example %q"
	exMsgVariantOrigin    = "variant %q of example %q has origin %q; use author (a person wrote or approved it) or synthetic (generated)"
	exMsgVariantEmpty     = "variant %q of example %q changes neither the input nor the expectation"
	exMsgVariantBoth      = "variant %q of example %q sets both input_patch and input_ref; use one"
	exMsgVariantUncheck   = "variant %q of example %q cannot be checked: its parent takes its input from input_ref."
	exMsgAllHeldOut       = "every example is held out, so nothing is left to tune on"
	exMsgPublicFlow       = "this flow is public, so everyone who can see it can read its examples, including their expected results. Only publish examples you are happy to show."
	exMsgUnreadable       = "this example can be run but not scored"
	exMsgInputRequired    = "input field %q is required"
	exMsgInputUnknown     = "input field %q is not declared in input_schema"
	exMsgInputKind        = "input field %q is not a valid %s"
	exMsgInputEnum        = "input field %q value %q is not one of [%s]"
	exMsgInputItem        = "input field %q index %d is not a string"

	exSuggID          = "Pick a short stable name such as standard_invoice; it is the key results are filed under, so it never changes once saved"
	exSuggCheckpoint  = "Did you rename the step? Update the checkpoint to the new name"
	exSuggSecretValue = "Remove the value; you are asked for it when you run the check"
	exSuggExpected    = "Add at least a rubric: one or two sentences on what a good result looks like"
)

// Compiled patterns, built once from the spec.
var (
	exIDRe        *regexp.Regexp
	exTagRe       *regexp.Regexp
	exSHA256Re    *regexp.Regexp
	exMediaTypeRe *regexp.Regexp
	exSignedKeyRe *regexp.Regexp
	exSecretRes   []*regexp.Regexp

	exOrigins      set
	exSideEffects  set
	exStatuses     set
	exReservedRefs set
)

// initExamples compiles the spec's patterns. The signed-URL query-key pattern is
// carried in the spec without a flag; the reference matches it case-insensitively
// (an `X-Amz-Signature` key is the canonical case), so it is compiled that way here.
func initExamples() {
	e := &spec.Examples
	exIDRe = regexp.MustCompile(e.IDPattern)
	exTagRe = regexp.MustCompile(e.TagPattern)
	exSHA256Re = regexp.MustCompile(e.SHA256Pattern)
	exMediaTypeRe = regexp.MustCompile(e.MediaTypePattern)
	exSignedKeyRe = regexp.MustCompile("(?i)" + e.SignedURLQueryKeyPattern)
	exSecretRes = make([]*regexp.Regexp, 0, len(e.SecretPatterns))
	for _, p := range e.SecretPatterns {
		expr := p.Expr
		if strings.Contains(p.Flags, "i") {
			expr = "(?i)" + expr
		}
		exSecretRes = append(exSecretRes, regexp.MustCompile(expr))
	}
	exOrigins = newSet(e.Origins)
	exSideEffects = newSet(e.SideEffects)
	exStatuses = newSet(e.Statuses)
	exReservedRefs = newSet(e.ReservedRefSchemes)
}

// exampleSecretMatch names the first credential shape s resembles, or "".
func exampleSecretMatch(s string) string {
	for i, re := range exSecretRes {
		if re.MatchString(s) {
			return spec.Examples.SecretPatterns[i].Name
		}
	}
	return ""
}

type exFinding struct {
	code, field, message, suggestion string
}

type exampleWalker struct {
	flow     doc
	iss      *issues
	findings []exFinding
}

func (w *exampleWalker) add(code, field, message string) {
	w.findings = append(w.findings, exFinding{code: code, field: field, message: message})
}

func (w *exampleWalker) addHint(code, field, message, suggestion string) {
	w.findings = append(w.findings, exFinding{code: code, field: field, message: message, suggestion: suggestion})
}

// validateExamples judges the flow's examples: block. An absent or empty block yields
// nothing: every flow that predates the grammar is unchanged.
func validateExamples(flow doc, iss *issues) {
	raw, ok := getSlice(flow, keyExamples)
	if !ok || len(raw) == 0 {
		return
	}
	w := &exampleWalker{flow: flow, iss: iss}
	w.validateSet(raw)
	seen := make(map[string]int, len(raw))
	for i, item := range raw {
		if ex, ok := asRecord(item); ok {
			w.validateExample(i, ex, seen)
		}
	}
	for _, f := range w.findings {
		issue := Issue{Field: f.field, Code: f.code, Message: f.message, Suggestion: f.suggestion}
		if spec.Examples.Codes[f.code] == exSeverityWarning {
			iss.warn(issue)
		} else {
			iss.error(issue)
		}
	}
}

func (w *exampleWalker) validateSet(raw []any) {
	lim := spec.Examples.Limits
	if n := len(raw); n > lim.MaxPerFlow {
		w.add(codeExTooMany, keyExamples, fmt.Sprintf(exMsgTooMany, n, lim.MaxPerFlow))
	}
	if n := exampleBlockBytes(raw); n > lim.MaxBlockBytes {
		w.add(codeExBlockTooLarge, keyExamples, fmt.Sprintf(exMsgBlockTooLarge, n, lim.MaxBlockBytes))
	}
	allHeld := true
	for _, item := range raw {
		ex, ok := asRecord(item)
		if held, isBool := asBool(get(ex, keyExHoldOut)); !ok || !isBool || !held {
			allHeld = false
		}
	}
	if allHeld {
		w.add(codeExAllHeldOut, keyExamples, exMsgAllHeldOut)
	}
	if w.isPublic() {
		w.add(codeExPublishedPublic, keyExamples, exMsgPublicFlow)
	}
}

func (w *exampleWalker) isPublic() bool {
	v, _ := w.iss.stringOf(w.flow, keyVisibility, "")
	return v == valueVisibilityPb
}

// exampleBlockBytes measures the block as the reference's typed structs would
// serialise it: struct-level empties are omitted, open maps (input, exact,
// input_patch) keep every key. Dropping the empties can only make this smaller, so
// the limit is never judged more strictly than the reference judges it.
func exampleBlockBytes(raw []any) int {
	pruned := pruneExampleValue(raw, parseKnownKeyShape("list<ExampleDefinition>"), 0)
	blob, err := json.Marshal(pruned)
	if err != nil {
		return 0
	}
	return len(blob)
}

func pruneExampleValue(v any, shape knownKeyShape, depth int) any {
	if depth > exMaxDepth {
		return nil
	}
	switch shape.kind {
	case shapeKindStruct:
		m, ok := asRecord(v)
		if !ok {
			return normalizeExampleValue(v, 0)
		}
		out := make(doc, len(m))
		for key, decl := range spec.KnownKeys.Types[shape.name] {
			val, present := m[key]
			if !present {
				continue
			}
			p := pruneExampleValue(val, parseKnownKeyShape(decl), depth+1)
			if exampleEmptyValue(p) {
				continue
			}
			out[key] = p
		}
		return out
	case shapeKindList:
		items, ok := asSlice(v)
		if !ok {
			return normalizeExampleValue(v, 0)
		}
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = pruneExampleValue(item, *shape.elem, depth+1)
		}
		return out
	case shapeKindMap:
		m, ok := asRecord(v)
		if !ok {
			return normalizeExampleValue(v, 0)
		}
		out := make(doc, len(m))
		for k, item := range m {
			out[k] = pruneExampleValue(item, *shape.elem, depth+1)
		}
		return out
	}
	return normalizeExampleValue(v, 0)
}

// exampleEmptyValue is encoding/json's omitempty for the value kinds a decoded
// document holds.
func exampleEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	case doc:
		return len(x) == 0
	}
	return false
}

// normalizeExampleValue turns what yaml.v3 decodes into something json can encode and
// the input kinds can be judged on: an unquoted date arrives as time.Time (a live run
// receives a JSON string). depth bounds a self-referential alias.
func normalizeExampleValue(v any, depth int) any {
	if depth > exMaxDepth {
		return nil
	}
	switch x := v.(type) {
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 && x.Nanosecond() == 0 {
			return x.Format(exDateLayout)
		}
		return x.Format(time.RFC3339)
	case doc:
		out := make(doc, len(x))
		for k, val := range x {
			out[k] = normalizeExampleValue(val, depth+1)
		}
		return out
	case map[any]any:
		out := make(doc, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = normalizeExampleValue(val, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeExampleValue(val, depth+1)
		}
		return out
	}
	return v
}

func exampleLabel(i int, id string) string {
	if id == "" {
		return fmt.Sprintf("#%d", i+1)
	}
	return id
}

// str reads m[key] as the reference's Go string field receives it.
func (w *exampleWalker) str(m doc, key, parentPath string) string {
	s, _ := w.iss.stringOf(m, key, parentPath)
	return s
}

func exNumber(m doc, key string) (float64, bool) { return asNumber(get(m, key)) }

func (w *exampleWalker) validateExample(i int, ex doc, seen map[string]int) {
	lim := spec.Examples.Limits
	path := fmt.Sprintf(exPathFormat, i)
	id := w.str(ex, keyID, path)
	label := exampleLabel(i, id)

	switch {
	case id == "":
		w.addHint(codeExIDMissing, path+"."+keyID, fmt.Sprintf(exMsgIDMissing, i+1), exSuggID)
	case !exIDRe.MatchString(id):
		w.addHint(codeExIDInvalid, path+"."+keyID, fmt.Sprintf(exMsgIDInvalid, id), exSuggID)
	default:
		if first, dup := seen[id]; dup {
			w.add(codeExIDDuplicate, path+"."+keyID, fmt.Sprintf(exMsgIDDuplicate, id, first+1, i+1))
		} else {
			seen[id] = i
		}
	}

	if title := w.str(ex, keyExTitle, path); strings.TrimSpace(title) == "" {
		w.add(codeExTitleMissing, path+"."+keyExTitle, fmt.Sprintf(exMsgTitleMissing, label))
	} else if n := utf8.RuneCountInString(title); n > lim.TitleMaxRunes {
		w.add(codeExTitleTooLong, path+"."+keyExTitle, fmt.Sprintf(exMsgTitleTooLong, label, n, lim.TitleMaxRunes))
	}

	if g := w.str(ex, keyExGuidance, path); strings.TrimSpace(g) == "" {
		w.add(codeExGuidanceMissing, path+"."+keyExGuidance, fmt.Sprintf(exMsgGuidanceMissing, label))
	} else if n := utf8.RuneCountInString(g); n > lim.MaxGuidanceRunes {
		w.add(codeExGuidanceTooLong, path+"."+keyExGuidance, fmt.Sprintf(exMsgGuidanceLong, label, n, lim.MaxGuidanceRunes))
	}
	if n := utf8.RuneCountInString(w.str(ex, keyExNotes, path)); n > lim.MaxNotesRunes {
		w.add(codeExNotesTooLong, path+"."+keyExNotes, fmt.Sprintf(exMsgNotesLong, label, n, lim.MaxNotesRunes))
	}

	w.validateTags(path, label, ex)

	w.validateWeight(path+"."+keyExWeight, label, ex)
	if v, ok := exNumber(ex, keyExMinScore); ok && !exampleScoreInRange(v) {
		w.add(codeExMinScoreRange, path+"."+keyExMinScore, fmt.Sprintf(exMsgMinScoreRange, label, v))
	}
	if se := w.str(ex, keyExSideEffects, path); se != "" && !exSideEffects.has(se) {
		w.add(codeExSideEffectsInvalid, path+"."+keyExSideEffects, fmt.Sprintf(exMsgSideEffects, label, se))
	}

	if held, _ := asBool(get(ex, keyExHoldOut)); held && w.isPublic() {
		w.add(codeExHoldoutInPublic, path+"."+keyExHoldOut, fmt.Sprintf(exMsgHoldoutPublic, label))
	}

	// input / input_ref
	hasInput, hasInputRef := present(ex, keyExInput), present(ex, keyExInputRef)
	switch {
	case hasInput && hasInputRef:
		w.add(codeExInputConflict, path, fmt.Sprintf(exMsgInputConflict, label))
	case !hasInput && !hasInputRef:
		w.add(codeExInputMissing, path, fmt.Sprintf(exMsgInputMissing, label))
	}
	var baseInput doc
	if in, ok := getRecord(ex, keyExInput); ok {
		baseInput = in
		w.checkInput(path+"."+keyExInput, label, in, codeExInputInvalid)
	}
	if ref, ok := getRecord(ex, keyExInputRef); ok {
		w.validateFileRef(path+"."+keyExInputRef, w.fileRefOf(ref, path+"."+keyExInputRef))
	}

	// expected
	binaryOnly := false
	if !present(ex, keyExExpected) {
		w.addHint(codeExExpectedMissing, path+"."+keyExExpected, fmt.Sprintf(exMsgExpectedMissing, label), exSuggExpected)
	} else if exp, ok := getRecord(ex, keyExExpected); ok {
		w.validateExpectation(path+"."+keyExExpected, label, exp, false, "")
		binaryOnly = w.expectationIsBinaryReferenceOnly(exp, path+"."+keyExExpected)
	}
	if binaryOnly {
		w.add(codeExUnreadableByJudge, path+"."+keyExExpected, exMsgUnreadable)
	}

	w.validateCheckpoints(path, label, ex)
	w.validateVariants(i, path, label, ex, hasInput, baseInput)
	w.scanSecretLike(i, path, ex)
}

func (w *exampleWalker) validateTags(path, label string, ex doc) {
	tags, ok := getSlice(ex, keyExTags)
	if !ok {
		return
	}
	if len(tags) > spec.Examples.Limits.MaxTags {
		w.add(codeExTagInvalid, path+"."+keyExTags, fmt.Sprintf(exMsgTagsTooMany, label, len(tags), spec.Examples.Limits.MaxTags))
	}
	seen := make(map[string]bool, len(tags))
	for j, raw := range tags {
		field := indexed(path+"."+keyExTags, j)
		tag := ""
		if raw != nil {
			s, ok := w.iss.stringAt(raw, field)
			if !ok {
				continue
			}
			tag = s
		}
		switch {
		case !exTagRe.MatchString(tag):
			w.add(codeExTagInvalid, field, fmt.Sprintf(exMsgTagInvalid, label, tag))
		case seen[tag]:
			w.add(codeExTagInvalid, field, fmt.Sprintf(exMsgTagDuplicate, label, tag))
		}
		seen[tag] = true
	}
}

func (w *exampleWalker) validateWeight(field, label string, m doc) {
	weight, ok := exNumber(m, keyExWeight)
	if !ok {
		return
	}
	if math.IsNaN(weight) || weight <= 0 || weight > spec.Examples.Limits.WeightMax {
		w.add(codeExWeightRange, field, fmt.Sprintf(exMsgWeightRange, label, weight))
	}
}

func exampleScoreInRange(v float64) bool {
	lim := spec.Examples.Limits
	return !math.IsNaN(v) && v >= lim.MinScoreMin && v <= lim.MinScoreMax
}

// ---------------------------------------------------------------------------
// FileRef
// ---------------------------------------------------------------------------

type exFileRef struct {
	url, sha256, mediaType, note string
}

// fileRefOf reads a decoded FileRef mapping as the reference's typed struct does:
// any scalar fills a string field.
func (w *exampleWalker) fileRefOf(m doc, path string) exFileRef {
	return exFileRef{
		url:       w.str(m, keyExURL, path),
		sha256:    w.str(m, keyExSHA256, path),
		mediaType: w.str(m, keyExMediaType, path),
		note:      w.str(m, keyExNote, path),
	}
}

var exFileRefKeys = newSet([]string{keyExURL, keyExSHA256, keyExMediaType, keyExNote})

// asExampleFileRef reads an input VALUE as a FileRef when its keys are exactly the
// FileRef keys; the values are read the way the reference reads a decoded `any`.
func asExampleFileRef(v any) (exFileRef, bool) {
	m, ok := v.(doc)
	if !ok || len(m) == 0 {
		return exFileRef{}, false
	}
	for k := range m {
		if !exFileRefKeys.has(k) {
			return exFileRef{}, false
		}
	}
	var ref exFileRef
	ref.url, _ = m[keyExURL].(string)
	ref.sha256, _ = m[keyExSHA256].(string)
	ref.mediaType, _ = m[keyExMediaType].(string)
	ref.note, _ = m[keyExNote].(string)
	return ref, true
}

func (w *exampleWalker) validateFileRef(field string, ref exFileRef) {
	lim := spec.Examples.Limits
	raw := ref.url
	if raw == "" || len(raw) > lim.MaxURLBytes {
		w.add(codeExRefInvalid, field, fmt.Sprintf(exMsgRefInvalid, lim.MaxURLBytes))
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		w.add(codeExRefInvalid, field, fmt.Sprintf(exMsgRefInvalid, lim.MaxURLBytes))
		return
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case exReservedRefs.has(scheme):
		w.add(codeExRefSchemeReserved, field, fmt.Sprintf(exMsgRefReserved, scheme))
		return
	case scheme != exSchemeHTTPS:
		w.add(codeExRefScheme, field, fmt.Sprintf(exMsgRefScheme, scheme))
		return
	case u.User != nil || u.Fragment != "" || strings.Contains(raw, "#") || u.Host == "":
		w.add(codeExRefInvalid, field, exMsgRefInvalidAuth)
		return
	}
	if ref.sha256 != "" && !exSHA256Re.MatchString(ref.sha256) {
		w.add(codeExRefSHA256Invalid, field, exMsgRefSHA256)
	}
	if ref.mediaType != "" && !exMediaTypeRe.MatchString(ref.mediaType) {
		w.add(codeExRefMediaInvalid, field, fmt.Sprintf(exMsgRefMediaType, ref.mediaType))
	}
	if n := utf8.RuneCountInString(ref.note); n > lim.MaxNoteRunes {
		w.add(codeExRefInvalid, field, fmt.Sprintf(exMsgRefNoteLong, n, lim.MaxNoteRunes))
	}
	if ref.sha256 == "" {
		w.add(codeExRefUnpinned, field, exMsgRefUnpinned)
	}
	for key := range u.Query() {
		if exSignedKeyRe.MatchString(key) {
			w.add(codeExRefSignedURL, field, exMsgRefSigned)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// input (NARROWED — see PARITY.md)
// ---------------------------------------------------------------------------

// exSchemaField is the part of an input_schema field the narrowed check reads.
type exSchemaField struct {
	name, typ string
	required  bool
	enum      []string
	vwField   string
	vwEquals  any
	vwIn      []any
	hasVW     bool
}

func (w *exampleWalker) schemaFields(schema doc) []exSchemaField {
	rawFields, _ := getSlice(schema, keyFields)
	out := make([]exSchemaField, 0, len(rawFields))
	for i, raw := range rawFields {
		f, ok := asRecord(raw)
		if !ok {
			continue
		}
		base := schemaFieldPath(keyInputSchema, i)
		sf := exSchemaField{
			name: w.str(f, keyName, base),
			typ:  w.str(f, keyType, base),
		}
		sf.required, _ = asBool(get(f, keyRequired))
		if values, ok := getSlice(f, keyEnum); ok {
			for _, v := range values {
				if s, ok := scalarText(v); ok {
					sf.enum = append(sf.enum, s)
				}
			}
		}
		if vw, ok := getRecord(f, keyVisibleWhen); ok {
			sf.hasVW = true
			sf.vwField = w.str(vw, keyField, base+"."+keyVisibleWhen)
			sf.vwEquals = get(vw, keyEquals)
			sf.vwIn, _ = getSlice(vw, keyIn)
		}
		out = append(out, sf)
	}
	return out
}

// checkInput judges one inline input mapping (an example's `input`, or a variant's
// patched input). It is a NARROWED check: unknown field, required-missing (honouring
// visible_when, skipping secrets), value KIND (string/multiline/secret/enum/date =
// string, number, bool, array_of_strings, file = a FileRef) and enum membership. It
// does NOT judge length, min/max, pattern or date-format constraints — the reference's
// host application judges those, and refuses the union of both verdicts.
func (w *exampleWalker) checkInput(field, label string, raw doc, code string) {
	input, _ := normalizeExampleValue(raw, 0).(doc)
	if input == nil {
		input = doc{}
	}
	if blob, err := json.Marshal(input); err == nil && len(blob) > spec.Examples.Limits.MaxInlineInputBytes {
		w.add(codeExInputTooLarge, field, fmt.Sprintf(exMsgInputTooLarge, label, len(blob), spec.Examples.Limits.MaxInlineInputBytes))
		return
	}
	schemaRaw := get(w.flow, keyInputSchema)
	query, queryOK := getRecord(w.flow, keyQuery)
	switch {
	case schemaRaw != nil:
		if schema, ok := asRecord(schemaRaw); ok {
			w.checkInputAgainstSchema(field, label, input, schema, code)
		}
	case queryOK && len(query) > 0:
		declared := sortedKeys(query)
		for _, k := range sortedKeys(input) {
			if _, ok := query[k]; !ok {
				w.add(code, field+"."+k, fmt.Sprintf(exMsgInputUndeclared, label, k, strings.Join(declared, ", ")))
			}
		}
	default:
		w.add(codeExInputUnvalidated, field, fmt.Sprintf(exMsgInputUnchecked, label))
	}
}

func (w *exampleWalker) checkInputAgainstSchema(field, label string, input doc, schema doc, code string) {
	fields := w.schemaFields(schema)
	byName := make(map[string]*exSchemaField, len(fields))
	for i := range fields {
		byName[fields[i].name] = &fields[i]
	}
	// The run input: a FileRef in a file-typed field becomes {ref: <url>}; a secret
	// value is reported and removed (a secret is never judged — the person running
	// the evaluation supplies it).
	runInput := make(doc, len(input))
	for _, name := range sortedKeys(input) {
		val := input[name]
		f := byName[name]
		switch {
		case f != nil && f.typ == exTypeSecret:
			if val != nil {
				w.addHint(codeExSecretValue, field+"."+name, fmt.Sprintf(exMsgSecretValue, label, name), exSuggSecretValue)
			}
		case f != nil && f.typ == typeFile && val != nil:
			ref, ok := asExampleFileRef(val)
			if !ok {
				w.add(codeExFileInputNeedsRef, field+"."+name, fmt.Sprintf(exMsgFileNeedsRef, label, name))
				continue
			}
			w.validateFileRef(field+"."+name, ref)
			runInput[name] = doc{exFileRefKey: ref.url}
		default:
			runInput[name] = val
		}
	}
	report := func(name, message string) {
		w.add(code, field+"."+name, fmt.Sprintf(exMsgInputInvalid, label, message))
	}
	for i := range fields {
		f := &fields[i]
		if !exFieldVisible(f, runInput) {
			continue
		}
		val, present := runInput[f.name]
		if !present || val == nil {
			// A required secret nobody may supply here is not an error at save: it
			// makes the case needs-secret at run time.
			if f.required && f.typ != exTypeSecret {
				report(f.name, fmt.Sprintf(exMsgInputRequired, f.name))
			}
			continue
		}
		for _, msg := range exInputKindFailures(f, val) {
			report(f.name, msg)
		}
	}
	for _, name := range sortedKeys(runInput) {
		if _, ok := byName[name]; !ok {
			report(name, fmt.Sprintf(exMsgInputUnknown, name))
		}
	}
}

func exFieldVisible(f *exSchemaField, input doc) bool {
	if !f.hasVW {
		return true
	}
	dep, present := input[f.vwField]
	if !present {
		return false
	}
	if f.vwEquals != nil {
		return exValuesEqual(dep, f.vwEquals)
	}
	for _, candidate := range f.vwIn {
		if exValuesEqual(dep, candidate) {
			return true
		}
	}
	return false
}

// exValuesEqual compares two decoded scalars, rejecting cross-kind matches.
func exValuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	if af, ok := asNumber(a); ok {
		bf, ok := asNumber(b)
		return ok && af == bf
	}
	switch av := a.(type) {
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	}
	return false
}

// exInputKindFailures judges only the KIND (and enum membership) of one value.
func exInputKindFailures(f *exSchemaField, val any) []string {
	mismatch := func(expected string) []string {
		return []string{fmt.Sprintf(exMsgInputKind, f.name, expected)}
	}
	switch f.typ {
	case exTypeString, exTypeMultiline, exTypeSecret:
		if _, ok := val.(string); !ok {
			return mismatch(exTypeString)
		}
	case typeNumber:
		if _, ok := asNumber(val); !ok {
			return mismatch(typeNumber)
		}
	case exTypeBool:
		if _, ok := val.(bool); !ok {
			return mismatch(exTypeBool)
		}
	case typeEnum:
		s, ok := val.(string)
		if !ok {
			return mismatch(typeEnum)
		}
		for _, allowed := range f.enum {
			if s == allowed {
				return nil
			}
		}
		return []string{fmt.Sprintf(exMsgInputEnum, f.name, s, strings.Join(f.enum, ","))}
	case typeArrayOfStrings:
		arr, ok := val.([]any)
		if !ok {
			return mismatch(typeArrayOfStrings)
		}
		var out []string
		for i, item := range arr {
			if _, ok := item.(string); !ok {
				out = append(out, fmt.Sprintf(exMsgInputItem, f.name, i))
			}
		}
		return out
	case exTypeDate:
		if _, ok := val.(string); !ok {
			return mismatch(exTypeDate)
		}
	}
	return nil
}

// applyExamplePatch applies an RFC 7386 JSON Merge Patch: a mapping merges
// recursively, null deletes the key, anything else replaces. The base is never
// mutated and the result shares no map with either argument.
func applyExamplePatch(base, patch doc) doc {
	out := deepCopyExampleMap(base)
	mergeExamplePatch(out, patch, 0)
	return out
}

func mergeExamplePatch(target, patch doc, depth int) {
	if depth > exMaxDepth {
		return
	}
	for k, pv := range patch {
		if pv == nil {
			delete(target, k)
			continue
		}
		if pm, ok := asRecord(pv); ok {
			existing, _ := asRecord(target[k])
			if existing == nil {
				existing = doc{}
			} else {
				existing = deepCopyExampleMap(existing)
			}
			mergeExamplePatch(existing, pm, depth+1)
			target[k] = existing
			continue
		}
		target[k] = deepCopyExampleValue(pv, 0)
	}
}

func deepCopyExampleMap(m doc) doc {
	out := make(doc, len(m))
	for k, v := range m {
		out[k] = deepCopyExampleValue(v, 0)
	}
	return out
}

func deepCopyExampleValue(v any, depth int) any {
	if depth > exMaxDepth {
		return v
	}
	if m, ok := asRecord(v); ok {
		out := make(doc, len(m))
		for k, val := range m {
			out[k] = deepCopyExampleValue(val, depth+1)
		}
		return out
	}
	if s, ok := asSlice(v); ok {
		out := make([]any, len(s))
		for i, val := range s {
			out[i] = deepCopyExampleValue(val, depth+1)
		}
		return out
	}
	return v
}

// ---------------------------------------------------------------------------
// Expectation
// ---------------------------------------------------------------------------

// outputNamesFor returns the names fields/exact may use and whether the list is
// declared at all. A final expectation reads the flow's `output:`; a checkpoint reads
// the step's output_schema.
func (w *exampleWalker) outputNamesFor(checkpoint bool, stepID string) (names []string, declared bool) {
	if !checkpoint {
		list, ok := getSlice(w.flow, keyOutput)
		if !ok || len(list) == 0 {
			return nil, false
		}
		for i, item := range list {
			s, _ := w.iss.stringAt(item, indexed(keyOutput, i))
			names = append(names, s)
		}
		return names, true
	}
	step, ok := getRecord(stepsOf(w.flow), stepID)
	if !ok {
		return nil, false
	}
	schema, ok := getRecord(step, keyOutputSchema)
	if !ok {
		return nil, false
	}
	fields, ok := getSlice(schema, keyFields)
	if !ok || len(fields) == 0 {
		return nil, false
	}
	for i, item := range fields {
		f, _ := asRecord(item)
		names = append(names, w.str(f, keyName, stepField(stepID, keyOutputSchema, indexed(keyFields, i))))
	}
	return names, true
}

func (w *exampleWalker) validateExpectation(path, label string, e doc, checkpoint bool, stepID string) {
	lim := spec.Examples.Limits
	status := w.str(e, keyExStatus, path)
	if status != "" && !exStatuses.has(status) {
		w.add(codeExStatusInvalid, path+"."+keyExStatus, exMsgStatusInvalid)
	}
	noOutput := status == exStatusFailed || status == exStatusPaused
	fields, _ := getRecord(e, keyExFields)
	exact, _ := getRecord(e, keyExExact)
	hasFields, hasExact := len(fields) > 0, len(exact) > 0
	switch {
	case hasFields && hasExact:
		w.add(codeExExpectedConflict, path, exMsgExpectedBoth)
	case noOutput && (hasFields || hasExact):
		w.add(codeExExpectedConflict, path, fmt.Sprintf(exMsgExpectedNoOut, status))
	}

	if v, ok := exNumber(e, keyExMinScore); ok && !exampleScoreInRange(v) {
		w.add(codeExMinScoreRange, path+"."+keyExMinScore, fmt.Sprintf(exMsgScoreRangeAt, v))
	}

	rubric := w.str(e, keyExRubric, path)
	if rubric != "" {
		if strings.TrimSpace(rubric) == "" {
			w.add(codeExRubricEmpty, path+"."+keyExRubric, exMsgRubricEmpty)
		} else if n := utf8.RuneCountInString(rubric); n > lim.MaxRubricRunes {
			w.add(codeExRubricTooLong, path+"."+keyExRubric, fmt.Sprintf(exMsgRubricLong, n, lim.MaxRubricRunes))
		}
	}

	hasReference := present(e, keyExReference)
	if ref, ok := getRecord(e, keyExReference); ok {
		w.validateReference(path+"."+keyExReference, ref)
	}

	names, declared := w.outputNamesFor(checkpoint, stepID)
	checkKey := func(field, key string) {
		switch {
		case !declared && !checkpoint:
			w.add(codeExExpectedFieldUnchk, field, fmt.Sprintf(exMsgFieldUnchecked, key))
		case !declared:
			w.add(codeExExpectedFieldUnchk, field, fmt.Sprintf(exMsgStepUnchecked, key))
		case !exampleContainsString(names, key):
			msg := exMsgFieldUnknown
			if checkpoint {
				msg = exMsgStepFieldUnknown
			}
			w.add(codeExExpectedFieldUnk, field, fmt.Sprintf(msg, key, strings.Join(names, ", ")))
		}
	}
	for _, key := range sortedKeys(fields) {
		fpath := path + "." + keyExFields + "." + key
		checkKey(fpath, key)
		w.validateFieldMatcher(fpath, fields[key])
	}
	for _, key := range sortedKeys(exact) {
		checkKey(path+"."+keyExExact+"."+key, key)
	}

	mustNot, _ := getSlice(e, keyExMustNot)
	if len(mustNot) > lim.MaxMustNot {
		w.add(codeExMustNotInvalid, path+"."+keyExMustNot, fmt.Sprintf(exMsgMustNotBad, lim.MaxMustNot, lim.MaxMustNotRunes))
	}
	for j, item := range mustNot {
		s, _ := w.iss.stringAt(item, indexed(path+"."+keyExMustNot, j))
		if n := utf8.RuneCountInString(s); strings.TrimSpace(s) == "" || n > lim.MaxMustNotRunes {
			w.add(codeExMustNotInvalid, indexed(path+"."+keyExMustNot, j), fmt.Sprintf(exMsgMustNotBad, lim.MaxMustNot, lim.MaxMustNotRunes))
		}
	}
	mustNotContain, _ := getSlice(e, keyExMustNotCont)
	var banned []string
	if len(mustNotContain) > lim.MaxMustNotContain {
		w.add(codeExMustNotContainBad, path+"."+keyExMustNotCont, fmt.Sprintf(exMsgMustNotContain, lim.MaxMustNotContain, lim.MaxMustNotContainLength))
	}
	for j, item := range mustNotContain {
		s, _ := w.iss.stringAt(item, indexed(path+"."+keyExMustNotCont, j))
		banned = append(banned, s)
		if n := utf8.RuneCountInString(s); n == 0 || n > lim.MaxMustNotContainLength {
			w.add(codeExMustNotContainBad, indexed(path+"."+keyExMustNotCont, j), fmt.Sprintf(exMsgMustNotContain, lim.MaxMustNotContain, lim.MaxMustNotContainLength))
		}
	}
	for _, key := range sortedKeys(fields) {
		m, _ := asRecord(fields[key])
		eq, ok := get(m, keyEquals).(string)
		if !ok || eq == "" {
			continue
		}
		for _, b := range banned {
			if b != "" && strings.EqualFold(b, eq) {
				w.add(codeExContradiction, path+"."+keyExFields+"."+key, fmt.Sprintf(exMsgContradiction, b, key))
			}
		}
	}

	if checkpoint && status != "" {
		w.add(codeExCheckpointStatus, path+"."+keyExStatus, fmt.Sprintf(exMsgCheckpointStatus, label))
	}

	if rubric == "" && !hasReference && !hasFields && !hasExact && len(mustNot) == 0 &&
		len(mustNotContain) == 0 && (status == "" || status == exStatusCompleted) {
		w.addHint(codeExExpectedEmpty, path, exMsgExpectedEmpty, exSuggExpected)
	}
}

func (w *exampleWalker) validateReference(path string, r doc) {
	text := w.str(r, keyExText, path)
	refRec, hasRef := getRecord(r, keyExRef)
	hasText := text != ""
	if hasText == hasRef {
		w.add(codeExReferenceInvalid, path, exMsgReferenceOne)
		return
	}
	if hasText {
		if len(text) > spec.Examples.Limits.MaxReferenceTextBytes {
			w.add(codeExReferenceInvalid, path, fmt.Sprintf(exMsgReferenceLong, len(text), spec.Examples.Limits.MaxReferenceTextBytes))
		}
		return
	}
	ref := w.fileRefOf(refRec, path+"."+keyExRef)
	w.validateFileRef(path+"."+keyExRef, ref)
	if ref.mediaType != "" && !isTextualMediaType(ref.mediaType) {
		w.add(codeExReferenceBinary, path, exMsgReferenceBinary)
	}
}

// isTextualMediaType says whether the judge can read a reference of this declared
// media type. Conservative: only what is plainly text.
func isTextualMediaType(mt string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mt, ";", 2)[0]))
	switch {
	case strings.HasPrefix(base, "text/"):
		return true
	case strings.HasSuffix(base, "+json"), strings.HasSuffix(base, "+xml"):
		return true
	}
	switch base {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml", "application/markdown":
		return true
	}
	return false
}

func (w *exampleWalker) expectationIsBinaryReferenceOnly(e doc, path string) bool {
	refRec, ok := getRecord(e, keyExReference)
	if !ok {
		return false
	}
	ref, ok := getRecord(refRec, keyExRef)
	if !ok {
		return false
	}
	mediaType := w.str(ref, keyExMediaType, path+"."+keyExReference+"."+keyExRef)
	if mediaType == "" || isTextualMediaType(mediaType) {
		return false
	}
	fields, _ := getRecord(e, keyExFields)
	exact, _ := getRecord(e, keyExExact)
	mustNot, _ := getSlice(e, keyExMustNot)
	mustNotContain, _ := getSlice(e, keyExMustNotCont)
	status := w.str(e, keyExStatus, path)
	return w.str(e, keyExRubric, path) == "" && len(fields) == 0 && len(exact) == 0 &&
		len(mustNot) == 0 && len(mustNotContain) == 0 && (status == "" || status == exStatusCompleted)
}

func (w *exampleWalker) validateFieldMatcher(path string, raw any) {
	m, ok := asRecord(raw)
	if !ok && raw != nil {
		return // a wrong shape is reported by the generic kind check
	}
	operators := 0
	if get(m, keyEquals) != nil {
		operators++
	}
	if w.str(m, keyExContains, path) != "" {
		operators++
	}
	if w.str(m, keyExNotContains, path) != "" {
		operators++
	}
	matches := w.str(m, keyExMatches, path)
	if matches != "" {
		operators++
	}
	if oneOf, _ := getSlice(m, keyExOneOf); len(oneOf) > 0 {
		operators++
	}
	_, hasApprox := exNumber(m, keyExApprox)
	if hasApprox {
		operators++
	}
	if _, isBool := asBool(get(m, keyExPresent)); isBool {
		operators++
	}
	if operators != 1 {
		w.add(codeExMatcherInvalid, path, exMsgMatcherOne)
		return
	}
	if matches != "" {
		maxLen := spec.Examples.Limits.MaxPatternLength
		if len(matches) > maxLen {
			w.add(codeExMatcherInvalid, path, fmt.Sprintf(exMsgMatcherRegex, maxLen))
		} else if _, err := regexp.Compile(matches); err != nil {
			w.add(codeExMatcherInvalid, path, fmt.Sprintf(exMsgMatcherRegex, maxLen))
		}
	}
	// approx needs its tolerance, and tolerance belongs to approx alone.
	tol, hasTol := exNumber(m, keyExTolerance)
	if hasApprox != hasTol || (hasTol && (math.IsNaN(tol) || tol < 0)) {
		w.add(codeExMatcherInvalid, path, exMsgMatcherApprox)
	}
}

// ---------------------------------------------------------------------------
// checkpoints
// ---------------------------------------------------------------------------

func (w *exampleWalker) validateCheckpoints(path, label string, ex doc) {
	cps, ok := getRecord(ex, keyExCheckpoints)
	if !ok || len(cps) == 0 {
		return
	}
	steps := stepsOf(w.flow)
	stepIDs := sortedKeys(steps)
	for _, id := range sortedKeys(cps) {
		cpath := path + "." + keyExCheckpoints + "." + id
		step, ok := getRecord(steps, id)
		if !ok {
			w.addHint(codeExCheckpointUnknown, cpath, fmt.Sprintf(exMsgCheckpointStep, label, id, strings.Join(stepIDs, ", ")), exSuggCheckpoint)
			continue
		}
		if exStepIsComposite(step) {
			w.add(codeExCheckpointComp, cpath, fmt.Sprintf(exMsgCheckpointComp, label, id))
			continue
		}
		cp, ok := asRecord(cps[id])
		if !ok {
			if cps[id] != nil {
				continue
			}
			cp = doc{}
		}
		w.validateExpectation(cpath, label, cp, true, id)
	}
}

// exStepIsComposite mirrors the reference's step kinds for_each, loop and parallel:
// a step that repeats or fans out has no single result to check.
func exStepIsComposite(step doc) bool {
	if present(step, keyForEach) || present(step, keyLoop) {
		return true
	}
	next, ok := getRecord(step, keyNext)
	return ok && present(next, keyParallel)
}

// ---------------------------------------------------------------------------
// variants
// ---------------------------------------------------------------------------

func (w *exampleWalker) validateVariants(i int, path, label string, ex doc, parentHasInput bool, baseInput doc) {
	lim := spec.Examples.Limits
	variants, ok := getSlice(ex, keyExVariants)
	if !ok || len(variants) == 0 {
		return
	}
	if len(variants) > lim.MaxVariants {
		w.add(codeExVariantsTooMany, path+"."+keyExVariants, fmt.Sprintf(exMsgVariantsMany, label, len(variants), lim.MaxVariants))
	}
	seen := make(map[string]bool, len(variants))
	for j, item := range variants {
		v, ok := asRecord(item)
		if !ok {
			continue
		}
		vpath := fmt.Sprintf(exVariantPathFormat, i, j)
		vid := w.str(v, keyID, vpath)
		vlabel := vid
		if vlabel == "" {
			vlabel = fmt.Sprintf("#%d", j+1)
		}
		both := label + "/" + vlabel

		switch {
		case !exIDRe.MatchString(vid):
			w.addHint(codeExVariantIDInvalid, vpath+"."+keyID, fmt.Sprintf(exMsgVariantIDBad, vid, label), exSuggID)
		case seen[vid]:
			w.add(codeExVariantIDDup, vpath+"."+keyID, fmt.Sprintf(exMsgVariantIDDup, vid, label))
		}
		seen[vid] = true

		if origin := w.str(v, keyExOrigin, vpath); !exOrigins.has(origin) {
			w.add(codeExVariantOriginBad, vpath+"."+keyExOrigin, fmt.Sprintf(exMsgVariantOrigin, vlabel, label, origin))
		}
		if n := utf8.RuneCountInString(w.str(v, keyExTitle, vpath)); n > lim.TitleMaxRunes {
			w.add(codeExTitleTooLong, vpath+"."+keyExTitle, fmt.Sprintf(exMsgTitleTooLong, both, n, lim.TitleMaxRunes))
		}
		if n := utf8.RuneCountInString(w.str(v, keyExGuidance, vpath)); n > lim.MaxGuidanceRunes {
			w.add(codeExGuidanceTooLong, vpath+"."+keyExGuidance, fmt.Sprintf(exMsgGuidanceLong, both, n, lim.MaxGuidanceRunes))
		}
		if n := utf8.RuneCountInString(w.str(v, keyExProvenance, vpath)); n > lim.MaxProvenanceRunes {
			w.add(codeExNotesTooLong, vpath+"."+keyExProvenance, fmt.Sprintf(exMsgNotesLong, both, n, lim.MaxProvenanceRunes))
		}
		w.validateWeight(vpath+"."+keyExWeight, both, v)

		patch, _ := getRecord(v, keyExInputPatch)
		hasPatch := len(patch) > 0
		refRec, hasRef := getRecord(v, keyExInputRef)
		switch {
		case hasPatch && hasRef:
			w.add(codeExVariantConflict, vpath, fmt.Sprintf(exMsgVariantBoth, vlabel, label))
		case !hasPatch && !hasRef && !present(v, keyExExpected):
			w.add(codeExVariantEmpty, vpath, fmt.Sprintf(exMsgVariantEmpty, vlabel, label))
		}
		if hasRef {
			w.validateFileRef(vpath+"."+keyExInputRef, w.fileRefOf(refRec, vpath+"."+keyExInputRef))
		}
		if hasPatch {
			if !parentHasInput {
				w.add(codeExVariantUnchecked, vpath+"."+keyExInputPatch, fmt.Sprintf(exMsgVariantUncheck, vlabel, label))
			} else {
				w.checkVariantPatch(vpath, label, vlabel, baseInput, patch)
			}
		}
		if exp, ok := getRecord(v, keyExExpected); ok {
			w.validateExpectation(vpath+"."+keyExExpected, both, exp, false, "")
		}
	}
}

// checkVariantPatch judges the PATCHED input like a live run, and reports a secret the
// patch itself supplies. A null patch value deleting a secret is fine.
func (w *exampleWalker) checkVariantPatch(vpath, label, vlabel string, base, patch doc) {
	both := label + "/" + vlabel
	if schema, ok := getRecord(w.flow, keyInputSchema); ok {
		for _, f := range w.schemaFields(schema) {
			if f.typ != exTypeSecret {
				continue
			}
			if val, ok := patch[f.name]; ok && val != nil {
				w.addHint(codeExSecretValue, vpath+"."+keyExInputPatch+"."+f.name,
					fmt.Sprintf(exMsgSecretValue, both, f.name), exSuggSecretValue)
			}
		}
	}
	patched := applyExamplePatch(base, patch)
	// A scratch walker, so the variant wording replaces the example wording and the
	// parent's own input problems are not reported twice.
	scratch := &exampleWalker{flow: w.flow, iss: w.iss}
	scratch.checkInput(vpath+"."+keyExInputPatch, both, patched, codeExVariantInputBad)
	for _, f := range scratch.findings {
		if f.code == codeExSecretValue {
			continue // the patch's own secrets are reported above; the parent's by the parent
		}
		w.findings = append(w.findings, f)
	}
}

// ---------------------------------------------------------------------------
// credential-shaped literals
// ---------------------------------------------------------------------------

func (w *exampleWalker) scanSecretLike(i int, path string, ex doc) {
	emit := func(field, s string) {
		if name := exampleSecretMatch(s); name != "" {
			w.addHint(codeExSecretLikeValue, field, fmt.Sprintf(exMsgSecretLike, field, name), exSuggSecretValue)
		}
	}
	emitKey := func(m doc, key, mpath string) {
		s, _ := w.iss.stringOf(m, key, mpath)
		emit(mpath+"."+key, s)
	}
	emitKey(ex, keyExTitle, path)
	emitKey(ex, keyExGuidance, path)
	emitKey(ex, keyExNotes, path)
	if in, ok := getRecord(ex, keyExInput); ok {
		scanExampleLeaves(path+"."+keyExInput, in, 0, emit)
	}
	if exp, ok := getRecord(ex, keyExExpected); ok {
		w.scanExpectationLeaves(path+"."+keyExExpected, exp, emit)
	}
	if cps, ok := getRecord(ex, keyExCheckpoints); ok {
		for _, id := range sortedKeys(cps) {
			if cp, ok := asRecord(cps[id]); ok {
				w.scanExpectationLeaves(path+"."+keyExCheckpoints+"."+id, cp, emit)
			}
		}
	}
	variants, _ := getSlice(ex, keyExVariants)
	for j, item := range variants {
		v, ok := asRecord(item)
		if !ok {
			continue
		}
		vpath := fmt.Sprintf(exVariantPathFormat, i, j)
		emitKey(v, keyExTitle, vpath)
		emitKey(v, keyExGuidance, vpath)
		emitKey(v, keyExProvenance, vpath)
		scanExampleLeaves(vpath+"."+keyExInputPatch, get(v, keyExInputPatch), 0, emit)
		if exp, ok := getRecord(v, keyExExpected); ok {
			w.scanExpectationLeaves(vpath+"."+keyExExpected, exp, emit)
		}
	}
}

func (w *exampleWalker) scanExpectationLeaves(path string, e doc, emit func(field, s string)) {
	str := func(m doc, key, mpath string) {
		s, _ := w.iss.stringOf(m, key, mpath)
		emit(mpath+"."+key, s)
	}
	str(e, keyExRubric, path)
	if ref, ok := getRecord(e, keyExReference); ok {
		str(ref, keyExText, path+"."+keyExReference)
	}
	for _, key := range []string{keyExMustNot, keyExMustNotCont} {
		list, _ := getSlice(e, key)
		for j, item := range list {
			s, _ := w.iss.stringAt(item, indexed(path+"."+key, j))
			emit(indexed(path+"."+key, j), s)
		}
	}
	fields, _ := getRecord(e, keyExFields)
	for _, key := range sortedKeys(fields) {
		m, _ := asRecord(fields[key])
		fpath := path + "." + keyExFields + "." + key
		scanExampleLeaves(fpath+"."+keyEquals, get(m, keyEquals), 0, emit)
		str(m, keyExContains, fpath)
		str(m, keyExNotContains, fpath)
		oneOf, _ := getSlice(m, keyExOneOf)
		for j, one := range oneOf {
			scanExampleLeaves(indexed(fpath+"."+keyExOneOf, j), one, 0, emit)
		}
	}
	scanExampleLeaves(path+"."+keyExExact, get(e, keyExExact), 0, emit)
}

func scanExampleLeaves(path string, v any, depth int, emit func(field, s string)) {
	if depth > exMaxDepth {
		return
	}
	switch x := v.(type) {
	case doc:
		for _, k := range sortedKeys(x) {
			scanExampleLeaves(path+"."+k, x[k], depth+1, emit)
		}
	case string:
		emit(path, x)
	case []any:
		for i, e := range x {
			scanExampleLeaves(indexed(path, i), e, depth+1, emit)
		}
	}
}

func exampleContainsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
