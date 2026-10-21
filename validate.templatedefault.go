package aifvalidate

import (
	"sort"
	"text/template"
	"text/template/parse"
)

// templateFuncDefault is the AIgentFlow template function whose argument order
// the rule below judges.
const templateFuncDefault = "default"

// codeTemplateDefaultPiped marks a pipeline that pipes a value into `default`.
const codeTemplateDefaultPiped = "template_default_piped"

const (
	msgTemplateDefaultPipedPrefix = "`default` takes the value FIRST, and a pipe passes the piped value LAST: `"
	msgTemplateDefaultPipedSuffix = "` is `default <fallback> <value>`, so the fallback is always the result"
	sugTemplateDefaultPiped       = "write the value first and the fallback second, as a call: `{{ default .x \"fallback\" }}` (the pipe form of other template libraries does not work here)"
)

// templateDefaultPipedMessage renders the finding's message for one pipeline.
func templateDefaultPipedMessage(pipeline string) string {
	return msgTemplateDefaultPipedPrefix + pipeline + msgTemplateDefaultPipedSuffix
}

// pipedDefaultPipelines returns the source text of every pipeline in tmpl (and in
// its defined / associated templates) in which `default` is not the FIRST
// command, i.e. a value is piped into it.
//
// `default` is value first in the engine, so `{{ .x | default "w" }}` is the
// call `default "w" .x`: the "value" is the literal, never empty, and the
// result is always the fallback. Nothing fails at run time; the flow just never
// uses the data it was given, which only a static rule can see.
//
// The rule walks the PARSE TREE, not the text. A regex over `| default` would
// fire inside a string literal, on a field or variable named `default`
// (`.query.default`, `$default`) and inside a comment; the parse tree tells
// them apart, because only a bare identifier in command position is a call.
// A `default` anywhere after the first command counts, not only the last.
func pipedDefaultPipelines(tmpl *template.Template) []string {
	if tmpl == nil {
		return nil
	}
	found := map[string]struct{}{}
	for _, t := range tmpl.Templates() {
		if t.Tree == nil || t.Root == nil {
			continue
		}
		walkPipedDefault(t.Root, found)
	}
	out := make([]string, 0, len(found))
	for pipe := range found {
		out = append(out, pipe)
	}
	sort.Strings(out)
	return out
}

func walkPipedDefault(node parse.Node, found map[string]struct{}) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			walkPipedDefault(child, found)
		}
	case *parse.ActionNode:
		walkPipedDefault(n.Pipe, found)
	case *parse.IfNode:
		walkPipedDefaultBranch(&n.BranchNode, found)
	case *parse.RangeNode:
		walkPipedDefaultBranch(&n.BranchNode, found)
	case *parse.WithNode:
		walkPipedDefaultBranch(&n.BranchNode, found)
	case *parse.TemplateNode:
		walkPipedDefault(n.Pipe, found)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for i, cmd := range n.Cmds {
			if i > 0 && commandCallsFunction(cmd, templateFuncDefault) {
				found[n.String()] = struct{}{}
			}
			for _, arg := range cmd.Args {
				walkPipedDefault(arg, found)
			}
		}
	case *parse.ChainNode:
		walkPipedDefault(n.Node, found)
	}
}

func walkPipedDefaultBranch(b *parse.BranchNode, found map[string]struct{}) {
	walkPipedDefault(b.Pipe, found)
	walkPipedDefault(b.List, found)
	if b.ElseList != nil {
		walkPipedDefault(b.ElseList, found)
	}
}

// commandCallsFunction reports whether the command CALLS name: its first word is
// the bare identifier. A field (`.default`), a variable (`$default`) and a string
// literal are other node types and never match.
func commandCallsFunction(cmd *parse.CommandNode, name string) bool {
	if cmd == nil || len(cmd.Args) == 0 {
		return false
	}
	ident, ok := cmd.Args[0].(*parse.IdentifierNode)
	return ok && ident.Ident == name
}
