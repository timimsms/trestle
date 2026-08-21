package check

// Every test in this package runs entirely in memory. That is a rule, not a
// coincidence: internal/check is a pure function, and a test that needs a
// fixture tree on disk is testing a seam rather than the engine and belongs in
// internal/integration. integration.TestCheckIsIOFree fails the build if a test
// file here so much as imports os.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// --- helpers ------------------------------------------------------------

// listing builds a sorted listing. A path ending in "/" is a directory; the
// slash is stripped, because that is what walk emits and getting this wrong in
// the test would hide the very bug the engine has to defend against.
func listing(paths ...string) []Entry {
	out := make([]Entry, 0, len(paths))
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			out = append(out, Entry{Path: strings.TrimSuffix(p, "/"), IsDir: true})
			continue
		}
		out = append(out, Entry{Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// tree expands file paths into the listing walk would produce for them:
// every file plus every ancestor directory.
func tree(files ...string) []Entry {
	seen := map[string]bool{}
	var paths []string
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			paths = append(paths, f)
		}
		for i := strings.LastIndexByte(f, '/'); i > 0; i = strings.LastIndexByte(f, '/') {
			f = f[:i]
			if seen[f] {
				break
			}
			seen[f] = true
			paths = append(paths, f+"/")
		}
	}
	return listing(paths...)
}

func diagram(t *testing.T, src string) Diagram {
	t.Helper()
	d, err := nodes.Parse("system.d2", []byte(src))
	if err != nil {
		t.Fatalf("parse diagram: %v", err)
	}
	return Diagram{Nodes: d, Directives: directive.Parse("system.d2", []byte(src))}
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

// summary renders violations as "CODE target" lines — the same (code, target)
// pair the fixture EXPECTED files contract on.
func summary(vs []Violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, string(v.Code)+" "+v.Target())
	}
	return out
}

func assertViolations(t *testing.T, got []Violation, want ...string) {
	t.Helper()
	g := summary(got)
	sorted := append([]string(nil), g...)
	sort.Strings(sorted)
	w := append([]string(nil), want...)
	sort.Strings(w)
	if strings.Join(sorted, "\n") != strings.Join(w, "\n") {
		t.Errorf("violations mismatch\n got: %v\nwant: %v", g, want)
	}
	for _, v := range got {
		if v.Hint == "" {
			t.Errorf("%s %s: empty hint; every violation carries a runnable next step", v.Code, v.Target())
		}
		if v.Detail == "" {
			t.Errorf("%s %s: empty detail", v.Code, v.Target())
		}
		if v.Severity == "" {
			t.Errorf("%s %s: empty severity", v.Code, v.Target())
		}
	}
}

// --- the five codes, positive and negative ------------------------------

func TestCodesPositiveAndNegative(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		files []Entry
		conf  *config.Config
		want  []string
	}{
		{
			name: "ORPHAN: a bind glob matching nothing",
			src: `# @bind svc_billing app/services/billing/**
# @bind svc_reporting app/services/reporting/**
svc_billing: Billing
svc_reporting: Reporting`,
			files: tree("app/services/billing/billing.rb"),
			want:  []string{"ORPHAN svc_reporting"},
		},
		{
			name: "ORPHAN negative: every bind matches at least one file",
			src: `# @bind svc_billing app/services/billing/**
svc_billing: Billing`,
			files: tree("app/services/billing/billing.rb"),
			want:  nil,
		},
		{
			name: "ORPHAN: a shared entry matching nothing (L11)",
			src: `# @bind svc_billing app/services/billing/**
svc_billing: Billing`,
			files: tree("app/services/billing/billing.rb", "lib/http_client/client.rb"),
			conf:  cfg(func(c *config.Config) { c.Shared = []string{"lib/http_client/**", "lib/legacy_pdf/**"} }),
			want:  []string{"ORPHAN lib/legacy_pdf/**"},
		},
		{
			name: "UNMAPPED: a discovered unit no binding covers",
			src: `# @bind svc_billing app/services/billing/**
svc_billing: Billing`,
			files: tree("app/services/billing/billing.rb", "app/services/notifications/notifier.rb"),
			conf:  cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
			want:  []string{"UNMAPPED app/services/notifications/"},
		},
		{
			name: "UNMAPPED negative: every unit covered",
			src: `# @bind svc_billing app/services/billing/**
# @bind svc_ledger app/services/ledger/**
svc_billing: Billing
svc_ledger: Ledger`,
			files: tree("app/services/billing/billing.rb", "app/services/ledger/ledger.rb"),
			conf:  cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
			want:  nil,
		},
		{
			name: "DANGLING: directive names a node that is not there",
			src: `# @bind svc_billing app/services/billing/**
# @bind svc_invoicing app/legacy/invoicing/**
svc_billing: Billing`,
			files: tree("app/services/billing/billing.rb", "app/legacy/invoicing/invoice.rb"),
			want:  []string{"DANGLING svc_invoicing"},
		},
		{
			name: "DANGLING negative: qualified and unqualified both resolve",
			src: `# @bind platform.svc_work_orders app/services/work_orders/**
# @bind svc_dispatch app/services/dispatch/**
platform: Platform {
  svc_work_orders: Work Orders
  svc_dispatch: Dispatch
}`,
			files: tree("app/services/work_orders/wo.rb", "app/services/dispatch/d.rb"),
			want:  nil,
		},
		{
			name: "UNBOUND: a leaf node with no directive",
			src: `# @bind svc_billing app/services/billing/**
svc_billing: Billing
queue_dispatch: Dispatch Queue`,
			files: tree("app/services/billing/billing.rb"),
			want:  []string{"UNBOUND queue_dispatch"},
		},
		{
			name: "UNBOUND negative: all four directives account for a node",
			src: `# @bind svc_billing app/services/billing/**
# @external ext_stripe
# @infra db_primary
# @ignore old_thing "kept for the migration narrative until Q4"
svc_billing: Billing
ext_stripe: Stripe
db_primary: Postgres
old_thing: Old`,
			files: tree("app/services/billing/billing.rb"),
			want:  nil,
		},
		{
			name: "SYNTAX: a malformed directive",
			src: `# @bind svc_billing app/services/billing/**
# @bind svc_search
svc_billing: Billing
svc_search: Search`,
			files: tree("app/services/billing/billing.rb"),
			// The node is UNBOUND too: O11 discards the malformed line, so it
			// cannot account for svc_search.
			want: []string{"SYNTAX svc_search", "UNBOUND svc_search"},
		},
		{
			name: "SYNTAX negative: every directive well formed",
			src: `# @bind svc_billing app/services/billing/**
# @ignore old_thing "deleted Q3, kept for the migration narrative"
svc_billing: Billing
old_thing: Old`,
			files: tree("app/services/billing/billing.rb"),
			want:  nil,
		},
		{
			name: "overlapping bindings are legal and get no code (L12)",
			src: `# @bind svc_invoicing app/services/billing/**
# @bind svc_payments app/services/billing/payment_processor.rb
svc_invoicing: Invoicing
svc_payments: Payments`,
			files: tree("app/services/billing/billing.rb", "app/services/billing/payment_processor.rb"),
			conf:  cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.conf
			if c == nil {
				c = cfg(nil)
			}
			got := Check(Input{Files: tc.files, Diagrams: []Diagram{diagram(t, tc.src)}, Config: c})
			assertViolations(t, got, tc.want...)
		})
	}
}

// --- O8: node ID resolution ---------------------------------------------

func TestO8Resolution(t *testing.T) {
	const src = `# @bind platform.svc_search app/a/**
# @bind support.svc_search app/b/**
# @bind svc_search app/c/**
# @bind svc_missing app/d/**
platform: Platform {
  svc_search: Invoice Search
}
support: Support {
  svc_search: Ticket Search
}`
	files := tree("app/a/a.rb", "app/b/b.rb", "app/c/c.rb", "app/d/d.rb")
	got := Check(Input{Files: files, Diagrams: []Diagram{diagram(t, src)}, Config: cfg(nil)})

	// Exact match resolves; unique suffix would resolve; ambiguous suffix is
	// SYNTAX and is never silently picked; zero candidates is DANGLING.
	assertViolations(t, got, "SYNTAX svc_search", "DANGLING svc_missing")

	for _, v := range got {
		if v.Code != CodeSyntax {
			continue
		}
		for _, want := range []string{"platform.svc_search", "support.svc_search"} {
			if !strings.Contains(v.Hint, want) {
				t.Errorf("ambiguous hint must name every candidate; %q missing from %q", want, v.Hint)
			}
		}
	}
}

func TestO8SuffixMustLandOnSegmentBoundary(t *testing.T) {
	const src = `# @bind orders app/x/**
platform: Platform {
  svc_work_orders: Work Orders
}`
	got := Check(Input{
		Files:    tree("app/x/x.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	// "orders" is a substring of svc_work_orders, not a dot-segment suffix.
	assertViolations(t, got, "DANGLING orders", "UNBOUND platform.svc_work_orders")
}

func TestDanglingHintNamesTheNearestNode(t *testing.T) {
	const src = `# @bind svc_bilingg app/services/billing/**
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/b.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	if len(got) == 0 {
		t.Fatal("expected a DANGLING violation")
	}
	if !strings.Contains(got[0].Hint, "svc_billing") {
		t.Errorf("DANGLING hint should name the nearest node by edit distance, got %q", got[0].Hint)
	}
}

// --- O9: containers -----------------------------------------------------

func TestO9ContainerWithAllDescendantsAccountedIsSilent(t *testing.T) {
	const src = `# @bind platform.svc_work_orders app/services/work_orders/**
# @bind svc_dispatch app/services/dispatch/**
# @infra platform.db_primary
platform: Repairs Platform {
  svc_work_orders: Work Orders
  svc_dispatch: Dispatch
  db_primary: Postgres
}`
	got := Check(Input{
		Files:    tree("app/services/work_orders/wo.rb", "app/services/dispatch/d.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got)
}

func TestO9ContainerWarnsOnTheDescendantNotTheContainer(t *testing.T) {
	const src = `# @bind platform.svc_work_orders app/services/work_orders/**
platform: Repairs Platform {
  svc_work_orders: Work Orders
  svc_dispatch: Dispatch
}`
	got := Check(Input{
		Files:    tree("app/services/work_orders/wo.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	// Exactly one warning, and it is on the descendant. One modeling gap
	// produces one warning; warning on both would train a suppression reflex.
	assertViolations(t, got, "UNBOUND platform.svc_dispatch")
}

func TestO9NestedContainersAreGroupingDevices(t *testing.T) {
	const src = `# @bind a.b.svc_one app/one/**
# @bind a.b.svc_two app/two/**
a: Outer {
  b: Inner {
    svc_one: One
    svc_two: Two
  }
}`
	got := Check(Input{
		Files:    tree("app/one/o.rb", "app/two/t.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got)
}

func TestO9ContainerMayCarryItsOwnBindAndIsOrphanCheckedNormally(t *testing.T) {
	const src = `# @bind platform app/platform/**
# @bind platform.svc_one app/one/**
platform: Platform {
  svc_one: One
}`
	got := Check(Input{
		Files:    tree("app/one/o.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	// The container's own binding matches nothing, so it is ORPHAN — the O9
	// suppression rule is about UNBOUND and does not touch it.
	assertViolations(t, got, "ORPHAN platform")
}

// --- O10: what "covered" means ------------------------------------------

func TestO10CoverageIsFileLevelNotPathLevel(t *testing.T) {
	// The whole point of O10: this glob matches every file in the directory
	// and never the directory path itself. A path test would call a correctly
	// bound service UNMAPPED.
	const src = `# @bind svc_billing app/services/billing/*.rb
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/billing.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	assertViolations(t, got)
}

func TestO10EmptyUnitAlwaysFiresWithADistinguishingHint(t *testing.T) {
	const src = `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	files := append(tree("app/services/billing/billing.rb"), Entry{Path: "app/services/empty", IsDir: true})
	got := Check(Input{
		Files:    listing(pathsOf(files)...),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	assertViolations(t, got, "UNMAPPED app/services/empty/")
	if strings.Contains(got[0].Hint, "@bind") {
		t.Errorf("an empty unit must not get the generic add-a-@bind hint, got %q", got[0].Hint)
	}
	if !strings.Contains(got[0].Hint, "no files") {
		t.Errorf("empty-unit hint should say the directory is empty, got %q", got[0].Hint)
	}
}

func pathsOf(es []Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		if e.IsDir {
			out = append(out, e.Path+"/")
			continue
		}
		out = append(out, e.Path)
	}
	return out
}

func TestO10SharedSuppressesUnmapped(t *testing.T) {
	// DESIGN §4: `shared` suppresses UNMAPPED and is ORPHAN-checked; `exclude`
	// does neither because the walk never showed it to us.
	const src = `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/b.rb", "app/services/middleware/request_id.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config: cfg(func(c *config.Config) {
			c.Discover = []string{"app/services/*/"}
			c.Shared = []string{"app/services/middleware/**"}
		}),
	})
	assertViolations(t, got)
}

// --- O11: invalid directives participate in nothing else ----------------

func TestO11DanglingDirectiveIsInertEverywhereElse(t *testing.T) {
	const src = `# @bind svc_gone app/services/notifications/**
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/notifications/n.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	// The stale directive is reported once and otherwise discarded: it confers
	// no coverage, so the code is also reported as unowned. Both statements
	// are true, and the second is the expensive one to miss.
	assertViolations(t, got,
		"DANGLING svc_gone",
		"UNMAPPED app/services/notifications/",
		"UNBOUND svc_billing",
	)
}

func TestO11SyntaxDirectiveDoesNotAccountForItsNode(t *testing.T) {
	const src = `# @ignore svc_legacy
svc_legacy: Legacy`
	got := Check(Input{
		Files:    tree("app/x/x.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got, "SYNTAX svc_legacy", "UNBOUND svc_legacy")
}

func TestO11AmbiguousDirectiveConfersNoCoverageAndIsNotOrphanChecked(t *testing.T) {
	const src = `# @bind a.svc app/services/shared_thing/**
# @bind b.svc app/services/other/**
# @bind svc app/services/nothing_here/**
a: A { svc: One }
b: B { svc: Two }`
	got := Check(Input{
		Files:    tree("app/services/shared_thing/s.rb", "app/services/other/o.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	// The ambiguous line's glob matches zero files, but it is NOT ORPHAN-checked
	// — it was discarded. Exactly one violation, the SYNTAX.
	assertViolations(t, got, "SYNTAX svc")
}

// --- discover: the trailing-slash trap ----------------------------------

func TestDiscoverToleratesBothAuthoringForms(t *testing.T) {
	const src = `svc_billing: Billing`
	files := tree("app/services/billing/billing.rb")
	for _, rule := range []string{"app/services/*/", "app/services/*"} {
		t.Run(rule, func(t *testing.T) {
			got := Check(Input{
				Files:    files,
				Diagrams: []Diagram{diagram(t, src)},
				Config:   cfg(func(c *config.Config) { c.Discover = []string{rule} }),
			})
			assertViolations(t, got, "UNMAPPED app/services/billing/", "UNBOUND svc_billing")
		})
	}
}

// The load-bearing negative: if the engine failed to synthesize the trailing
// slash, discover would match zero units, UNMAPPED would never fire, and the
// check would pass while inspecting no code at all.
func TestDiscoverRuleMatchingNothingIsReportedNotSilent(t *testing.T) {
	const src = `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/billing.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"src/packages/*/"} }),
	})
	assertViolations(t, got, "ORPHAN src/packages/*/")
}

// --- bare directory forms -----------------------------------------------

func TestDirectoryMatchClaimsItsSubtree(t *testing.T) {
	// `shared: lib/pricing_engine` (no glob) is a form config explicitly
	// accepts. It matches only the directory entry, so a files-only reading
	// would fail it as ORPHAN on its first run.
	const src = `# @bind svc_billing app/services/billing
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/nested/deep.rb", "lib/pricing_engine/price.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config: cfg(func(c *config.Config) {
			c.Discover = []string{"app/services/*/"}
			c.Shared = []string{"lib/pricing_engine"}
		}),
	})
	assertViolations(t, got)
}

// A directory's children are a contiguous run of the sorted listing, but not
// the run that immediately follows it: '-' and '.' sort below '/', so
// `billing-old` and `billing.rb` land between `billing` and `billing/…`. Walking
// forward from the directory entry instead of searching for the run makes a
// bound service look unmapped and an unrelated sibling look owned.
func TestSiblingsThatSortBetweenADirectoryAndItsChildren(t *testing.T) {
	const src = `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	got := Check(Input{
		Files: tree(
			"app/services/billing/billing.rb",
			"app/services/billing-old/legacy.rb",
			"app/services/billing.rb",
		),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	assertViolations(t, got, "UNMAPPED app/services/billing-old/")
}

// --- severity -----------------------------------------------------------

func TestSeverityOverrides(t *testing.T) {
	const src = `# @bind svc_reporting app/services/reporting/**
svc_reporting: Reporting
queue_dispatch: Queue`
	files := tree("app/services/billing/b.rb")

	t.Run("defaults", func(t *testing.T) {
		got := Check(Input{Files: files, Diagrams: []Diagram{diagram(t, src)}, Config: cfg(nil)})
		bySev := map[Code]Severity{}
		for _, v := range got {
			bySev[v.Code] = v.Severity
		}
		if bySev[CodeOrphan] != config.SeverityFail {
			t.Errorf("ORPHAN default severity = %q, want fail", bySev[CodeOrphan])
		}
		if bySev[CodeUnbound] != config.SeverityWarn {
			t.Errorf("UNBOUND default severity = %q, want warn (O3)", bySev[CodeUnbound])
		}
	})

	t.Run("promotion", func(t *testing.T) {
		c := cfg(func(c *config.Config) { c.Severity[config.CodeUnbound] = config.SeverityFail })
		got := Check(Input{Files: files, Diagrams: []Diagram{diagram(t, src)}, Config: c})
		for _, v := range got {
			if v.Code == CodeUnbound && v.Severity != config.SeverityFail {
				t.Errorf("UNBOUND severity = %q, want the configured fail", v.Severity)
			}
		}
	})

	t.Run("off suppresses the code entirely", func(t *testing.T) {
		c := cfg(func(c *config.Config) { c.Severity[config.CodeOrphan] = config.SeverityOff })
		got := Check(Input{Files: files, Diagrams: []Diagram{diagram(t, src)}, Config: c})
		assertViolations(t, got, "UNBOUND queue_dispatch")
	})
}

// The taxonomy is duplicated across a package boundary: config restates the
// five codes as strings because it cannot import this package. If the two lists
// drift, `severity: {UNBOUND: warn}` silently stops applying — a config the user
// wrote that quietly does nothing. Pin them together (GAMEPLAN §3).
func TestCodesMatchConfigCodes(t *testing.T) {
	if len(Codes) != len(config.Codes) {
		t.Fatalf("check has %d codes, config has %d; the taxonomy is closed at five", len(Codes), len(config.Codes))
	}
	mine := map[string]bool{}
	for _, c := range Codes {
		mine[string(c)] = true
	}
	for _, c := range config.Codes {
		if !mine[c] {
			t.Errorf("config knows severity key %q but check never emits it; `severity: {%s: ...}` would do nothing", c, c)
		}
	}
	theirs := map[string]bool{}
	for _, c := range config.Codes {
		theirs[c] = true
	}
	for _, c := range Codes {
		if !theirs[string(c)] {
			t.Errorf("check emits %q but config rejects it as a severity key; the code cannot be configured", c)
		}
	}
	// Every code must be reachable through SeverityFor, or `off` and the
	// per-code overrides silently do nothing for it.
	c := cfg(nil)
	for _, code := range Codes {
		if s := c.SeverityFor(string(code)); !s.Valid() {
			t.Errorf("SeverityFor(%q) = %q, not a valid severity", code, s)
		}
	}
}

// --- multiple diagrams --------------------------------------------------

func TestBindingsPoolAcrossDiagramsForCoverage(t *testing.T) {
	// Node IDs are scoped per diagram, but coverage is a fact about the repo:
	// code owned by a node in the second diagram is owned.
	a := diagram(t, `# @bind svc_billing app/services/billing/**
svc_billing: Billing`)
	b := diagram(t, `# @bind svc_ledger app/services/ledger/**
svc_ledger: Ledger`)
	got := Check(Input{
		Files:    tree("app/services/billing/b.rb", "app/services/ledger/l.rb"),
		Diagrams: []Diagram{a, b},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	})
	assertViolations(t, got)
}

func TestSameNodeIDInTwoDiagramsIsNotAmbiguous(t *testing.T) {
	src := `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	got := Check(Input{
		Files:    tree("app/services/billing/b.rb"),
		Diagrams: []Diagram{diagram(t, src), diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got)
}

// --- ignore -------------------------------------------------------------

func TestIgnoreSuppressesEverythingForItsNode(t *testing.T) {
	const src = `# @bind svc_legacy app/services/gone/**
# @ignore svc_legacy "read-only until the Q4 migration lands"
svc_legacy: Legacy`
	got := Check(Input{
		Files:    tree("app/services/billing/b.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got)
}

// @ignore is not partial — but it also cannot reach a line that did not parse.
// The node token on a malformed directive is exactly the thing O11 says not to
// trust, and letting a typo'd @ignore suppress SYNTAX would let one malformed
// line hide the next.
func TestIgnoreDoesNotSuppressSyntax(t *testing.T) {
	const src = `# @ignore svc_legacy "read-only until the Q4 migration lands"
# @bind svc_legacy
svc_legacy: Legacy`
	got := Check(Input{
		Files:    tree("app/x/x.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(nil),
	})
	assertViolations(t, got, "SYNTAX svc_legacy")
}

// --- determinism --------------------------------------------------------

func TestOutputIsDeterministic(t *testing.T) {
	const src = `# @bind svc_a app/gone_a/**
# @bind svc_b app/gone_b/**
# @bind svc_missing app/x/**
svc_a: A
svc_b: B
svc_c: C`
	in := Input{
		Files:    tree("app/x/x.rb", "app/services/orphaned/o.rb"),
		Diagrams: []Diagram{diagram(t, src)},
		Config:   cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	}
	first := fmt.Sprint(summary(Check(in)))
	for i := 0; i < 20; i++ {
		if got := fmt.Sprint(summary(Check(in))); got != first {
			t.Fatalf("run %d differs:\n %s\n %s", i, got, first)
		}
	}
}

func TestUnsortedListingStillProducesTheSameAnswer(t *testing.T) {
	const src = `# @bind svc_billing app/services/billing/**
svc_billing: Billing`
	sorted := tree("app/services/billing/b.rb", "app/services/notifications/n.rb")
	shuffled := make([]Entry, len(sorted))
	for i, e := range sorted {
		shuffled[len(sorted)-1-i] = e
	}
	c := cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} })

	want := summary(Check(Input{Files: sorted, Diagrams: []Diagram{diagram(t, src)}, Config: c}))
	got := summary(Check(Input{Files: shuffled, Diagrams: []Diagram{diagram(t, src)}, Config: c}))
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("unsorted input changed the answer\n got: %v\nwant: %v", got, want)
	}
	if shuffled[0].Path != sorted[len(sorted)-1].Path {
		t.Error("Check mutated the caller's slice")
	}
}

func TestNilConfigUsesDefaults(t *testing.T) {
	const src = `svc_billing: Billing`
	got := Check(Input{Files: tree("app/x/x.rb"), Diagrams: []Diagram{diagram(t, src)}})
	assertViolations(t, got, "UNBOUND svc_billing")
	if got[0].Severity != config.SeverityWarn {
		t.Errorf("severity = %q, want the default warn", got[0].Severity)
	}
}

func TestEmptyInputIsClean(t *testing.T) {
	if got := Check(Input{}); len(got) != 0 {
		t.Errorf("empty input produced %v", summary(got))
	}
}
