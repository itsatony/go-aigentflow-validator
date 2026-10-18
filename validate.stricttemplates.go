package aifvalidate

import "fmt"

const keyStrictTemplates = "strict_templates"

// yamlLegacyBoolSpellings are the YAML 1.1 spellings the reference's decoder
// still accepts for a bool field (yaml.v3 compatibility shim). Accepting them
// here keeps this rule from refusing a document the reference saves.
var yamlLegacyBoolSpellings = newSet([]string{
	"y", "Y", "yes", "Yes", "YES", "on", "On", "ON",
	"n", "N", "no", "No", "NO", "off", "Off", "OFF",
})

// validateStrictTemplates judges the top-level optional bool `strict_templates`.
// The reference decodes it into a bool, so a number, a list, a mapping or a
// string that is not a bool spelling fails to decode: the flow would not save.
// Absent and null are valid (the zero value). Only the KIND is judged; what
// the flag does at run time is not a static concern.
func validateStrictTemplates(flow doc, iss *issues) {
	if !has(flow, keyStrictTemplates) {
		return
	}
	value := get(flow, keyStrictTemplates)
	if value == nil {
		return
	}
	if _, ok := asBool(value); ok {
		return
	}
	if s, ok := value.(string); ok && yamlLegacyBoolSpellings.has(s) {
		return
	}
	iss.error(Issue{
		Field: keyStrictTemplates, Code: codeInvalidType,
		Message:    fmt.Sprintf("'%s' must be a boolean (true or false)", keyStrictTemplates),
		Suggestion: "write `strict_templates: true` or `strict_templates: false`",
	})
}
