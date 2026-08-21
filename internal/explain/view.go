package explain

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/timimsms/trestle/internal/report"
)

// Kind is which of the three questions a view answers. It is carried into the
// JSON payload so a consumer can tell an inventory from a single-node lookup
// without inferring it from the length of an array.
type Kind string

// The three views. There are three views and one command: `--overlaps` is a
// flag precisely so that the top-level surface stays at four commands.
const (
	KindInventory Kind = "inventory"
	KindNode      Kind = "node"
	KindOverlaps  Kind = "overlaps"
)

// View is one rendering of a [Report]: the whole inventory, one node (or the
// several a written ID turned out to name), or the overlaps.
//
// Report is always the full report even when Nodes holds one entry. The node is
// the answer to the question; the report is the context it was answered in, and
// the counts and the disabled codes belong to the repo either way.
type View struct {
	Kind     Kind
	Query    string // the node ID as written, for KindNode
	Report   *Report
	Nodes    []*Node
	Overlaps []Overlap
}

// Inventory is `trestle explain` with no argument: every node the tool parsed,
// with its binding status.
//
// This is the view the dogfooding trials asked for by name. Without it the node
// set is only observable through the violations it fails to produce, and
// "`--strict` reported nothing, so my tooltip did not spawn a phantom node" is
// an inference nobody should have to make.
func (r *Report) Inventory() *View {
	return &View{Kind: KindInventory, Report: r, Nodes: r.Nodes, Overlaps: r.Overlaps}
}

// NodeView is `trestle explain <node_id>`. The ID resolves per O8, and an
// ambiguous one yields every candidate rather than a choice.
func (r *Report) NodeView(query string) *View {
	found := r.Find(query)
	return &View{
		Kind:     KindNode,
		Query:    query,
		Report:   r,
		Nodes:    found,
		Overlaps: r.OverlapsFor(found),
	}
}

// OverlapView is `trestle explain --overlaps`.
func (r *Report) OverlapView() *View {
	return &View{Kind: KindOverlaps, Report: r, Overlaps: r.Overlaps}
}

// Found reports whether a node lookup resolved to anything. It is false only
// for [KindNode].
func (v *View) Found() bool { return v.Kind != KindNode || len(v.Nodes) > 0 }

// Ambiguous reports whether the written ID named more than one node.
func (v *View) Ambiguous() bool { return v.Kind == KindNode && len(v.Nodes) > 1 }

// Write renders the view.
func Write(w io.Writer, v *View, format report.Format) error {
	switch format {
	case report.FormatJSON:
		return writeJSON(w, v)
	case report.FormatHuman:
		return writeHuman(w, v)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// UnknownNodeError reports that a written node ID resolved to nothing.
//
// This is the one thing `explain` exits non-zero for, and it is exit 2 — a tool
// error — not a finding. The command was asked to describe a node and there is
// no such node, which is the same class of mistake as an unknown `--format`.
// Reporting it as a clean exit would let an agent that just misspelled the ID of
// the node it renamed read the silence as confirmation.
type UnknownNodeError struct {
	Query    string
	Diagrams []string
}

func (e *UnknownNodeError) Error() string {
	where := "any parsed diagram"
	if len(e.Diagrams) > 0 {
		where = strings.Join(e.Diagrams, ", ")
	}
	return fmt.Sprintf("no node %q in %s\n"+
		"  hint: run `trestle explain` with no argument to list every node the tool parsed",
		e.Query, where)
}

// claimGroup is a set of nodes that claim the same paths, which is the shape a
// human reads overlaps in: the finding is "these two boxes disagree about who
// owns this", and one line per path would bury it under a directory's worth of
// repetition.
type claimGroup struct {
	Nodes  []string
	Claims []Claim
	Paths  []string
}

// groupOverlaps collapses per-path overlaps into one entry per set of claiming
// nodes, preserving path order within each group.
func groupOverlaps(os []Overlap) []claimGroup {
	var order []string
	byKey := map[string]*claimGroup{}
	for _, o := range os {
		ids := make([]string, 0, len(o.Claims))
		for _, c := range o.Claims {
			ids = append(ids, c.Node)
		}
		key := strings.Join(ids, "\x00")
		g, ok := byKey[key]
		if !ok {
			g = &claimGroup{Nodes: ids}
			byKey[key] = g
			order = append(order, key)
		}
		g.Paths = append(g.Paths, o.Path)
		for _, c := range o.Claims {
			if !hasClaim(g.Claims, c) {
				g.Claims = append(g.Claims, c)
			}
		}
	}
	out := make([]claimGroup, 0, len(order))
	for _, key := range order {
		g := byKey[key]
		sort.SliceStable(g.Claims, func(i, j int) bool {
			if g.Claims[i].Node != g.Claims[j].Node {
				return g.Claims[i].Node < g.Claims[j].Node
			}
			return g.Claims[i].Glob < g.Claims[j].Glob
		})
		out = append(out, *g)
	}
	return out
}

func hasClaim(cs []Claim, c Claim) bool {
	for _, x := range cs {
		if x.Node == c.Node && x.Glob == c.Glob && x.Source == c.Source {
			return true
		}
	}
	return false
}
