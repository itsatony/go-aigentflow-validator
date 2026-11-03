package aifvalidate

import (
	"fmt"
	"sort"
	"strings"
)

// The "did you mean" for an unknown key (AIgentFlow #322), ported from the
// reference's closeKeys (aigentflow.parser.keyhints.go). Mirrors keyHints.ts in
// the JS port, which reproduces these Go string semantics exactly: the case fold
// is strings.ToLower, the distance counts runes, the threshold divides the
// guess's byte length.

const (
	// keyHintMaxDistanceFloor and keyHintDistanceDivisor set the threshold:
	// max(floor, len(guess)/divisor).
	keyHintMaxDistanceFloor = 2
	keyHintDistanceDivisor  = 3
	keyHintDidYouMeanFormat = " Did you mean %s?"
	keyHintQuoteFormat      = "`%s`"
	keyHintJoin             = " or "
)

// closeKeys offers the accepted keys a guess may have meant: the alias table's
// targets first (in table order, only those accepted declares), then every
// accepted key within the edit-distance threshold, nearest first. Ties keep
// accepted's order, so callers pass the SORTED key list both ports print as
// "Known keys here". Deduplicated; nil when nothing is close.
func closeKeys(guess string, accepted []string) []string {
	acceptedSet := newSet(accepted)
	var out []string
	seen := map[string]bool{}
	add := func(key string) {
		if acceptedSet.has(key) && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	lowerGuess := strings.ToLower(guess)
	for _, alias := range spec.KeyAliases[lowerGuess] {
		add(alias)
	}
	limit := max(keyHintMaxDistanceFloor, len(guess)/keyHintDistanceDivisor)
	type scored struct {
		key      string
		distance int
	}
	var near []scored
	for _, key := range accepted {
		if d := editDistance(lowerGuess, strings.ToLower(key)); d <= limit {
			near = append(near, scored{key, d})
		}
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].distance < near[j].distance })
	for _, n := range near {
		add(n.key)
	}
	return out
}

// didYouMean is " Did you mean `a` or `b`?", or "" when no accepted key is close.
func didYouMean(guess string, accepted []string) string {
	matches := closeKeys(guess, accepted)
	if len(matches) == 0 {
		return ""
	}
	quoted := make([]string, len(matches))
	for i, key := range matches {
		quoted[i] = fmt.Sprintf(keyHintQuoteFormat, key)
	}
	return fmt.Sprintf(keyHintDidYouMeanFormat, strings.Join(quoted, keyHintJoin))
}
