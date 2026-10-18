package aifvalidate

import (
	"fmt"
	"strings"
)

const keyFlowData = "data"

// flowDataKeyReservation reports whether a top-level `data:` key names engine
// state, and why (reference: FlowDataKeyReservation). Matching is exact and
// case-sensitive; exact names are tried before prefixes.
func flowDataKeyReservation(key string) (reason string, reserved bool) {
	if reason, ok := spec.FlowDataReservedKeys.Exact[key]; ok {
		return reason, true
	}
	for _, p := range spec.FlowDataReservedKeys.Prefixes {
		if strings.HasPrefix(key, p.Prefix) {
			return p.Reason, true
		}
	}
	return "", false
}

// validateFlowDataKeys refuses a top-level `data:` key that names engine state
// (flow_data_key_reserved; reference: validateFlowDataKeysAreNotReserved).
//
// `data:` is seeded into the mission's `.data` namespace, which is also where
// the engine keeps its own bookkeeping; a key naming it would forge internal
// state. ERROR, never demoted: a `data:` block has no loop-body twin. Only the
// flow-level block is scanned (a step has no `data:` of its own). A `data:`
// that is not a mapping is not this rule's concern.
func validateFlowDataKeys(flow doc, iss *issues) {
	data, ok := getRecord(flow, keyFlowData)
	if !ok {
		return
	}
	for _, key := range sortedKeys(data) {
		reason, reserved := flowDataKeyReservation(key)
		if !reserved {
			continue
		}
		iss.error(Issue{
			Field:      fmt.Sprintf(spec.FlowDataReservedKeys.FieldFormat, key),
			Code:       codeFlowDataKeyReserved,
			Message:    fmt.Sprintf(spec.FlowDataReservedKeys.MessageFormat, key, reason),
			Suggestion: spec.FlowDataReservedKeys.Suggestion,
		})
	}
}
