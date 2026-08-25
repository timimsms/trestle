package explain

import (
	"encoding/json"
	"io"

	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/report"
)

// The JSON view is the one docs/DESIGN.md §5 calls load-bearing: `explain` is what an
// agent runs to orient before editing a node, and it will be read by a program
// more often than by a person. It follows `internal/report`'s conventions
// deliberately — `"version": 1` from day one, absent strings as null rather than
// "", arrays that are `[]` and never null, and an order that is a property of
// the content rather than of map iteration.
//
// # One shape, three questions
//
// Every view emits the same document. `kind` says which question was asked;
// `nodes` and `overlaps` hold the answer, filtered to the query for a
// single-node lookup; `summary`, `diagrams` and `disabled` always describe the
// whole repo, because they are the context the answer is only true in. A
// consumer can therefore parse one schema, and `summary.overlaps` means the same
// number whichever way the command was invoked.
//
// # files is null unless it was asked for
//
// A binding's `files` array is populated in the single-node view and null in the
// others. Null is not zero: `[]` means the glob claims nothing — the ORPHAN case
// — and null means the list was not part of this answer. Dumping every path
// behind every glob into an inventory would put a 100k-file repo's entire
// listing in the payload for a question nobody asked.
type document struct {
	Version int     `json:"version"`
	Kind    string  `json:"kind"`
	Query   *string `json:"query"`
	// Diagrams is every .d2 the tool parsed. It is here because the first
	// question behind "why is my node missing" is usually "did you read the
	// file I am editing".
	Diagrams []string `json:"diagrams"`
	// Disabled names codes set to `off` in config, in check.Codes order. A
	// consumer reading `summary.failures` from `trestle check` without checking
	// this field is trusting a check that may have inspected nothing.
	Disabled   []string         `json:"disabled"`
	Summary    summaryJSON      `json:"summary"`
	Nodes      []nodeJSON       `json:"nodes"`
	Overlaps   []overlapJSON    `json:"overlaps"`
	Unresolved []unresolvedJSON `json:"unresolved"`
}

// summaryJSON is the inventory in numbers, always about the whole repo.
type summaryJSON struct {
	Nodes      int `json:"nodes"`
	Bound      int `json:"bound"`
	External   int `json:"external"`
	Infra      int `json:"infra"`
	Ignored    int `json:"ignored"`
	Container  int `json:"container"`
	Unbound    int `json:"unbound"`
	Bindings   int `json:"bindings"`
	Files      int `json:"files"`
	Overlaps   int `json:"overlaps"`
	Unresolved int `json:"unresolved"`
	Failures   int `json:"failures"`
	Warnings   int `json:"warnings"`
}

type nodeJSON struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Diagram string  `json:"diagram"`
	Label   string  `json:"label"`
	Shape   string  `json:"shape"`
	Parent  *string `json:"parent"`
	Line    int     `json:"line"`
	Status  string  `json:"status"`
	// Files is the number of distinct files this node's bindings claim, so a
	// consumer never has to add up the per-binding counts and get overlap
	// between two of a node's own globs wrong.
	Files      int             `json:"files"`
	Children   []string        `json:"children"`
	Bindings   []bindingJSON   `json:"bindings"`
	Marks      []markJSON      `json:"marks"`
	Violations []violationJSON `json:"violations"`
}

type bindingJSON struct {
	Glob    string     `json:"glob"`
	Source  sourceJSON `json:"source"`
	Matches int        `json:"matches"`
	Files   []string   `json:"files"`
}

type markJSON struct {
	Kind   string     `json:"kind"`
	Reason *string    `json:"reason"`
	Source sourceJSON `json:"source"`
}

// violationJSON is field-for-field the shape `trestle check --format=json`
// emits, minus the fields that would restate the node it is nested under. A
// consumer that already parses check's violations parses these.
type violationJSON struct {
	Code     string     `json:"code"`
	Severity string     `json:"severity"`
	Node     *string    `json:"node"`
	Path     *string    `json:"path"`
	Source   sourceJSON `json:"source"`
	Detail   string     `json:"detail"`
	Hint     string     `json:"hint"`
}

type sourceJSON struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// overlapJSON is keyed by path rather than by the pair of nodes, because the
// question a program asks of this array is "who claims this file". The human
// view groups by claimant instead, which is the question a person debugging a
// copy-pasted glob is asking.
type overlapJSON struct {
	Path  string      `json:"path"`
	Nodes []claimJSON `json:"nodes"`
}

type claimJSON struct {
	Node   string     `json:"node"`
	Glob   string     `json:"glob"`
	Source sourceJSON `json:"source"`
}

type unresolvedJSON struct {
	// Kind is null when the line did not parse at all, in which case nothing on
	// it — including the node token — can be trusted (O11).
	Kind    *string    `json:"kind"`
	Node    *string    `json:"node"`
	Glob    *string    `json:"glob"`
	Diagram string     `json:"diagram"`
	Source  sourceJSON `json:"source"`
	Detail  string     `json:"detail"`
	Raw     *string    `json:"raw"`
	// Candidates is populated when the ID suffix-matched more than one node
	// (O8). It is the fix, not just the diagnosis: qualify with one of these.
	Candidates []string `json:"candidates"`
}

func writeJSON(w io.Writer, v *View) error {
	r := v.Report
	doc := document{
		Version:    report.SchemaVersion,
		Kind:       string(v.Kind),
		Diagrams:   emptyIfNil(r.Diagrams),
		Disabled:   make([]string, 0, len(r.Disabled)),
		Summary:    summarize(r),
		Nodes:      make([]nodeJSON, 0, len(v.Nodes)),
		Overlaps:   make([]overlapJSON, 0, len(v.Overlaps)),
		Unresolved: make([]unresolvedJSON, 0, len(r.Unresolved)),
	}
	if v.Kind == KindNode {
		doc.Query = optional(v.Query)
	}
	for _, c := range r.Disabled {
		doc.Disabled = append(doc.Disabled, string(c))
	}
	withFiles := v.Kind == KindNode
	for _, n := range v.Nodes {
		doc.Nodes = append(doc.Nodes, nodeDoc(n, withFiles))
	}
	for _, o := range v.Overlaps {
		e := overlapJSON{Path: o.Path, Nodes: make([]claimJSON, 0, len(o.Claims))}
		for _, c := range o.Claims {
			e.Nodes = append(e.Nodes, claimJSON{Node: c.Node, Glob: c.Glob, Source: source(c.Source)})
		}
		doc.Overlaps = append(doc.Overlaps, e)
	}
	for _, u := range r.Unresolved {
		doc.Unresolved = append(doc.Unresolved, unresolvedJSON{
			Kind:       optional(string(u.Kind)),
			Node:       optional(u.Node),
			Glob:       optional(u.Glob),
			Diagram:    u.Diagram,
			Source:     source(u.Source),
			Detail:     u.Detail,
			Raw:        optional(u.Raw),
			Candidates: emptyIfNil(u.Candidates),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

func nodeDoc(n *Node, withFiles bool) nodeJSON {
	out := nodeJSON{
		ID:         n.ID,
		Name:       n.Name,
		Diagram:    n.Diagram,
		Label:      n.Label,
		Shape:      n.Shape,
		Parent:     optional(n.Parent),
		Line:       n.Line,
		Status:     string(n.Status),
		Files:      n.Files(),
		Children:   emptyIfNil(n.Children),
		Bindings:   make([]bindingJSON, 0, len(n.Bindings)),
		Marks:      make([]markJSON, 0, len(n.Marks)),
		Violations: make([]violationJSON, 0, len(n.Violations)),
	}
	for _, b := range n.Bindings {
		e := bindingJSON{Glob: b.Glob, Source: source(b.Source), Matches: b.Matches()}
		if withFiles {
			e.Files = emptyIfNil(b.Files)
		}
		out.Bindings = append(out.Bindings, e)
	}
	for _, m := range n.Marks {
		out.Marks = append(out.Marks, markJSON{
			Kind:   string(m.Kind),
			Reason: optional(m.Reason),
			Source: source(m.Source),
		})
	}
	for _, v := range n.Violations {
		out.Violations = append(out.Violations, violationJSON{
			Code:     string(v.Code),
			Severity: string(v.Severity),
			Node:     optional(v.Node),
			Path:     optional(v.Path),
			Source:   source(v.Source),
			Detail:   v.Detail,
			Hint:     v.Hint,
		})
	}
	return out
}

func summarize(r *Report) summaryJSON {
	return summaryJSON{
		Nodes:      r.Counts.Nodes,
		Bound:      r.Counts.Status[StatusBound],
		External:   r.Counts.Status[StatusExternal],
		Infra:      r.Counts.Status[StatusInfra],
		Ignored:    r.Counts.Status[StatusIgnored],
		Container:  r.Counts.Status[StatusContainer],
		Unbound:    r.Counts.Status[StatusUnbound],
		Bindings:   r.Counts.Bindings,
		Files:      r.Counts.Files,
		Overlaps:   r.Counts.Overlaps,
		Unresolved: r.Counts.Unresolved,
		Failures:   r.Counts.Failures,
		Warnings:   r.Counts.Warnings,
	}
}

func source(p directive.Position) sourceJSON {
	return sourceJSON{File: p.File, Line: p.Line}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
