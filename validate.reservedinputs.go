package aifvalidate

import (
	"fmt"
	"strings"
)

// validateReservedInputNames refuses an input the flow declares in the
// platform's "__" namespace (reserved_input_name).
//
// The platform keeps that namespace for values it writes itself: the acting
// person, the credential manifest, the untrusted-input marker. A declared
// default under such a name used to be a way for a flow author to write one of
// them, and a value or default written there is never delivered at run time, so
// the author learns at save that the name cannot carry data.
//
// Both places an input is declared are read: the keys of the top-level `query:`
// map (reported sorted, field `query.<name>`) and the `name` of every
// `input_schema.fields[]` entry (in file order, field
// `input_schema.fields.<name>`). The schema field is addressed by NAME, not by
// index like this port's other input_schema findings, because the reference
// reports it that way and a host that merges both verdicts drops a finding both
// sides raise only when code and field are equal. A non-string name is skipped
// (input_schema_invalid_field_name already names it). Findings carry no step id.
func validateReservedInputNames(flow doc, iss *issues) {
	rule := spec.ReservedInputNames
	report := func(name, fieldFormat string) {
		iss.error(Issue{
			Field:      fmt.Sprintf(fieldFormat, name),
			Code:       rule.Code,
			Message:    fmt.Sprintf(rule.MessageFormat, fmt.Sprintf("%q", name)),
			Suggestion: rule.Suggestion,
		})
	}
	if query, ok := getRecord(flow, keyQuery); ok {
		for _, name := range sortedKeys(query) {
			if strings.HasPrefix(name, rule.Prefix) {
				report(name, rule.QueryFieldFormat)
			}
		}
	}
	schema, ok := getRecord(flow, keyInputSchema)
	if !ok {
		return
	}
	fields, _ := getSlice(schema, keyFields)
	for _, raw := range fields {
		f, isRec := asRecord(raw)
		if !isRec {
			continue
		}
		if name, isStr := getString(f, keyName); isStr && strings.HasPrefix(name, rule.Prefix) {
			report(name, rule.SchemaFieldFormat)
		}
	}
}
