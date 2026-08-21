package check

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

// The corpus below is deliberately awkward. Prefix narrowing works by byte
// comparison on a sorted slice, so the paths that break it are the ones that
// sort *near* a pattern's literal prefix without being under it — `billing.rb`
// and `billing-old/` sort between `app/services/billing` and
// `app/services/billing/`, and `billingx/` sorts just past the upper bound.
var corpusPaths = []string{
	"app",
	"app/adapters",
	"app/adapters/stripe_gateway",
	"app/adapters/stripe_gateway/client.rb",
	"app/jobs",
	"app/jobs/reconciler",
	"app/jobs/reconciler/reconciler.rb",
	"app/middleware",
	"app/middleware/request_id.rb",
	"app/services",
	"app/services/billing",
	"app/services/billing.rb",
	"app/services/billing-old",
	"app/services/billing-old/legacy.rb",
	"app/services/billing/billing.rb",
	"app/services/billing/invoice_builder.rb",
	"app/services/billing/nested",
	"app/services/billing/nested/deep.rb",
	"app/services/billing_test.rb",
	"app/services/billingx",
	"app/services/billingx/x.rb",
	"app/services/ledger",
	"app/services/ledger/ledger.rb",
	"app/services/notifications",
	"app/services/notifications/notifier.rb",
	"docs",
	"docs/architecture",
	"docs/architecture/system.d2",
	"lib",
	"lib/http_client",
	"lib/http_client/client.rb",
	"lib/logging",
	"lib/logging/logger.rb",
	".trestle.yml",
	"README.md",
	"a",
	"a/b",
	"a/b/c.go",
	"vendor",
	"vendor/gem",
	"vendor/gem/thing.rb",
}

var corpusPatterns = []string{
	// the ordinary shapes
	"app/services/billing/**",
	"app/services/*/",
	"app/services/*",
	"app/services/**",
	"app/services/billing/*.rb",
	"app/services/billing/payment_processor.rb",
	"app/services/billing",
	"lib/http_client/**",
	"lib/**",
	"app/*/middleware/**",
	"app/middleware/**",
	// no literal prefix at all — the full-scan fallback
	"**/*.rb",
	"**",
	"**/*_test.rb",
	"**/billing/**",
	// prefixes that end mid-segment
	"app/serv*/**",
	"app/services/billing*",
	"app/services/billing*/**",
	"app/services/billing?",
	// metacharacter classes
	"app/services/{billing,ledger}/**",
	"app/services/[bl]*/",
	"app/services/[bl]*",
	`a/b/c.go`,
	"docs/architecture/*.d2",
	// degenerate
	"",
	"/",
	"nonexistent/**",
	"app",
	"app/",
	"z*/**",
}

func corpusIndex() *index {
	entries := make([]Entry, 0, len(corpusPaths))
	for _, p := range corpusPaths {
		// Anything that is a prefix of another path, at a slash boundary, is a
		// directory — which is what a real walk would have reported.
		isDir := false
		for _, q := range corpusPaths {
			if strings.HasPrefix(q, p+"/") {
				isDir = true
				break
			}
		}
		entries = append(entries, Entry{Path: p, IsDir: isDir})
	}
	return newIndex(entries)
}

// This is the test that licenses the fast path. Narrowing by literal prefix is
// only worth having if it is an exact rewrite: a matcher that is fast and
// subtly wrong is worse than a slow one, because its mistakes look like drift
// rather than like a bug. Prove the equivalence against doublestar itself over
// the cross product, rather than reasoning about it.
func TestEachMatchesDoublestar(t *testing.T) {
	ix := corpusIndex()

	for _, pat := range corpusPatterns {
		var want []string
		for _, e := range ix.entries {
			if ok, err := doublestar.Match(pat, e.Path); err == nil && ok {
				want = append(want, e.Path)
			}
		}
		var got []string
		ix.each(pat, func(_ int, e Entry) { got = append(got, e.Path) })

		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("pattern %q: prefix narrowing changed the match set\n got: %v\nwant: %v", pat, got, want)
		}
	}
}

// bounds is only allowed to be a superset filter. State that separately from
// the equivalence test so a failure says which of the two invariants broke.
func TestBoundsNeverExcludeAMatch(t *testing.T) {
	ix := corpusIndex()
	for _, pat := range corpusPatterns {
		lo, hi := ix.bounds(pat)
		for i, e := range ix.entries {
			ok, err := doublestar.Match(pat, e.Path)
			if err != nil || !ok {
				continue
			}
			if i < lo || i >= hi {
				t.Errorf("pattern %q matches %q at index %d, outside the narrowed range [%d,%d)",
					pat, e.Path, i, lo, hi)
			}
		}
	}
}

// eachFile is deliberately NOT plain doublestar: a pattern that matches a
// directory claims the files beneath it, the same way walk's `exclude:` prunes
// a subtree rather than one entry. Pin that difference explicitly so it stays a
// decision rather than becoming an accident.
func TestEachFileClaimsSubtreesOfMatchedDirectories(t *testing.T) {
	ix := corpusIndex()
	tests := []struct {
		pattern string
		want    []string
	}{
		{"app/services/billing", []string{
			"app/services/billing/billing.rb",
			"app/services/billing/invoice_builder.rb",
			"app/services/billing/nested/deep.rb",
		}},
		{"app/services/billing/**", []string{
			"app/services/billing/billing.rb",
			"app/services/billing/invoice_builder.rb",
			"app/services/billing/nested/deep.rb",
		}},
		{"app/services/billing/*.rb", []string{
			"app/services/billing/billing.rb",
			"app/services/billing/invoice_builder.rb",
		}},
		{"lib/http_client/**", []string{"lib/http_client/client.rb"}},
		{"app/services/nothing/**", nil},
		// Directories are never themselves reported: ORPHAN counts files.
		{"app/services/*/", nil},
	}
	for _, tc := range tests {
		var got []string
		n := ix.eachFile(tc.pattern, func(i int) { got = append(got, ix.entries[i].Path) })
		if n != len(got) {
			t.Errorf("pattern %q: count %d disagrees with %d callbacks", tc.pattern, n, len(got))
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("pattern %q\n got: %v\nwant: %v", tc.pattern, got, tc.want)
		}
	}
}

// A directory match must claim the subtree exactly once, and only files.
func TestEachFileDoesNotDoubleCount(t *testing.T) {
	ix := corpusIndex()
	seen := map[int]int{}
	ix.eachFile("app/**", func(i int) { seen[i]++ })
	for i, n := range seen {
		if n != 1 {
			t.Errorf("%s reported %d times", ix.entries[i].Path, n)
		}
		if ix.entries[i].IsDir {
			t.Errorf("%s is a directory and must not be counted as a file", ix.entries[i].Path)
		}
	}
}

// The trailing-slash trap, at the level it actually bites. walk emits bare
// directory paths; the shipped example config writes discover rules with a
// trailing slash. Both authoring forms must name the same units.
func TestEachUnitSynthesizesTheTrailingSlash(t *testing.T) {
	ix := corpusIndex()
	want := []string{
		"app/services/billing",
		"app/services/billing-old",
		"app/services/billingx",
		"app/services/ledger",
		"app/services/notifications",
	}
	for _, rule := range []string{"app/services/*/", "app/services/*"} {
		var got []string
		ix.eachUnit(rule, func(_ int, e Entry) { got = append(got, e.Path) })
		sort.Strings(got)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("rule %q\n got: %v\nwant: %v", rule, got, want)
		}
	}
}

func TestEachUnitReportsOnlyDirectories(t *testing.T) {
	ix := corpusIndex()
	ix.eachUnit("app/services/*/", func(_ int, e Entry) {
		if !e.IsDir {
			t.Errorf("%s is a file and cannot be a discover unit", e.Path)
		}
	})
}

func TestSubtreeIsTheContiguousRunBeneathAnEntry(t *testing.T) {
	ix := corpusIndex()
	for i, e := range ix.entries {
		lo, hi := ix.subtree(i)
		want := map[string]bool{}
		for _, p := range corpusPaths {
			if strings.HasPrefix(p, e.Path+"/") {
				want[p] = true
			}
		}
		got := map[string]bool{}
		for j := lo; j < hi; j++ {
			got[ix.entries[j].Path] = true
		}
		if fmt.Sprint(sortedKeys(got)) != fmt.Sprint(sortedKeys(want)) {
			t.Errorf("subtree(%q)\n got: %v\nwant: %v", e.Path, sortedKeys(got), sortedKeys(want))
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestLiteralPrefix(t *testing.T) {
	tests := map[string]string{
		"app/services/billing/**": "app/services/billing/",
		"app/services/*/":         "app/services/",
		"**/*.rb":                 "",
		"a/b/c.go":                "a/b/c.go",
		"app/serv*/**":            "app/serv",
		"app/{a,b}/**":            "app/",
		"":                        "",
	}
	for in, want := range tests {
		if got := literalPrefix(in); got != want {
			t.Errorf("literalPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlobAnchor(t *testing.T) {
	tests := map[string]string{
		"app/services/billing/**":                   "app/services/billing",
		"app/services/billing/payment_processor.rb": "app/services/billing/payment_processor.rb",
		"lib/legacy_pdf/**":                         "lib/legacy_pdf",
		"**/*.rb":                                   ".",
		"app/serv*/**":                              "app",
		"app/services/billing/":                     "app/services/billing",
	}
	for in, want := range tests {
		if got := globAnchor(in); got != want {
			t.Errorf("globAnchor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggestNodeID(t *testing.T) {
	tests := map[string]string{
		"app/services/notifications": "svc_notifications",
		"app/adapters/stripe":        "adp_stripe",
		"app/jobs/reconciler":        "job_reconciler",
		"packages/db":                "db",
		"app/services/Work Orders":   "svc_work_orders",
	}
	for in, want := range tests {
		if got := suggestNodeID(in); got != want {
			t.Errorf("suggestNodeID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSyntaxTargetIsReportingOnly(t *testing.T) {
	tests := map[string]string{
		"# @bind   svc_search":          "svc_search",
		"# @ignore svc_legacy_tickets":  "svc_legacy_tickets",
		"# @bind":                       "",
		"# @bindd svc_x app/**":         "",
		`# @ignore "no node here"`:      "",
		"## @external ext_stripe extra": "ext_stripe",
	}
	for raw, want := range tests {
		if got := syntaxTarget(raw); got != want {
			t.Errorf("syntaxTarget(%q) = %q, want %q", raw, got, want)
		}
	}
}
