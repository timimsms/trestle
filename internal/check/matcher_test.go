package check

import (
	"strings"
	"testing"
)

// Matcher exists so `explain` can report the match set Check is running on. The
// tests below are therefore all differential: they assert that the exported
// answer is the engine's own answer, not that it is some independently
// plausible one. If these ever have to be relaxed, `explain` has started
// describing a check nobody is running.

func TestMatcherAgreesWithCoverage(t *testing.T) {
	files := tree(
		"app/services/billing/billing.rb",
		"app/services/billing/nested/deep.rb",
		"app/services/billing.rb",
		"app/services/billing-old/legacy.rb",
		"app/services/ledger/ledger.rb",
		"lib/http_client/client.rb",
	)
	m := NewMatcher(files)

	for _, pattern := range []string{
		"app/services/billing/**",
		"app/services/billing",
		"app/services/*/",
		"**/*.rb",
		"app/services/nope/**",
		"lib/http_client",
	} {
		t.Run(pattern, func(t *testing.T) {
			// The engine's own path to the same question: mark coverage with
			// eachFile and read back which entries it touched.
			ix := newIndex(files)
			var want []string
			ix.eachFile(pattern, func(i int) { want = append(want, ix.entries[i].Path) })

			got := m.Files(pattern)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("Files(%q) = %v, engine says %v", pattern, got, want)
			}
			if n := m.Count(pattern); n != len(want) {
				t.Errorf("Count(%q) = %d, want %d", pattern, n, len(want))
			}
		})
	}
}

// A binding that matches nothing is the ORPHAN case, and "matches 0 files" is
// the sentence `explain` exists to print. An empty slice must come back as an
// empty slice.
func TestMatcherReportsZeroForAGlobThatClaimsNothing(t *testing.T) {
	m := NewMatcher(tree("app/services/billing/billing.rb"))
	if got := m.Files("app/services/reporting/**"); len(got) != 0 {
		t.Errorf("Files = %v, want none", got)
	}
	if got := m.Count("app/services/reporting/**"); got != 0 {
		t.Errorf("Count = %d, want 0", got)
	}
}

// A glob matching a directory claims every file beneath it, and never the
// directory entry itself: ORPHAN and discover coverage are both defined in
// files (DESIGN §3, O10).
func TestMatcherReturnsFilesNeverDirectories(t *testing.T) {
	m := NewMatcher(tree(
		"app/services/billing/billing.rb",
		"app/services/billing/nested/deep.rb",
	))
	got := m.Files("app/services/billing")
	want := []string{"app/services/billing/billing.rb", "app/services/billing/nested/deep.rb"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Files = %v, want %v", got, want)
	}
}

// A pattern can claim the same file through two routes — `app/**` matches the
// directory and the files under it — and it is one file either way. A count
// that double-reported would make an inventory's totals nonsense.
func TestMatcherReportsEachFileOnce(t *testing.T) {
	m := NewMatcher(tree("app/services/billing/billing.rb", "app/services/ledger/ledger.rb"))
	got := m.Files("app/**")
	if len(got) != 2 {
		t.Errorf("Files = %v, want each file once", got)
	}
}

// The matcher must not perturb the index it shares its machinery with: it uses
// the same scratch space eachFile does, and a leftover mark would silently
// change what a later query answers.
func TestMatcherIsReusable(t *testing.T) {
	m := NewMatcher(tree("app/a/x.rb", "app/b/y.rb"))
	first := m.Files("app/**")
	_ = m.Files("app/a/**")
	second := m.Files("app/**")
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Errorf("repeat query changed: %v then %v", first, second)
	}
}
