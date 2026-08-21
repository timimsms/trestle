package explain

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
	"github.com/timimsms/trestle/internal/report"
)

// These tests are I/O-free, like the engine's. `explain` is a pure function of
// the same tuple, and a test that needed a fixture tree would mean the seam had
// moved.

// --- helpers ------------------------------------------------------------

// tree expands file paths into the listing walk would produce: every file plus
// every ancestor directory, sorted.
func tree(files ...string) []check.Entry {
	seen := map[string]bool{}
	var out []check.Entry
	add := func(p string, dir bool) {
		if seen[p] {
			return
		}
		seen[p] = true
		out = append(out, check.Entry{Path: p, IsDir: dir})
	}
	for _, f := range files {
		add(f, false)
		for i := strings.LastIndexByte(f, '/'); i > 0; i = strings.LastIndexByte(f, '/') {
			f = f[:i]
			add(f, true)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func diagram(t *testing.T, path, src string) check.Diagram {
	t.Helper()
	d, err := nodes.Parse(path, []byte(src))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return check.Diagram{Nodes: d, Directives: directive.Parse(path, []byte(src))}
}

func cfg(mutate func(*config.Config)) *config.Config {
	c := &config.Config{
		Version:  1,
		Severity: config.DefaultSeverity(),
		Root:     "/repo",
		Path:     "/repo/.trestle.yml",
	}
	if mutate != nil {
		mutate(c)
	}
	return c
}

// build is the common case: one diagram, one listing, default config.
func build(t *testing.T, src string, files []check.Entry) *Report {
	t.Helper()
	return Build(check.Input{
		Files:    files,
		Diagrams: []check.Diagram{diagram(t, "system.d2", src)},
		Config:   cfg(nil),
	})
}

func statusOfID(t *testing.T, r *Report, id string) Status {
	t.Helper()
	n, ok := r.Node(id)
	if !ok {
		t.Fatalf("no node %q in report", id)
	}
	return n.Status
}

// --- the inventory ------------------------------------------------------

// The inventory is the point of the command. Every node the tool parsed appears
// in it, with the one-word answer to "does Trestle know what is behind this
// box".
func TestInventoryListsEveryNodeWithItsStatus(t *testing.T) {
	src := `
# @bind     svc_billing  app/services/billing/**
# @infra    db_primary
# @external ext_stripe
# @ignore   svc_legacy "kept for the migration narrative until Q4"

platform: Platform {
  svc_billing: Billing
}
svc_legacy: Legacy
db_primary: Primary
ext_stripe: Stripe
tenant: Tenant
`
	r := build(t, src, tree("app/services/billing/billing.rb"))

	want := map[string]Status{
		"platform":             StatusContainer,
		"platform.svc_billing": StatusBound,
		"svc_legacy":           StatusIgnored,
		"db_primary":           StatusInfra,
		"ext_stripe":           StatusExternal,
		"tenant":               StatusUnbound,
	}
	if r.Counts.Nodes != len(want) {
		t.Errorf("nodes = %d, want %d", r.Counts.Nodes, len(want))
	}
	for id, status := range want {
		if got := statusOfID(t, r, id); got != status {
			t.Errorf("%s: status = %q, want %q", id, got, status)
		}
	}
	if r.Counts.Status[StatusUnbound] != 1 {
		t.Errorf("unbound count = %d, want 1", r.Counts.Status[StatusUnbound])
	}
}

// The complaint this command was built for: a `;` in a tooltip is a statement
// separator in D2, and the prose after it becomes a node. It was invisible
// before — an author could only infer its absence from a clean --strict run.
// Here it is an entry in a list.
func TestPhantomNodeFromASemicolonIsVisibleInTheInventory(t *testing.T) {
	src := `
# @bind svc_billing app/services/billing/**

svc_billing: Billing {
  tooltip: the ledger; the fast one
}
`
	r := build(t, src, tree("app/services/billing/billing.rb"))

	var phantom []string
	for _, n := range r.Nodes {
		if n.Status == StatusUnbound {
			phantom = append(phantom, n.ID)
		}
	}
	if len(phantom) != 1 {
		t.Fatalf("unbound nodes = %v, want exactly the phantom", phantom)
	}
	if !strings.Contains(phantom[0], "fast one") {
		t.Errorf("phantom node is %q, want the prose after the semicolon", phantom[0])
	}
}

// Match counts, not just globs. "matches 0 files" is the whole point: it is the
// ORPHAN case, and it is the answer whether or not ORPHAN is switched on.
func TestBindingsCarryTheirCurrentMatchCount(t *testing.T) {
	src := `
# @bind svc_billing   app/services/billing/**
# @bind svc_billing   app/models/invoice*.rb
# @bind svc_reporting app/services/reporting/**

svc_billing: Billing
svc_reporting: Reporting
`
	r := build(t, src, tree(
		"app/services/billing/billing.rb",
		"app/services/billing/invoice_builder.rb",
		"app/models/invoice.rb",
		"app/models/work_order.rb",
	))

	billing, _ := r.Node("svc_billing")
	if len(billing.Bindings) != 2 {
		t.Fatalf("bindings = %d, want 2", len(billing.Bindings))
	}
	if got := billing.Bindings[0].Matches(); got != 2 {
		t.Errorf("first glob matches %d, want 2", got)
	}
	if got := billing.Bindings[1].Matches(); got != 1 {
		t.Errorf("second glob matches %d, want 1", got)
	}
	if got := billing.Files(); got != 3 {
		t.Errorf("node claims %d distinct files, want 3", got)
	}

	reporting, _ := r.Node("svc_reporting")
	if got := reporting.Bindings[0].Matches(); got != 0 {
		t.Errorf("orphan glob matches %d, want 0", got)
	}
	if got := len(reporting.Violations); got != 1 || reporting.Violations[0].Code != check.CodeOrphan {
		t.Errorf("violations = %+v, want one ORPHAN", reporting.Violations)
	}
}

// A node's own two globs overlapping each other is not an overlap: the file has
// one owner. Reporting it would bury the real finding, which is two *nodes*
// disagreeing.
func TestNodeOverlappingItselfIsNotAnOverlap(t *testing.T) {
	src := `
# @bind svc_billing app/services/billing/**
# @bind svc_billing app/services/billing/*.rb

svc_billing: Billing
`
	r := build(t, src, tree("app/services/billing/billing.rb"))
	if len(r.Overlaps) != 0 {
		t.Errorf("overlaps = %+v, want none", r.Overlaps)
	}
	n, _ := r.Node("svc_billing")
	if got := n.Files(); got != 1 {
		t.Errorf("distinct files = %d, want 1", got)
	}
}

// A node's violations are filed under it, and they arrive with their hints
// intact. `explain` is where someone comes to find out what to type.
func TestNodeViolationsKeepTheirHints(t *testing.T) {
	src := `
# @bind svc_reporting app/services/reporting/**

svc_reporting: Reporting
tenant: Tenant
`
	r := build(t, src, tree("app/services/billing/billing.rb"))
	for _, id := range []string{"svc_reporting", "tenant"} {
		n, _ := r.Node(id)
		if len(n.Violations) == 0 {
			t.Fatalf("%s: expected a violation", id)
		}
		for _, v := range n.Violations {
			if strings.TrimSpace(v.Hint) == "" {
				t.Errorf("%s: %s carries no hint", id, v.Code)
			}
		}
	}
}

// --- O8, from the debugging side ---------------------------------------

// Find is O8 and nothing else: exact wins, otherwise segment-boundary suffix.
// It must be the same rule `check` resolves directives with, which is why it
// asks nodes.Candidates rather than reimplementing it.
func TestFindImplementsO8(t *testing.T) {
	src := `
platform: Platform {
  svc_work_orders: Work Orders
}
svc_work_orders: Top Level
`
	r := build(t, src, nil)

	cases := []struct {
		query string
		want  []string
	}{
		{"platform.svc_work_orders", []string{"platform.svc_work_orders"}},
		// Exact wins outright: the top-level node is named exactly, so the
		// nested one is not also a candidate.
		{"svc_work_orders", []string{"svc_work_orders"}},
		{"orders", nil}, // suffix must land on a segment boundary
		{"nope", nil},
	}
	for _, c := range cases {
		got := idsOf(r.Find(c.query))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("Find(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

// The ambiguous case is the one worth showing: `check` reports it as SYNTAX and
// refuses to pick, so the command you run to debug that must list every
// candidate rather than choosing one.
func TestAmbiguousIDListsEveryCandidate(t *testing.T) {
	src := `
billing: Billing Domain {
  svc_search: Invoice Search
}
support: Support Domain {
  svc_search: Ticket Search
}
`
	r := build(t, src, nil)
	v := r.NodeView("svc_search")
	if !v.Ambiguous() {
		t.Fatal("svc_search must be ambiguous")
	}
	if got := idsOf(v.Nodes); strings.Join(got, ",") != "billing.svc_search,support.svc_search" {
		t.Errorf("candidates = %v", got)
	}
	if !v.Found() {
		t.Error("an ambiguous ID resolved to nodes and must count as found")
	}
}

// Find and the engine must agree about every ID anyone might type, because a
// debugging command that resolves IDs differently from the thing it debugs is
// worse than none.
func TestFindAgreesWithTheEngineResolution(t *testing.T) {
	src := `
billing: Billing {
  svc_search: Invoice Search
  svc_billing: Billing
}
support: Support {
  svc_search: Ticket Search
}
tenant: Tenant
`
	dg := diagram(t, "system.d2", src)
	r := Build(check.Input{Diagrams: []check.Diagram{dg}, Config: cfg(nil)})

	for _, query := range []string{
		"svc_search", "svc_billing", "billing.svc_search", "tenant", "billing", "orders", "",
	} {
		want := dg.Nodes.Candidates(query)
		got := idsOf(r.Find(query))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Find(%q) = %v, nodes.Candidates says %v", query, got, want)
		}
	}
}

// Node IDs are scoped per diagram, so the same ID in two files is two nodes and
// that is ambiguity worth showing rather than a reason to guess.
func TestSameIDInTwoDiagramsResolvesToBoth(t *testing.T) {
	src := "svc_billing: Billing\n"
	r := Build(check.Input{
		Diagrams: []check.Diagram{
			diagram(t, "system.d2", src),
			diagram(t, "data-flow.d2", src),
		},
		Config: cfg(nil),
	})
	found := r.Find("svc_billing")
	if len(found) != 2 {
		t.Fatalf("found %d nodes, want one per diagram", len(found))
	}
	if found[0].Diagram == found[1].Diagram {
		t.Error("both candidates came from the same diagram")
	}
}

func idsOf(ns []*Node) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

// --- unresolved directives ---------------------------------------------

// A binding that resolved to nothing is invisible among the nodes. Without this
// section the inventory would look complete while a dead `@bind` sat in the
// file, which is the failure mode the whole tool is about.
func TestUnresolvedDirectivesAreListedSeparately(t *testing.T) {
	src := `
# @bind svc_ghost   app/legacy/**
# @bind svc_search  app/search/**
# @bind

billing: Billing {
  svc_search: Invoice Search
}
support: Support {
  svc_search: Ticket Search
}
`
	r := build(t, src, tree("app/legacy/old.rb", "app/search/index.rb"))
	if len(r.Unresolved) != 3 {
		t.Fatalf("unresolved = %d, want 3: %+v", len(r.Unresolved), r.Unresolved)
	}

	var dangling, ambiguous, malformed *Unresolved
	for i := range r.Unresolved {
		u := &r.Unresolved[i]
		switch {
		case u.Kind == "":
			malformed = u
		case len(u.Candidates) > 0:
			ambiguous = u
		default:
			dangling = u
		}
	}
	if dangling == nil || dangling.Node != "svc_ghost" {
		t.Errorf("dangling = %+v, want svc_ghost", dangling)
	}
	if ambiguous == nil || len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous = %+v, want two candidates", ambiguous)
	}
	if malformed == nil || malformed.Raw == "" {
		t.Errorf("malformed = %+v, want the raw line quoted back", malformed)
	}
	if r.Counts.Unresolved != 3 {
		t.Errorf("counts.Unresolved = %d, want 3", r.Counts.Unresolved)
	}
	// Source order: the reader is about to open the file.
	for i := 1; i < len(r.Unresolved); i++ {
		if r.Unresolved[i-1].Source.Line > r.Unresolved[i].Source.Line {
			t.Errorf("unresolved is not in source order: %v", r.Unresolved)
		}
	}
}

// O11: an unresolvable directive participates in nothing else. It must not
// bind, and it must not make its node look accounted for.
func TestUnresolvableDirectiveDoesNotBindAnything(t *testing.T) {
	src := `
# @bind svc_ghost app/legacy/**

tenant: Tenant
`
	r := build(t, src, tree("app/legacy/old.rb"))
	if r.Counts.Bindings != 0 {
		t.Errorf("bindings = %d, want 0", r.Counts.Bindings)
	}
	if got := statusOfID(t, r, "tenant"); got != StatusUnbound {
		t.Errorf("tenant status = %q, want unbound", got)
	}
}

// --- overlaps -----------------------------------------------------------

// L12 made overlap legal and gave it no violation code on the promise that it
// would be visible here. This is the promise being kept, and it stays
// informational: nothing about it is a failure.
func TestOverlapsListPathsClaimedByMoreThanOneNode(t *testing.T) {
	src := `
# @bind svc_invoicing app/services/billing/**
# @bind svc_payments  app/services/billing/payment_processor.rb

svc_invoicing: Invoicing
svc_payments: Payments
`
	r := build(t, src, tree(
		"app/services/billing/billing.rb",
		"app/services/billing/invoice_generator.rb",
		"app/services/billing/payment_processor.rb",
	))

	if len(r.Overlaps) != 1 {
		t.Fatalf("overlaps = %+v, want exactly the shared path", r.Overlaps)
	}
	o := r.Overlaps[0]
	if o.Path != "app/services/billing/payment_processor.rb" {
		t.Errorf("overlap path = %q", o.Path)
	}
	if len(o.Claims) != 2 || o.Claims[0].Node != "svc_invoicing" || o.Claims[1].Node != "svc_payments" {
		t.Errorf("claims = %+v, want both nodes in ID order", o.Claims)
	}
	// The overlap fixture exits 0 and reports nothing. Overlap is not a
	// violation and this must never start producing one.
	if r.Counts.Failures != 0 || r.Counts.Warnings != 0 {
		t.Errorf("overlap produced %d failures and %d warnings; it is informational",
			r.Counts.Failures, r.Counts.Warnings)
	}
}

func TestOverlapsAreSortedByPath(t *testing.T) {
	src := `
# @bind svc_a app/services/billing/**
# @bind svc_b app/services/billing/**

svc_a: A
svc_b: B
`
	r := build(t, src, tree(
		"app/services/billing/c.rb",
		"app/services/billing/a.rb",
		"app/services/billing/b.rb",
	))
	got := make([]string, 0, len(r.Overlaps))
	for _, o := range r.Overlaps {
		got = append(got, o.Path)
	}
	want := []string{"app/services/billing/a.rb", "app/services/billing/b.rb", "app/services/billing/c.rb"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("overlaps = %v, want %v", got, want)
	}
}

// --- severity: off ------------------------------------------------------

// A disabled code is the one thing `check` structurally cannot report. It has to
// be visible here, and the evidence underneath it — the zero match count — has
// to survive too, because that is what the reader needs when the violation is
// gone.
func TestDisabledCodesAreSurfacedAndTheEvidenceSurvives(t *testing.T) {
	src := `
# @bind svc_reporting app/services/reporting/**

svc_reporting: Reporting
`
	r := Build(check.Input{
		Files:    tree("app/services/billing/billing.rb"),
		Diagrams: []check.Diagram{diagram(t, "system.d2", src)},
		Config: cfg(func(c *config.Config) {
			c.Severity[config.CodeOrphan] = config.SeverityOff
		}),
	})

	if len(r.Disabled) != 1 || r.Disabled[0] != check.CodeOrphan {
		t.Fatalf("disabled = %v, want [ORPHAN]", r.Disabled)
	}
	n, _ := r.Node("svc_reporting")
	if len(n.Violations) != 0 {
		t.Errorf("ORPHAN is off; the engine must report nothing: %+v", n.Violations)
	}
	if got := n.Bindings[0].Matches(); got != 0 {
		t.Errorf("match count = %d, want 0 — the evidence must outlive the violation", got)
	}

	var buf bytes.Buffer
	if err := Write(&buf, r.Inventory(), report.FormatHuman); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ORPHAN is off") {
		t.Errorf("human output does not say ORPHAN was switched off:\n%s", buf.String())
	}
}

// The note prints in every view, because the fact is about the run and not
// about which question was asked of it.
func TestDisabledNotePrintsInEveryView(t *testing.T) {
	src := "# @bind svc_a app/a/**\n\nsvc_a: A\n"
	r := Build(check.Input{
		Files:    tree("app/a/a.rb"),
		Diagrams: []check.Diagram{diagram(t, "system.d2", src)},
		Config: cfg(func(c *config.Config) {
			c.Severity[config.CodeUnbound] = config.SeverityOff
		}),
	})
	for name, v := range map[string]*View{
		"inventory": r.Inventory(),
		"node":      r.NodeView("svc_a"),
		"overlaps":  r.OverlapView(),
	} {
		var buf bytes.Buffer
		if err := Write(&buf, v, report.FormatHuman); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "UNBOUND is off") {
			t.Errorf("%s view hides the disabled code:\n%s", name, buf.String())
		}
	}
}

// --- output -------------------------------------------------------------

// The JSON document is what an agent parses. Its shape is fixed: version 1, the
// same three arrays whichever question was asked, and null only where a value is
// genuinely absent.
func TestJSONShapeIsStableAcrossViews(t *testing.T) {
	src := `
# @bind svc_invoicing app/services/billing/**
# @bind svc_payments  app/services/billing/payment_processor.rb

svc_invoicing: Invoicing
svc_payments: Payments
`
	r := build(t, src, tree(
		"app/services/billing/billing.rb",
		"app/services/billing/payment_processor.rb",
	))

	for name, v := range map[string]*View{
		"inventory": r.Inventory(),
		"node":      r.NodeView("svc_payments"),
		"overlaps":  r.OverlapView(),
	} {
		var buf bytes.Buffer
		if err := Write(&buf, v, report.FormatJSON); err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, buf.String())
		}
		if doc["version"] != float64(report.SchemaVersion) {
			t.Errorf("%s: version = %v, want %d", name, doc["version"], report.SchemaVersion)
		}
		if doc["kind"] != name {
			t.Errorf("%s: kind = %v", name, doc["kind"])
		}
		for _, key := range []string{"diagrams", "disabled", "nodes", "overlaps", "unresolved"} {
			if _, ok := doc[key].([]any); !ok {
				t.Errorf("%s: %s is %v, want an array in every view", name, key, doc[key])
			}
		}
		// summary describes the repo, whichever question was asked.
		sum, ok := doc["summary"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no summary", name)
		}
		if sum["nodes"] != float64(2) || sum["overlaps"] != float64(1) {
			t.Errorf("%s: summary = %v, want the repo-wide counts", name, sum)
		}
	}
}

// null and [] are different answers: [] means the glob claims nothing — the
// ORPHAN case — and null means the file list was not part of this view.
func TestBindingFilesAreNullUnlessTheViewAsksForThem(t *testing.T) {
	src := "# @bind svc_a app/a/**\n\nsvc_a: A\n"
	r := build(t, src, tree("app/a/a.rb"))

	node := decode(t, r.NodeView("svc_a"))
	if files := node.Nodes[0].Bindings[0].Files; len(files) != 1 {
		t.Errorf("node view: files = %v, want the list", files)
	}
	inv := decode(t, r.Inventory())
	if inv.Nodes[0].Bindings[0].Files != nil {
		t.Errorf("inventory: files = %v, want null", inv.Nodes[0].Bindings[0].Files)
	}
	if inv.Nodes[0].Bindings[0].Matches != 1 {
		t.Error("inventory must still carry the count")
	}
}

func TestEmptyGlobReportsAnEmptyFileListNotNull(t *testing.T) {
	src := "# @bind svc_a app/nope/**\n\nsvc_a: A\n"
	r := build(t, src, tree("app/a/a.rb"))
	doc := decode(t, r.NodeView("svc_a"))
	files := doc.Nodes[0].Bindings[0].Files
	if files == nil || len(files) != 0 {
		t.Errorf("files = %v, want an empty array", files)
	}
}

type testDoc struct {
	Nodes []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Bindings []struct {
			Glob    string   `json:"glob"`
			Matches int      `json:"matches"`
			Files   []string `json:"files"`
		} `json:"bindings"`
	} `json:"nodes"`
}

func decode(t *testing.T, v *View) testDoc {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, v, report.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var doc testDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	return doc
}

// Human output is column-aligned, so a node with nothing in its detail column
// would otherwise end in ten invisible spaces. Golden files full of trailing
// whitespace are unreviewable.
func TestHumanOutputHasNoTrailingWhitespace(t *testing.T) {
	src := `
# @bind     svc_a  app/a/**
# @bind     svc_b  app/a/**
# @external ext_x
# @bind     ghost  app/nope/**
# @bind

svc_a: A
svc_b: B
ext_x: X
tenant: Tenant
`
	r := build(t, src, tree("app/a/a.rb"))
	for _, v := range []*View{r.Inventory(), r.NodeView("svc_a"), r.OverlapView()} {
		var buf bytes.Buffer
		if err := Write(&buf, v, report.FormatHuman); err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(buf.String(), "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("%s line %d has trailing whitespace: %q", v.Kind, i+1, line)
			}
		}
	}
}

// Every violation `explain` prints carries its hint, exactly as `check` does.
func TestHumanNodeViewPrintsTheHint(t *testing.T) {
	src := "# @bind svc_a app/nope/**\n\nsvc_a: A\n"
	r := build(t, src, tree("app/a/a.rb"))
	var buf bytes.Buffer
	if err := Write(&buf, r.NodeView("svc_a"), report.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "matches 0 files") {
		t.Errorf("the zero match count is missing:\n%s", out)
	}
	if !strings.Contains(out, "hint: ") {
		t.Errorf("no hint in:\n%s", out)
	}
}

func TestUnknownFormatIsAnError(t *testing.T) {
	r := build(t, "svc_a: A\n", nil)
	if err := Write(&bytes.Buffer{}, r.Inventory(), report.Format("yaml")); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

// An empty repo produces an empty report rather than a panic. It is also the
// shape `explain` takes on a diagram whose every node vanished.
func TestEmptyInput(t *testing.T) {
	r := Build(check.Input{Config: cfg(nil)})
	if r.Counts.Nodes != 0 || len(r.Overlaps) != 0 {
		t.Errorf("empty input produced %+v", r.Counts)
	}
	for _, v := range []*View{r.Inventory(), r.OverlapView(), r.NodeView("nothing")} {
		var buf bytes.Buffer
		if err := Write(&buf, v, report.FormatJSON); err != nil {
			t.Fatalf("%s: %v", v.Kind, err)
		}
	}
	if r.NodeView("nothing").Found() {
		t.Error("a query against an empty report must not report a find")
	}
}

// The report must not depend on map iteration order.
func TestOutputIsDeterministic(t *testing.T) {
	src := `
# @bind svc_a app/a/**
# @bind svc_b app/a/**
# @bind ghost app/a/**

svc_a: A
svc_b: B
`
	files := tree("app/a/a.rb", "app/a/b.rb")
	first := render(t, build(t, src, files))
	for i := 0; i < 5; i++ {
		if got := render(t, build(t, src, files)); got != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, got, first)
		}
	}
}

func render(t *testing.T, r *Report) string {
	t.Helper()
	var buf bytes.Buffer
	for _, v := range []*View{r.Inventory(), r.OverlapView()} {
		if err := Write(&buf, v, report.FormatJSON); err != nil {
			t.Fatal(err)
		}
	}
	return buf.String()
}
