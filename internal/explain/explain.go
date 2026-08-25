// Package explain builds the inventory `trestle explain` prints: every node the
// tool parsed, what each of its bindings currently matches, and the paths two
// nodes both claim.
//
// # Why this exists
//
// Eleven dogfooding trials produced one complaint more often than any other:
// there was no way to see what Trestle had parsed. An agent authoring a diagram
// could tell that `--strict` reported no warnings and had to *infer* from that
// silence that its tooltips had not spawned phantom nodes. Inferring the node
// set from the absence of violations is backwards, and it is the reason the node
// inventory — not `explain <node_id>` — is the center of this package.
//
// # It is a report, never a verdict
//
// Nothing here computes an exit code and nothing here decides anything. A node
// with three failing violations and a node with none are both printed; the
// caller exits 0 either way (docs/DESIGN.md §5). Overlaps in particular are
// informational by construction, which is the promise L12 made when it declined
// to spend a sixth violation code on them.
//
// # Purity, and the two things it borrows
//
// [Build] is a pure function of the same (listing, nodes, directives, config)
// tuple the engine takes — literally [check.Input] — and does no I/O. It borrows
// exactly two things from elsewhere rather than restating them:
//
//   - [check.Matcher] answers what a glob claims, so `explain` reports the match
//     set `check` is actually running on rather than a lookalike written next
//     door.
//   - [nodes.Diagram.Candidates] resolves a written node ID, so `explain <id>`
//     and `check` implement one O8, not two.
//
// Both matter for the same reason: a debugging command that quietly disagrees
// with the command it exists to debug is worse than no debugging command.
//
// # No color
//
// Human output here is plain text in every environment. `internal/report`
// colorizes because a CI log's failures need to be findable at a glance; an
// inventory is a dump an agent reads, its columns carry the structure without
// help, and a second copy of the ANSI machinery in a second package is drift
// waiting to happen for a decoration nobody asked for.
package explain

import (
	"sort"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// Status is a node's binding status: the one-word answer to "does the tool know
// what is behind this box?".
//
// It is deliberately not a violation code and never becomes one. `bound` and
// `unbound` are not pass and fail — an unbound node is a warning by default and
// an `ignored` one is silent by request. The taxonomy stays at five.
type Status string

// The six binding statuses.
const (
	// StatusBound means at least one `@bind` resolved to this node. It says
	// nothing about whether the glob matches anything; that is what the match
	// count next to it is for.
	StatusBound Status = "bound"
	// StatusExternal means `@external` — somebody else's system.
	StatusExternal Status = "external"
	// StatusInfra means `@infra` — yours, with no code in this repo.
	StatusInfra Status = "infra"
	// StatusIgnored means `@ignore` — every violation for the node is
	// suppressed, and the reason string is printed alongside so the suppression
	// has to keep justifying itself.
	StatusIgnored Status = "ignored"
	// StatusContainer means a node with children and no directive of its own.
	// O9: a container is a grouping device, so it never reports UNBOUND; an
	// unaccounted-for descendant reports on itself instead.
	StatusContainer Status = "container"
	// StatusUnbound means a leaf with no directive at all. This is the phantom
	// node a stray `;` in a tooltip produces, and finding it here is the point.
	StatusUnbound Status = "unbound"
)

// Statuses lists every status in the order counts and output present them: the
// accounted-for ones first, the gap last.
var Statuses = []Status{StatusBound, StatusExternal, StatusInfra, StatusIgnored, StatusContainer, StatusUnbound}

// Binding is one `@bind` that resolved to a node, with what its glob claims
// right now.
type Binding struct {
	// Glob is the pattern exactly as authored.
	Glob string
	// Source is the directive's line.
	Source directive.Position
	// Files holds every file the glob claims, in listing order. Empty is the
	// finding: a binding matching nothing is ORPHAN, and "matches 0 files" is
	// the sentence people come to this command to read.
	Files []string
}

// Matches is how many files the glob claims.
func (b Binding) Matches() int { return len(b.Files) }

// Mark is an `@external`, `@infra` or `@ignore` that resolved to a node. These
// account for a node without binding it to any path.
type Mark struct {
	Kind   directive.Kind
	Reason string // @ignore only
	Source directive.Position
}

// Node is one diagram node and everything known about it.
type Node struct {
	ID      string // fully qualified, e.g. "platform.svc_work_orders"
	Name    string // final segment
	Diagram string // the .d2 file it was declared in
	Label   string
	Shape   string
	Parent  string
	Line    int
	Status  Status
	// Children holds direct children, so a container's contents are visible
	// without cross-referencing the whole list.
	Children []string
	Bindings []Binding
	Marks    []Mark
	// Violations are the findings whose subject is this node. It is empty for a
	// node whose code is `off` in config: the engine cannot report what it has
	// been told not to report, which is why [Report.Disabled] is printed too.
	Violations []check.Violation
}

// Files is the number of distinct files this node's bindings claim.
func (n *Node) Files() int {
	if len(n.Bindings) == 1 {
		return len(n.Bindings[0].Files)
	}
	seen := map[string]bool{}
	for _, b := range n.Bindings {
		for _, f := range b.Files {
			seen[f] = true
		}
	}
	return len(seen)
}

// Claim is one node's claim on a path, and the binding that made it.
type Claim struct {
	Node   string
	Glob   string
	Source directive.Position
}

// Overlap is one path claimed by more than one node.
//
// L12 makes overlap legal and gives it no violation code, on the grounds that
// two nodes may honestly share a directory. That reasoning came with a
// promissory note — "surfaced via `trestle explain --overlaps`" — and this type
// is the note being paid. The same signal is what a copy-pasted glob looks like,
// so it is shown and not judged.
type Overlap struct {
	Path   string
	Claims []Claim
}

// Unresolved is a directive line that named no single node: a malformed line, a
// node that is not in the diagram, or an ID that suffix-matches two.
//
// It is listed separately from the nodes because by definition it belongs to
// none of them, and dropping it would make the inventory look complete while a
// binding sat in the file doing nothing. Candidates is populated for the
// ambiguous case (O8), which makes this the place to debug the SYNTAX `check`
// reports.
type Unresolved struct {
	Kind       directive.Kind // "" when the line did not parse at all
	Node       string         // as written, which is exactly what could not be trusted
	Glob       string
	Diagram    string
	Source     directive.Position
	Detail     string
	Raw        string // the source line, for a malformed directive
	Candidates []string
}

// Counts is the inventory in numbers. Every field is a fact about the whole
// repo, including in a single-node view — the node is the answer to the query,
// and these are the context it was answered in.
type Counts struct {
	Nodes      int
	Status     map[Status]int
	Bindings   int
	Files      int // distinct files claimed by at least one binding
	Overlaps   int
	Unresolved int
	Failures   int
	Warnings   int
}

// Report is everything `explain` knows.
type Report struct {
	// Diagrams lists every parsed .d2 path, in config order.
	Diagrams []string
	// Nodes is every node in every diagram, in declaration order within each
	// diagram — the order they appear in the file, which is the order someone
	// about to edit that file is reading.
	Nodes []*Node
	// Overlaps is sorted by path.
	Overlaps []Overlap
	// Unresolved is in source order.
	Unresolved []Unresolved
	// Disabled lists the codes `.trestle.yml` set to `off`. A silently disabled
	// code is the one thing `check` structurally cannot report, so `explain`
	// carries it into every view.
	Disabled []check.Code
	Counts   Counts

	// parsed keeps the node sets so [Report.Find] can resolve a written ID
	// through nodes.Candidates itself rather than through a second
	// implementation of O8 that would be free to drift from it.
	parsed []*nodes.Diagram
}

// Build computes the report. It is a pure function of the engine's own input
// and performs no I/O.
func Build(in check.Input) *Report {
	violations := check.Check(in)
	m := check.NewMatcher(in.Files)

	r := &Report{
		Disabled: check.DisabledCodes(in.Config),
		Counts:   Counts{Status: map[Status]int{}},
	}
	for _, s := range Statuses {
		r.Counts.Status[s] = 0
	}

	byID := make([]map[string]*Node, len(in.Diagrams))
	for i := range in.Diagrams {
		byID[i] = r.addDiagram(&in.Diagrams[i], m)
	}
	r.attach(violations, in.Diagrams, byID)
	r.finish()
	return r
}

// addDiagram materializes one diagram's nodes and resolves its directives
// against them, returning the node index so violations can be attached later.
func (r *Report) addDiagram(dg *check.Diagram, m *check.Matcher) map[string]*Node {
	index := map[string]*Node{}
	if dg.Nodes == nil {
		return index
	}
	r.Diagrams = append(r.Diagrams, dg.Nodes.Path)
	r.parsed = append(r.parsed, dg.Nodes)

	for _, id := range dg.Nodes.IDs {
		n, ok := dg.Nodes.Node(id)
		if !ok {
			continue
		}
		node := &Node{
			ID:       id,
			Name:     n.Name,
			Diagram:  dg.Nodes.Path,
			Label:    n.Label,
			Shape:    n.Shape,
			Parent:   n.Parent,
			Line:     n.Line,
			Children: n.Children,
			Status:   StatusUnbound,
		}
		if n.IsContainer() {
			node.Status = StatusContainer
		}
		index[id] = node
		r.Nodes = append(r.Nodes, node)
	}

	// The unparseable lines first, in source order with the rest: a malformed
	// directive is the likeliest explanation for a node that looks unbound.
	for _, se := range dg.Directives.Syntax {
		r.Unresolved = append(r.Unresolved, Unresolved{
			Diagram: dg.Nodes.Path,
			Source:  se.Source,
			Detail:  se.Detail,
			Raw:     se.Raw,
		})
	}

	for _, d := range dg.Directives.Directives {
		cands := dg.Nodes.Candidates(d.Node)
		if len(cands) != 1 {
			u := Unresolved{
				Kind:    d.Kind,
				Node:    d.Node,
				Glob:    d.Glob,
				Diagram: dg.Nodes.Path,
				Source:  d.Source,
				Detail:  "names a node that is not in " + dg.Nodes.Path,
			}
			if len(cands) > 1 {
				u.Candidates = cands
				u.Detail = "ambiguous node ID"
			}
			r.Unresolved = append(r.Unresolved, u)
			continue
		}
		node := index[cands[0]]
		if node == nil {
			continue
		}
		if d.Kind == directive.KindBind {
			node.Bindings = append(node.Bindings, Binding{
				Glob:   d.Glob,
				Source: d.Source,
				Files:  m.Files(d.Glob),
			})
			continue
		}
		node.Marks = append(node.Marks, Mark{Kind: d.Kind, Reason: d.Reason, Source: d.Source})
	}
	return index
}

// attach files each violation under the node it is about.
//
// The match is on the diagram *and* the ID, because node IDs are scoped per
// file: two diagrams may each declare `svc_billing` and a violation belongs to
// exactly one of them. Violations that name no node — UNMAPPED, the `shared:`
// and `discover:` ORPHANs — and those whose node token did not resolve are
// counted and not filed; the first belong to the config and the second are
// already listed as [Unresolved].
func (r *Report) attach(vs []check.Violation, diagrams []check.Diagram, byID []map[string]*Node) {
	for _, v := range vs {
		if v.Node == "" {
			continue
		}
		for i := range diagrams {
			if diagrams[i].Nodes == nil || diagrams[i].Nodes.Path != v.Source.File {
				continue
			}
			if n := byID[i][v.Node]; n != nil {
				n.Violations = append(n.Violations, v)
			}
			break
		}
	}
	for _, v := range vs {
		switch v.Severity {
		case config.SeverityFail:
			r.Counts.Failures++
		case config.SeverityWarn:
			r.Counts.Warnings++
		}
	}
}

// finish resolves each node's status, computes the overlaps, and totals the
// counts.
func (r *Report) finish() {
	claims := map[string][]Claim{}
	files := map[string]bool{}

	for _, n := range r.Nodes {
		n.Status = statusOf(n)
		r.Counts.Status[n.Status]++
		r.Counts.Bindings += len(n.Bindings)
		for _, b := range n.Bindings {
			for _, f := range b.Files {
				files[f] = true
				last := claims[f]
				// A node binding the same path twice — two globs that both
				// match it — is not an overlap. Overlap is about two *nodes*
				// disagreeing over ownership, and reporting a node against
				// itself would bury the real ones.
				if len(last) > 0 && last[len(last)-1].Node == n.ID {
					continue
				}
				claims[f] = append(last, Claim{Node: n.ID, Glob: b.Glob, Source: b.Source})
			}
		}
	}
	r.Counts.Nodes = len(r.Nodes)
	r.Counts.Files = len(files)
	r.Counts.Unresolved = len(r.Unresolved)

	// Malformed lines are collected before well-formed ones and both are needed
	// in the order they appear in the file: the reader is about to open it.
	sort.SliceStable(r.Unresolved, func(i, j int) bool {
		a, b := r.Unresolved[i], r.Unresolved[j]
		if a.Source.File != b.Source.File {
			return a.Source.File < b.Source.File
		}
		return a.Source.Line < b.Source.Line
	})

	for path, cs := range claims {
		if len(cs) < 2 {
			continue
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].Node != cs[j].Node {
				return cs[i].Node < cs[j].Node
			}
			return cs[i].Glob < cs[j].Glob
		})
		r.Overlaps = append(r.Overlaps, Overlap{Path: path, Claims: cs})
	}
	sort.Slice(r.Overlaps, func(i, j int) bool { return r.Overlaps[i].Path < r.Overlaps[j].Path })
	r.Counts.Overlaps = len(r.Overlaps)
}

// statusOf reduces a node's directives to one word.
//
// The order is a priority, not a search: `@ignore` outranks everything because a
// suppressed node's other properties are not what you need to know about it, and
// `@bind` outranks the marks because a path is a stronger statement than a
// label. A node carrying two contradictory directives still lists both under
// Marks, so nothing is hidden by the reduction.
func statusOf(n *Node) Status {
	for _, m := range n.Marks {
		if m.Kind == directive.KindIgnore {
			return StatusIgnored
		}
	}
	if len(n.Bindings) > 0 {
		return StatusBound
	}
	for _, kind := range []directive.Kind{directive.KindInfra, directive.KindExternal} {
		for _, m := range n.Marks {
			if m.Kind == kind {
				if kind == directive.KindInfra {
					return StatusInfra
				}
				return StatusExternal
			}
		}
	}
	if len(n.Children) > 0 {
		return StatusContainer
	}
	return StatusUnbound
}

// Find returns the nodes a written ID refers to, applying O8 across every
// diagram: an exact match on a fully-qualified ID wins outright, and otherwise
// every segment-boundary suffix match is a candidate.
//
// It returns all of them rather than choosing. Ambiguity is the case worth
// showing — it is the SYNTAX `check` reports, and the fix is to qualify the ID,
// which requires seeing the candidates.
// Resolution is per diagram — nodes.Candidates is the O8 rule and this asks it
// once per file — and then across diagrams by the same precedence: if any
// diagram answered with the exact ID, the suffix matches in other files are
// dropped. Two files may each declare `svc_billing`, and that is ambiguity worth
// showing, not a reason to guess.
func (r *Report) Find(id string) []*Node {
	if id == "" {
		return nil
	}
	var exact, suffix []*Node
	for _, dg := range r.parsed {
		for _, cand := range dg.Candidates(id) {
			n, ok := r.nodeIn(dg.Path, cand)
			if !ok {
				continue
			}
			if cand == id {
				exact = append(exact, n)
				continue
			}
			suffix = append(suffix, n)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return suffix
}

func (r *Report) nodeIn(diagram, id string) (*Node, bool) {
	for _, n := range r.Nodes {
		if n.Diagram == diagram && n.ID == id {
			return n, true
		}
	}
	return nil, false
}

// Node returns the node with a fully-qualified ID.
func (r *Report) Node(id string) (*Node, bool) {
	for _, n := range r.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return nil, false
}

// OverlapsFor returns the overlaps at least one of the given nodes takes part
// in, so a single-node view can say "this path is also claimed by X" without
// listing the whole repo's.
func (r *Report) OverlapsFor(ns []*Node) []Overlap {
	if len(ns) == 0 {
		return nil
	}
	want := make(map[string]bool, len(ns))
	for _, n := range ns {
		want[n.ID] = true
	}
	var out []Overlap
	for _, o := range r.Overlaps {
		for _, c := range o.Claims {
			if want[c.Node] {
				out = append(out, o)
				break
			}
		}
	}
	return out
}
