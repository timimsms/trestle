// Package directive scans raw `.d2` bytes for Trestle binding directives.
//
// Directives are magic comments (L3). D2's parser discards comments, so this
// package is deliberately independent of the D2 compiler: a malformed binding
// must never be able to break a render.
//
//	# @bind     <node_id> <glob>
//	# @external <node_id>
//	# @infra    <node_id>
//	# @ignore   <node_id> "<reason>"
//
// One directive per line. No continuations. Position-independent — a directive
// need not sit near the node it names. Parsing is `strings.Fields` and done;
// there is no grammar and no parser generator here, and this package never
// looks at anything but comment lines.
//
// This package reports malformed directives as [SyntaxError] values with their
// source position. It does not decide severity and it does not emit violations:
// SYNTAX is *reported* here and *classified* by internal/check.
package directive

import "fmt"

// Kind is a directive keyword, including its leading `@`.
type Kind string

// The four directive kinds. There are no others; an unrecognized `@token` is a
// syntax error and is never fuzzy-matched onto one of these.
const (
	KindBind     Kind = "@bind"
	KindExternal Kind = "@external"
	KindInfra    Kind = "@infra"
	KindIgnore   Kind = "@ignore"
)

// Kinds lists every recognized directive keyword, in documentation order.
var Kinds = []Kind{KindBind, KindExternal, KindInfra, KindIgnore}

// String returns the directive keyword, e.g. "@bind".
func (k Kind) String() string { return string(k) }

// Form returns the canonical written form of a directive kind, suitable for
// quoting back to the author in a hint. It returns "" for an unknown kind.
func Form(k Kind) string {
	switch k {
	case KindBind:
		return `# @bind <node_id> <glob>`
	case KindExternal:
		return `# @external <node_id>`
	case KindInfra:
		return `# @infra <node_id>`
	case KindIgnore:
		return `# @ignore <node_id> "<reason>"`
	default:
		return ""
	}
}

// Position is the origin of a directive: the file it was read from and the
// 1-based line number within it. Every directive carries one. Violations quote
// it, and without it the hints are useless.
type Position struct {
	File string
	Line int
}

// String renders the position as "file:line", or "line N" when the file is
// unknown (e.g. when parsing an in-memory buffer).
func (p Position) String() string {
	if p.File == "" {
		return fmt.Sprintf("line %d", p.Line)
	}
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// Directive is one well-formed magic comment.
//
// Glob is set only for [KindBind]; Reason only for [KindIgnore]. Multiple
// `@bind` directives may name the same node — their globs are ORed by the
// check engine.
type Directive struct {
	Kind   Kind
	Node   string
	Glob   string // @bind only
	Reason string // @ignore only, always non-empty
	Source Position
	Raw    string // the source line, whitespace-trimmed
}

// String renders the directive in canonical form, without the leading `#`.
func (d Directive) String() string {
	switch d.Kind {
	case KindBind:
		return fmt.Sprintf("%s %s %s", d.Kind, d.Node, d.Glob)
	case KindIgnore:
		return fmt.Sprintf("%s %s %q", d.Kind, d.Node, d.Reason)
	default:
		return fmt.Sprintf("%s %s", d.Kind, d.Node)
	}
}

// SyntaxError is a malformed directive line.
//
// It is a *report*, not a violation: internal/check turns it into a SYNTAX
// violation and assigns severity. Raw is quoted back to the author and Want,
// when non-empty, is the correct form of the directive that was attempted.
type SyntaxError struct {
	Source Position
	Raw    string // the offending source line, whitespace-trimmed
	Detail string // what is wrong, lowercase and unpunctuated
	Want   string // canonical form of the attempted directive, "" if unknown
}

// Error implements error.
func (e SyntaxError) Error() string {
	return fmt.Sprintf("%s: %s", e.Source, e.Detail)
}

// Result is everything one or more `.d2` sources yielded.
//
// Directives and Syntax are both in source order. A file with syntax errors
// still yields every well-formed directive it contains — one bad line does not
// discard the rest of the diagram's bindings.
type Result struct {
	Directives []Directive
	Syntax     []SyntaxError
}

// OfKind returns the directives of one kind, preserving source order.
func (r Result) OfKind(k Kind) []Directive {
	var out []Directive
	for _, d := range r.Directives {
		if d.Kind == k {
			out = append(out, d)
		}
	}
	return out
}

// Count returns how many directives of one kind were parsed.
func (r Result) Count(k Kind) int {
	n := 0
	for _, d := range r.Directives {
		if d.Kind == k {
			n++
		}
	}
	return n
}

// Merge appends another result's directives and syntax errors to this one.
func (r *Result) Merge(other Result) {
	r.Directives = append(r.Directives, other.Directives...)
	r.Syntax = append(r.Syntax, other.Syntax...)
}
