// Package nodes extracts node IDs and the container tree from a D2 diagram.
//
// This is the one place in Trestle that must actually understand D2. It uses
// the upstream compiler (`d2compiler.Compile`) rather than a hand-rolled parser
// or a regex: Gate B (docs/DECISIONS.md, Gate B) established that walking
// `g.Root.ChildrenArray` and reading `Object.AbsID()` recovers every node with
// its container qualification intact. There is no fallback path, by design — if
// the compiler cannot read a diagram, D2 itself cannot render it either.
//
// The package decides nothing. It returns the node set and the parent/child
// relation; `internal/check` decides what is a violation. The parent relation
// is not optional decoration: the O9 container rule (docs/DECISIONS.md, Resolutions) suppresses
// UNBOUND for a container whose descendants are all accounted for, and that
// cannot be evaluated from a flat list of IDs.
package nodes

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2parser"
)

// Node is one object in a D2 diagram.
//
// A node is identified by its fully-qualified ID: the dot-joined path from the
// diagram root, e.g. "platform.svc_work_orders". Directives may name a node by
// an unqualified suffix of that ID; see [Diagram.Candidates].
type Node struct {
	// ID is the fully-qualified node ID (D2's AbsID), e.g.
	// "platform.svc_work_orders".
	ID string

	// Name is the final segment of ID, e.g. "svc_work_orders". For a top-level
	// node it equals ID.
	Name string

	// Parent is the ID of the containing node, or "" for a top-level node.
	Parent string

	// Children holds the IDs of direct children, in declaration order.
	Children []string

	// Shape is D2's resolved shape keyword ("rectangle", "cylinder", "queue",
	// "person", ...). Carried because `explain` and `render` want it; the check
	// engine does not read it.
	Shape string

	// Label is the display label. Per L5 the ID is the code identifier and the
	// label is presentation, so hints should quote ID and only ever show Label
	// as context.
	Label string

	// Line is the 1-based line in the source file where the node is first
	// declared, or 0 if D2 reported no reference for it (which happens for
	// objects the compiler synthesizes). Violations quote it.
	Line int
}

// IsContainer reports whether the node has children. Only containers are
// eligible for the O9 UNBOUND suppression rule.
func (n *Node) IsContainer() bool { return len(n.Children) > 0 }

// Diagram is the node set of one .d2 file.
//
// Every accessor returns IDs in declaration order (the order D2's AST yields
// them), never map order, so output is reproducible run to run.
type Diagram struct {
	// Path is the file path the diagram was parsed under. It is echoed into
	// diagnostics; it is not opened by this package after parsing.
	Path string

	// IDs holds every node ID in the diagram, fully qualified, in declaration
	// order (a pre-order traversal of the container tree).
	IDs []string

	// Roots holds the IDs of top-level nodes, in declaration order.
	Roots []string

	// Nodes maps a fully-qualified ID to its node. Ranging over this map is
	// nondeterministic; range over IDs instead.
	Nodes map[string]*Node
}

// Len returns the number of nodes in the diagram.
func (d *Diagram) Len() int { return len(d.IDs) }

// Has reports whether id is a fully-qualified ID present in the diagram.
// It does not do suffix resolution; see [Diagram.Candidates].
func (d *Diagram) Has(id string) bool {
	_, ok := d.Nodes[id]
	return ok
}

// Node returns the node with the given fully-qualified ID.
func (d *Diagram) Node(id string) (*Node, bool) {
	n, ok := d.Nodes[id]
	return n, ok
}

// Parents returns the full parent relation as id -> parent ID, with "" as the
// parent of a top-level node. Provided because the check engine wants the
// relation as data rather than as a pointer walk.
func (d *Diagram) Parents() map[string]string {
	m := make(map[string]string, len(d.IDs))
	for _, id := range d.IDs {
		m[id] = d.Nodes[id].Parent
	}
	return m
}

// Children returns the IDs of the direct children of id, in declaration order.
// It returns nil for a leaf or an unknown ID.
func (d *Diagram) Children(id string) []string {
	n, ok := d.Nodes[id]
	if !ok {
		return nil
	}
	return n.Children
}

// Descendants returns every transitive descendant of id, in declaration order.
// It returns nil for a leaf or an unknown ID.
//
// This is the input to the O9 container rule: a container is accounted for when
// every one of its descendants is accounted for.
func (d *Diagram) Descendants(id string) []string {
	n, ok := d.Nodes[id]
	if !ok {
		return nil
	}
	var out []string
	var rec func(*Node)
	rec = func(cur *Node) {
		for _, cid := range cur.Children {
			out = append(out, cid)
			rec(d.Nodes[cid])
		}
	}
	rec(n)
	return out
}

// Candidates returns the node IDs that a directive's node ID refers to, per the
// O8 resolution rule (docs/DECISIONS.md, Resolutions):
//
//   - an exact match on a fully-qualified ID wins outright and is returned alone;
//   - otherwise, every node whose ID ends with "." + id is a candidate, so
//     "svc_work_orders" resolves "platform.svc_work_orders" but "orders" does
//     not — the suffix must land on a segment boundary.
//
// This package does not decide what to do with the result. Zero candidates is
// DANGLING and more than one is SYNTAX, and both of those are `internal/check`'s
// call to make; Candidates only reports the fact.
func (d *Diagram) Candidates(id string) []string {
	if id == "" {
		return nil
	}
	if _, ok := d.Nodes[id]; ok {
		return []string{id}
	}
	suffix := "." + id
	var out []string
	for _, nid := range d.IDs {
		if strings.HasSuffix(nid, suffix) {
			out = append(out, nid)
		}
	}
	return out
}

// Diagnostic is one compiler message, located in the source file.
type Diagnostic struct {
	File    string
	Line    int // 1-based; 0 when D2 reported no position
	Column  int // 1-based; 0 when D2 reported no position
	Message string
}

func (d Diagnostic) String() string {
	if d.Line == 0 {
		return fmt.Sprintf("%s: %s", d.File, d.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", d.File, d.Line, d.Column, d.Message)
}

// ErrCompile identifies a failure to compile a .d2 file. Match it with
// errors.Is to distinguish "your diagram does not compile" (exit 2, a tool
// error — Trestle could not do its job) from "your diagram disagrees with the
// repo" (exit 1, a violation). Conflating the two teaches people to ignore both.
var ErrCompile = errors.New("d2 compile failed")

// CompileError is returned when d2compiler rejects a file. It carries every
// diagnostic D2 produced, with positions, so the CLI can print them verbatim
// rather than a single flattened string.
type CompileError struct {
	Path        string
	Diagnostics []Diagnostic

	err error // the underlying d2 error
}

func (e *CompileError) Error() string {
	if len(e.Diagnostics) == 0 {
		return fmt.Sprintf("%s: %v", e.Path, e.err)
	}
	msgs := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		msgs = append(msgs, d.String())
	}
	return strings.Join(msgs, "\n")
}

// Unwrap exposes the underlying d2 error for callers that want to inspect it.
func (e *CompileError) Unwrap() error { return e.err }

// Is reports a match against ErrCompile so callers can classify without
// depending on the d2 error types.
func (e *CompileError) Is(target error) bool { return target == ErrCompile }

// Parse compiles src as a D2 diagram and returns its node set.
//
// path is used only for diagnostics and for [Diagram.Path]; nothing is read
// from disk. A compile failure is returned as a *CompileError, which satisfies
// errors.Is(err, ErrCompile) and maps to exit code 2.
func Parse(path string, src []byte) (*Diagram, error) {
	g, _, err := d2compiler.Compile(path, strings.NewReader(string(src)), nil)
	if err != nil {
		return nil, &CompileError{Path: path, Diagnostics: diagnostics(path, err), err: err}
	}
	if g == nil || g.Root == nil {
		return nil, &CompileError{
			Path: path,
			err:  errors.New("d2compiler returned no graph"),
		}
	}

	d := &Diagram{
		Path:  path,
		Nodes: make(map[string]*Node, len(g.Objects)),
	}
	collect(d, g.Root, "")
	return d, nil
}

// ParseFile reads path and parses it. It is the only filesystem access in this
// package: `internal/walk` owns the repo walk, and this reads one named file
// the caller already decided to open.
func ParseFile(path string) (*Diagram, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read diagram: %w", err)
	}
	return Parse(path, src)
}

// collect walks the object tree depth-first in declaration order. D2 guarantees
// the ordering because Compile calls SortObjectsByAST before returning.
func collect(d *Diagram, obj *d2graph.Object, parentID string) {
	for _, child := range obj.ChildrenArray {
		id := child.AbsID()
		n := &Node{
			ID:     id,
			Name:   child.ID,
			Parent: parentID,
			Shape:  child.Shape.Value,
			Label:  child.Label.Value,
			Line:   declLine(child),
		}
		d.IDs = append(d.IDs, id)
		d.Nodes[id] = n
		if parentID == "" {
			d.Roots = append(d.Roots, id)
		} else if p, ok := d.Nodes[parentID]; ok {
			p.Children = append(p.Children, id)
		}
		collect(d, child, id)
	}
}

// declLine returns the 1-based source line of a node's first reference. D2's
// AST positions are 0-based; violations quote 1-based lines like every other
// tool, so the conversion happens here rather than in three call sites.
func declLine(obj *d2graph.Object) int {
	for _, ref := range obj.References {
		if ref.Key == nil {
			continue
		}
		return ref.Key.Range.Start.Line + 1
	}
	return 0
}

// diagnostics flattens a d2parser.ParseError into positioned Diagnostics. Any
// other error shape is returned as a single unpositioned diagnostic rather than
// dropped — an unrecognized error is still a real failure.
//
// d2 pre-formats each message with its own "file:line:col: " prefix. That is
// stripped so Message is the message and the position is structured data;
// Diagnostic.String puts the prefix back, once.
func diagnostics(path string, err error) []Diagnostic {
	var pe *d2parser.ParseError
	if errors.As(err, &pe) && len(pe.Errors) > 0 {
		out := make([]Diagnostic, 0, len(pe.Errors))
		for _, e := range pe.Errors {
			file := e.Range.Path
			if file == "" {
				file = path
			}
			out = append(out, Diagnostic{
				File:    file,
				Line:    e.Range.Start.Line + 1,
				Column:  e.Range.Start.Column + 1,
				Message: strings.TrimPrefix(e.Message, e.Range.String()+": "),
			})
		}
		return out
	}
	return []Diagnostic{{File: path, Message: err.Error()}}
}
