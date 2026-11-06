package aifvalidate

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
	"text/template/parse"
)

// human_answer_field (WARNING, AIgentFlow v2.878.0): a human step's answer read
// under a key no answer door commits.
//
// Every door a person answers a human step through commits the same envelope:
// `response`, plus `comments` when they wrote one and `timed_out` when a
// default_response was applied, and nothing else. A flow reading any other key
// gets "<no value>" with no error at all: a demo flow read
// `.step.response.content` on a human step, the answer rendered `<no value>`, a
// guard ran atoi over it and the run could not continue. Two sites are judged:
//
//   - `.data.<h>.<x>` in any step's template, where h is a human step, and
//   - `.step.response.<x>` in h's OWN `next:` conditions and `post_processing`,
//     which are the two places `.step.response` is the answer.
//
// Mirrors humanAnswerFieldWarnings (validation.humananswer.go) on the same
// parse tree: top-level human steps only, root field chains only. A
// `range`/`with` body rebinds the dot and is not walked, `index .data "h" "x"`
// is not judged, and an `async://human/announce` step never parks.

// humanAnswerSteps is the set of top-level steps whose committed data is a
// person's answer: an executor starting with the human prefix whose remainder
// is non-empty and whose mode (the remainder's first `/` segment) parks.
func humanAnswerSteps(flow doc) map[string]bool {
	steps := stepsOf(flow)
	if steps == nil {
		return nil
	}
	out := map[string]bool{}
	for id, raw := range steps {
		step, ok := asRecord(raw)
		if !ok {
			continue
		}
		executor, ok := getString(step, keyExecutor)
		if !ok || !strings.HasPrefix(executor, spec.HumanAnswerField.ExecutorPrefix) {
			continue
		}
		rest := strings.TrimPrefix(executor, spec.HumanAnswerField.ExecutorPrefix)
		if rest == "" {
			continue
		}
		if !hasString(spec.HumanAnswerField.NonParkingModes, strings.Split(rest, "/")[0]) {
			out[id] = true
		}
	}
	return out
}

func isHumanAnswerKey(key string) bool {
	return hasString(spec.HumanAnswerField.AnswerKeys, key)
}

// hasString reports whether list holds want.
func hasString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// humanAnswerFieldIssues judges one parsed template at field, a template of
// step stepID, against the flow's human steps. One finding per distinct
// reference, ordered by message, as the reference orders them.
func humanAnswerFieldIssues(field, stepID string, tmpl *template.Template, humanSteps map[string]bool) []Issue {
	if tmpl == nil || len(humanSteps) == 0 {
		return nil
	}
	const (
		dataRoot    = "data"
		stepRoot    = "step"
		responseKey = "response"
		minChain    = 3
	)
	ownAnswerSite := humanSteps[stepID] && isOwnStepResponseSite(field, stepID)
	meant := map[string]string{}
	for _, t := range tmpl.Templates() {
		if t.Tree == nil || t.Root == nil {
			continue
		}
		walkRootFieldChains(t.Root, func(ident []string) {
			if len(ident) < minChain || isHumanAnswerKey(ident[2]) {
				return
			}
			switch {
			case ident[0] == dataRoot && humanSteps[ident[1]]:
				h, x := ident[1], ident[2]
				meant[fmt.Sprintf(spec.HumanAnswerField.DataMessageFormat, stepID, h, x, h, h, x)] =
					"." + dataRoot + "." + h + "." + responseKey
			case ownAnswerSite && ident[0] == stepRoot && ident[1] == responseKey:
				x := ident[2]
				meant[fmt.Sprintf(spec.HumanAnswerField.StepMessageFormat, stepID, x, x)] =
					"." + stepRoot + "." + responseKey + "." + responseKey
			}
		})
	}
	messages := make([]string, 0, len(meant))
	for m := range meant {
		messages = append(messages, m)
	}
	sort.Strings(messages)
	out := make([]Issue, 0, len(messages))
	for _, m := range messages {
		out = append(out, Issue{
			Field: field, Code: spec.HumanAnswerField.Code, StepID: stepID, Message: m,
			Suggestion: fmt.Sprintf(spec.HumanAnswerField.SuggestionFormat, meant[m]),
		})
	}
	return out
}

// isOwnStepResponseSite reports whether a template at field of top-level step
// stepID runs after that step's answer: its `next:` conditions and its
// `post_processing` operations. Its query and pre_processing run before the
// park, and a loop sub-step's paths carry the parent's id and a `.loop.`
// segment, so they never match.
func isOwnStepResponseSite(field, stepID string) bool {
	prefix := stepField(stepID) + "."
	return strings.HasPrefix(field, prefix+keyNext+"."+keyConditions+"[") ||
		strings.HasPrefix(field, prefix+keyPostProcessing+"[")
}

// walkRootFieldChains calls visit with the identifier chain of every field
// reference rooted at the template's dot (`.data.step.field` →
// [data step field]), wherever Go templates can nest one. `range` and `with`
// rebind the dot inside their bodies, so only their controlling pipeline is a
// root reference; a parenthesised chain (`(expr).data`) hangs off a non-root
// base and is never one; a variable (`$.data`) is not a field.
func walkRootFieldChains(node parse.Node, visit func(ident []string)) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			walkRootFieldChains(child, visit)
		}
	case *parse.ActionNode:
		walkRootFieldChains(n.Pipe, visit)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, cmd := range n.Cmds {
			walkRootFieldChains(cmd, visit)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			walkRootFieldChains(arg, visit)
		}
	case *parse.FieldNode:
		visit(n.Ident)
	case *parse.ChainNode:
		walkRootFieldChains(n.Node, visit)
	case *parse.IfNode:
		walkRootFieldChains(n.Pipe, visit)
		walkRootFieldChains(n.List, visit)
		walkRootFieldChains(n.ElseList, visit)
	case *parse.RangeNode:
		walkRootFieldChains(n.Pipe, visit)
	case *parse.WithNode:
		walkRootFieldChains(n.Pipe, visit)
	case *parse.TemplateNode:
		walkRootFieldChains(n.Pipe, visit)
	}
}
