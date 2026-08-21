// Package check is the check engine — the product. Everything else in Trestle
// is packaging around this one function.
//
// # Purity
//
// Check is a pure function of (listing, nodes, directives, config). It performs
// no I/O of any kind: the filesystem walk happened once, in internal/walk, and
// the listing is passed in. That is not a stylistic preference. It is what
// makes the engine testable without a fixture tree, cheap to call in a loop,
// and safe to reason about — and it is why this package must never import
// internal/walk, os, or io/fs. integration.TestCheckIsIOFree enforces it.
//
// The same reasoning explains [Entry], which mirrors walk.Entry field for
// field: importing walk for one two-field struct would drag io/fs across the
// seam. integration.TestCheckEntryMirrorsWalkEntry pins the two shapes together
// so the copy cannot drift.
//
// # What it decides, and what it does not
//
// Check returns violations, each carrying a severity resolved from config. It
// does not compute an exit code, does not format anything, and knows nothing
// about --strict. Those are the CLI's job (Phase 4); keeping them out is what
// keeps this function a value-in, value-out unit.
//
// # The taxonomy is closed at five
//
// ORPHAN, UNMAPPED, DANGLING, UNBOUND, SYNTAX. There is no sixth code and new
// failure modes fold into an existing one — a zero-match `discover:` rule is
// reported as ORPHAN, not as a new kind. Overlapping bindings get no code at
// all (L12); they surface through `explain --overlaps`.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// Code is a violation code. The set is closed at five; see [Codes].
type Code string

// The five violation codes. Do not add a sixth (GAMEPLAN §6).
const (
	CodeOrphan   Code = "ORPHAN"
	CodeUnmapped Code = "UNMAPPED"
	CodeDangling Code = "DANGLING"
	CodeUnbound  Code = "UNBOUND"
	CodeSyntax   Code = "SYNTAX"
)

// Codes lists every violation code, in the order output should present them:
// SYNTAX and DANGLING first because a malformed or stale directive explains the
// violations underneath it.
var Codes = []Code{CodeSyntax, CodeDangling, CodeOrphan, CodeUnmapped, CodeUnbound}

// Severity is an alias for config.Severity rather than a second declaration of
// the same three values. config restates the five *codes* as strings because it
// cannot import this package; it must not also restate the severities, and a
// type alias means `severity:` overrides cannot silently stop applying.
type Severity = config.Severity

// Entry is one path in the listing. It mirrors walk.Entry exactly.
//
// IsDir is load-bearing, not metadata: `discover:` rules name directories and
// `@bind` globs name files, and the trailing slash a discover rule carries only
// matches once this flag says a slash may be synthesized.
type Entry struct {
	// Path is repo-root-relative, slash-separated, with no trailing slash on
	// directories and no leading "./".
	Path string
	// IsDir reports whether the entry is a directory.
	IsDir bool
}

// Diagram pairs one .d2 file's node set with the directives scanned from it.
//
// The pairing is per-file on purpose. Node IDs are scoped to their diagram: two
// diagrams may each declare `svc_billing`, and merging them would turn every
// unqualified directive in both into an ambiguous-suffix SYNTAX error. Bindings
// still pool repo-wide for UNMAPPED coverage — code owned by a node in
// data-flow.d2 is owned — which is the reason Check takes all the diagrams at
// once instead of being called once per file.
type Diagram struct {
	Nodes      *nodes.Diagram
	Directives directive.Result
}

// Input is everything Check needs. Every field is data; nothing is opened.
type Input struct {
	// Files is the single listing from internal/walk, with `exclude:` already
	// pruned. Sorted bytewise by path; Check re-sorts a copy if it is not.
	Files []Entry
	// Diagrams holds one entry per configured .d2 file.
	Diagrams []Diagram
	// Config is the validated .trestle.yml. A nil Config is treated as an
	// empty one with default severities.
	Config *config.Config
}

// Violation is one finding. Every field but Hint is evidence; Hint is the
// contract — a failing check that does not say what to type is one people learn
// to route around, and Phase 4 golden-tests it.
type Violation struct {
	Code     Code
	Severity Severity
	// Node is the node ID the violation is about, or "" when it is about a
	// path or a config entry.
	Node string
	// Path is the filesystem path, glob or config entry the violation is
	// about, or "" when it is about a node.
	Path string
	// Source locates the offending directive, node declaration or config file.
	Source directive.Position
	// Detail states what is wrong, in one line, without repeating the target.
	Detail string
	// Hint is a runnable next step.
	Hint string
}

// Target is the thing the violation is about: the node ID when there is one,
// otherwise the path. It is what report output and fixture EXPECTED files key
// on.
func (v Violation) Target() string {
	switch {
	case v.Node != "":
		return v.Node
	case v.Path != "":
		return v.Path
	default:
		return v.Source.String()
	}
}

// Failing reports whether the violation's severity is `fail`. It is a fact
// about severity, not an exit code — the caller decides consequences, including
// what --strict does to warnings.
func (v Violation) Failing() bool { return v.Severity == config.SeverityFail }

func (v Violation) String() string {
	return fmt.Sprintf("%s %s: %s", v.Code, v.Target(), v.Detail)
}

// Check runs every check against one listing and returns the violations, in a
// deterministic order. It never returns nil for a clean run's sake — an empty
// slice and nil are both "clean" and callers should test length.
func Check(in Input) []Violation {
	cfg := in.Config
	if cfg == nil {
		cfg = &config.Config{Severity: config.DefaultSeverity()}
	}
	c := &checker{
		cfg: cfg,
		ix:  newIndex(in.Files),
	}
	c.covered = make([]bool, c.ix.len())

	// The order of these passes matters exactly once: coverage has to be fully
	// accumulated before discover units are judged, because a unit may be
	// covered by a binding declared in a different diagram.
	for i := range in.Diagrams {
		c.resolveDiagram(&in.Diagrams[i])
	}
	c.checkBindings()
	c.checkShared()
	c.checkDiscover()
	c.checkUnbound()

	c.sortViolations()
	return c.out
}

// --- internals ----------------------------------------------------------

type checker struct {
	cfg     *config.Config
	ix      *index
	covered []bool
	out     []Violation

	binds []resolvedBind
	diags []*resolvedDiagram
}

// resolvedBind is a @bind that survived O11: its node resolved to exactly one
// AST node, so it is allowed to participate in the rest of the checks.
type resolvedBind struct {
	dir  directive.Directive
	node string
	diag *resolvedDiagram
}

type resolvedDiagram struct {
	dg *Diagram
	// accounted holds node IDs named by a valid @bind, @external, @infra or
	// @ignore. It is the input to UNBOUND.
	accounted map[string]bool
	// ignored holds node IDs named by a valid @ignore, which suppresses every
	// violation for that node.
	ignored map[string]bool
}

func (c *checker) sev(code Code) Severity {
	return c.cfg.SeverityFor(string(code))
}

// DisabledCodes returns the codes `.trestle.yml` has set to `off`, in Codes
// order. It is a pure function of config and takes no part in checking.
//
// It exists because `off` is the one place a code disappears entirely. A repo
// that sets `ORPHAN: off` and `UNMAPPED: off` otherwise gets `0 failures, 0
// warnings` and exit 0 from a check that inspected nothing — a green result
// that means nothing, which is the precise failure this tool exists to prevent
// and the same family as a config matching zero diagrams. Callers must report
// what was switched off alongside the result.
//
// This is deliberately not a sixth violation code. Disabling a code is legal;
// doing it invisibly is not.
func DisabledCodes(cfg *config.Config) []Code {
	if cfg == nil {
		return nil
	}
	var off []Code
	for _, code := range Codes {
		if cfg.SeverityFor(string(code)) == config.SeverityOff {
			off = append(off, code)
		}
	}
	return off
}

// emit records a violation unless its code is configured `off`. `off` is legal
// and is the one place a code disappears entirely, so callers pair the result
// with [DisabledCodes]; `explain` should surface it too.
func (c *checker) emit(v Violation) {
	v.Severity = c.sev(v.Code)
	if v.Severity == config.SeverityOff {
		return
	}
	c.out = append(c.out, v)
}

// resolveDiagram reports SYNTAX and DANGLING for one diagram and collects the
// directives that survive.
//
// O11 in one place: a directive that is SYNTAX or DANGLING is reported once and
// otherwise discarded. It does not bind, does not account for its node, is not
// ORPHAN-checked and confers no discover coverage. Half-using a directive whose
// node ID could not be trusted would invent intent, which is the wrong instinct
// in a tool built to catch stale intent.
func (c *checker) resolveDiagram(dg *Diagram) {
	rd := &resolvedDiagram{
		dg:        dg,
		accounted: map[string]bool{},
		ignored:   map[string]bool{},
	}
	c.diags = append(c.diags, rd)

	for _, se := range dg.Directives.Syntax {
		c.emit(Violation{
			Code:   CodeSyntax,
			Node:   syntaxTarget(se.Raw),
			Source: se.Source,
			Detail: se.Detail,
			Hint:   syntaxHint(se),
		})
	}

	if dg.Nodes == nil {
		return
	}

	for _, d := range dg.Directives.Directives {
		cands := dg.Nodes.Candidates(d.Node)
		switch {
		case len(cands) == 0:
			c.emit(Violation{
				Code:   CodeDangling,
				Node:   d.Node,
				Path:   d.Glob,
				Source: d.Source,
				Detail: fmt.Sprintf("%s names a node that is not in %s", d.Kind, dg.Nodes.Path),
				Hint:   danglingHint(d, dg.Nodes),
			})
			continue
		case len(cands) > 1:
			// O8: never pick one. A silent pick means a rename can quietly
			// re-point a binding at the wrong node, which is the precise bug
			// class this tool exists to catch.
			c.emit(Violation{
				Code:   CodeSyntax,
				Node:   d.Node,
				Source: d.Source,
				Detail: fmt.Sprintf("ambiguous node ID; it suffix-matches %s", strings.Join(cands, " and ")),
				Hint:   ambiguousHint(d, cands),
			})
			continue
		}

		id := cands[0]
		rd.accounted[id] = true
		if d.Kind == directive.KindIgnore {
			rd.ignored[id] = true
		}
		if d.Kind == directive.KindBind {
			c.binds = append(c.binds, resolvedBind{dir: d, node: id, diag: rd})
		}
	}
}

// checkBindings applies every surviving @bind glob to the one listing, marks
// the files it claims as covered, and reports the globs that claim nothing.
//
// @ignore suppresses this, as it suppresses every violation for its node — that
// is what the mandatory reason string buys. Two things it deliberately does not
// reach: a SYNTAX line, whose node token is precisely what O11 says cannot be
// trusted, and its own DANGLING, since an @ignore naming a node that is not
// there has nothing to suppress. Note that an ignored node's bindings still
// confer discover coverage; @ignore is a statement about the node, not a
// retraction of the code it owns.
func (c *checker) checkBindings() {
	for _, b := range c.binds {
		n := c.ix.eachFile(b.dir.Glob, func(i int) { c.covered[i] = true })
		if n > 0 || b.diag.ignored[b.node] {
			continue
		}
		c.emit(Violation{
			Code:   CodeOrphan,
			Node:   b.node,
			Path:   b.dir.Glob,
			Source: b.dir.Source,
			Detail: fmt.Sprintf("@bind %s matches 0 files", b.dir.Glob),
			Hint:   orphanHint(b.dir.Glob),
		})
	}
}

// checkShared applies `shared:` entries to the same listing.
//
// L11 is not optional and `shared` is not write-only config: an entry pointing
// at a directory that no longer exists fails the build like any other stale
// binding, and that is the only thing stopping the shared layer from quietly
// accumulating dead declarations. `exclude:` is deliberately not checked this
// way — it is a blindspot by design, and the asymmetry is the point.
//
// Shared entries also confer coverage (DESIGN §4: shared suppresses UNMAPPED).
// Code declared shared is real code with a declared owner of "nobody"; making
// it fire UNMAPPED as well would make the declaration useless.
func (c *checker) checkShared() {
	for _, entry := range c.cfg.Shared {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if n := c.ix.eachFile(entry, func(i int) { c.covered[i] = true }); n > 0 {
			continue
		}
		c.emit(Violation{
			Code:   CodeOrphan,
			Path:   entry,
			Source: directive.Position{File: c.cfg.Path},
			Detail: fmt.Sprintf("shared: %s matches 0 files", entry),
			Hint:   sharedOrphanHint(entry),
		})
	}
}

// checkDiscover reports code the diagram never learned about.
//
// O10: a unit is covered when at least one non-excluded FILE beneath it is
// matched by some binding — not when a binding matches the unit's own path.
// The path-based reading looks equivalent and is not: `@bind svc_billing
// app/services/billing/*.rb` matches every file in the directory and never the
// directory itself, so a path test would call a correctly-bound service
// UNMAPPED. False positives on ordinary authoring are what get a check
// bypassed.
func (c *checker) checkDiscover() {
	for _, rule := range c.cfg.Discover {
		if strings.TrimSpace(rule) == "" {
			continue
		}
		units := 0
		c.ix.eachUnit(rule, func(i int, e Entry) {
			units++
			c.checkUnit(i, e)
		})
		if units > 0 {
			continue
		}
		// A discover rule that matches nothing is the silent-failure mode:
		// UNMAPPED stops firing and the check passes while inspecting no code.
		// It folds into ORPHAN — a declaration that matches nothing — rather
		// than becoming a sixth code.
		c.emit(Violation{
			Code:   CodeOrphan,
			Path:   rule,
			Source: directive.Position{File: c.cfg.Path},
			Detail: fmt.Sprintf("discover: %s matches 0 directories", rule),
			Hint:   discoverOrphanHint(rule),
		})
	}
}

func (c *checker) checkUnit(i int, e Entry) {
	lo, hi := c.ix.subtree(i)
	files := 0
	for j := lo; j < hi; j++ {
		if c.ix.entries[j].IsDir {
			continue
		}
		files++
		if c.covered[j] {
			return
		}
	}

	unit := e.Path + "/"
	v := Violation{
		Code:   CodeUnmapped,
		Path:   unit,
		Source: directive.Position{File: c.cfg.Path},
		Detail: "no @bind glob covers this path",
		Hint:   unmappedHint(e.Path),
	}
	if files == 0 {
		// O10's corollary: an empty unit can never be covered, so it always
		// fires. That is a real finding, but the generic "add a @bind" hint
		// would send the author looking for code that is not there.
		v.Detail = "no files beneath this path for a @bind glob to cover"
		v.Hint = emptyUnitHint(e.Path)
	}
	c.emit(v)
}

// checkUnbound reports nodes of unknown provenance.
//
// O9, stated as the single rule the two halves of GAMEPLAN §3 add up to: a
// container never emits UNBOUND. If every descendant is accounted for the
// container is a grouping device and there is nothing to report; if some
// descendant is not, that descendant reports it. Never both — one modeling gap
// produces one warning. Leaves are unaffected, which is why `tenant` in the
// worked example correctly warns.
//
// A container may still carry its own @bind, and if it does that binding is
// ORPHAN-checked like any other; ORPHAN and UNBOUND are independent.
func (c *checker) checkUnbound() {
	for _, rd := range c.diags {
		if rd.dg.Nodes == nil {
			continue
		}
		for _, id := range rd.dg.Nodes.IDs {
			if rd.accounted[id] {
				continue
			}
			n, ok := rd.dg.Nodes.Node(id)
			if !ok || n.IsContainer() {
				continue
			}
			c.emit(Violation{
				Code:   CodeUnbound,
				Node:   id,
				Source: directive.Position{File: rd.dg.Nodes.Path, Line: n.Line},
				Detail: "no @bind, @external, @infra or @ignore",
				Hint:   unboundHint(id),
			})
		}
	}
}

// codeRank orders codes for output: the directive-level problems that explain
// the others come first.
func codeRank(c Code) int {
	for i, k := range Codes {
		if k == c {
			return i
		}
	}
	return len(Codes)
}

// sortViolations makes the result reproducible run to run and machine to
// machine. Grouping by source file and line is also the shape DESIGN §5's human
// output wants.
func (c *checker) sortViolations() {
	sort.SliceStable(c.out, func(i, j int) bool {
		a, b := c.out[i], c.out[j]
		if a.Source.File != b.Source.File {
			return a.Source.File < b.Source.File
		}
		if a.Source.Line != b.Source.Line {
			return a.Source.Line < b.Source.Line
		}
		if ra, rb := codeRank(a.Code), codeRank(b.Code); ra != rb {
			return ra < rb
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.Path < b.Path
	})
}
