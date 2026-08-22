package check

import (
	"testing"

	"github.com/timimsms/trestle/internal/config"
)

func entries(paths ...string) []Entry {
	out := make([]Entry, 0, len(paths))
	for _, p := range paths {
		isDir := p[len(p)-1] == '/'
		if isDir {
			p = p[:len(p)-1]
		}
		out = append(out, Entry{Path: p, IsDir: isDir})
	}
	return out
}

var repo = entries(
	"app/", "app/services/",
	"app/services/billing/", "app/services/billing/billing.rb", "app/services/billing/invoice.rb",
	"app/services/dispatch/", "app/services/dispatch/dispatch.rb",
	"lib/", "lib/http_client/", "lib/http_client/client.rb",
	"README.md",
)

// The failure this type exists for: a config that watches almost nothing must
// not be reportable as an unqualified success. Measured against the real shape
// of the first Rails repo Trestle met — one rule matching lib/*/, 27 of 600
// files, permanently green.
func TestMeasureReportsNarrowCoverage(t *testing.T) {
	cov := Measure(repo, &config.Config{Discover: []string{"lib/*/"}})

	if cov.Rules != 1 {
		t.Errorf("Rules = %d, want 1", cov.Rules)
	}
	if cov.Units != 1 {
		t.Errorf("Units = %d, want 1 (lib/http_client)", cov.Units)
	}
	if cov.Files != 1 {
		t.Errorf("Files = %d, want 1", cov.Files)
	}
	if cov.TotalFiles != 5 {
		t.Errorf("TotalFiles = %d, want 5", cov.TotalFiles)
	}
	if !cov.Watched() {
		t.Error("Watched() = false, but a rule matched a directory")
	}
	if got := cov.Percent(); got != 20 {
		t.Errorf("Percent() = %d, want 20", got)
	}
}

// Zero rules is the state `init` writes when its layout detection finds
// nothing, and it is the state that produced a green check over an entire Go
// repo. Watched() must be false so the reporter can say so.
func TestMeasureWithNoRules(t *testing.T) {
	cov := Measure(repo, &config.Config{})

	if cov.Rules != 0 || cov.Units != 0 || cov.Files != 0 {
		t.Errorf("want all zero, got %+v", cov)
	}
	if cov.Watched() {
		t.Error("Watched() = true with no rules; UNMAPPED cannot fire in this state")
	}
	if cov.TotalFiles != 5 {
		t.Errorf("TotalFiles = %d, want 5 — the repo is still there", cov.TotalFiles)
	}
}

// Rules that match nothing are as inert as no rules at all. This is the Go case:
// `internal/*/` is a perfectly good rule against a module rooted one directory
// down, and it watches zero files.
func TestMeasureWithRulesThatMatchNothing(t *testing.T) {
	cov := Measure(repo, &config.Config{Discover: []string{"api/internal/*/", "cmd/*/"}})

	if cov.Rules != 2 {
		t.Errorf("Rules = %d, want 2 — they are configured even though they match nothing", cov.Rules)
	}
	if cov.Units != 0 || cov.Files != 0 {
		t.Errorf("want nothing matched, got %+v", cov)
	}
	if cov.Watched() {
		t.Error("Watched() = true, but no rule matched a directory")
	}
}

// Overlapping rules must not double-count. Coverage above 100% would destroy
// the one number this type exists to make trustworthy.
func TestMeasureDoesNotDoubleCountOverlappingRules(t *testing.T) {
	cov := Measure(repo, &config.Config{Discover: []string{"app/services/*/", "app/*/"}})

	if cov.Files > cov.TotalFiles {
		t.Errorf("Files = %d exceeds TotalFiles = %d", cov.Files, cov.TotalFiles)
	}
	if got := cov.Percent(); got > 100 {
		t.Errorf("Percent() = %d", got)
	}
	// billing (2) + dispatch (1), counted once each.
	if cov.Files != 3 {
		t.Errorf("Files = %d, want 3", cov.Files)
	}
}

func TestMeasureFullCoverage(t *testing.T) {
	cov := Measure(
		entries("app/", "app/billing/", "app/billing/a.rb", "app/dispatch/", "app/dispatch/b.rb"),
		&config.Config{Discover: []string{"app/*/"}},
	)
	if got := cov.Percent(); got != 100 {
		t.Errorf("Percent() = %d, want 100", got)
	}
}

func TestMeasureHandlesNilConfigAndEmptyRepo(t *testing.T) {
	if cov := Measure(repo, nil); cov.Rules != 0 || cov.TotalFiles != 5 {
		t.Errorf("nil config: %+v", cov)
	}
	cov := Measure(nil, &config.Config{Discover: []string{"app/*/"}})
	if got := cov.Percent(); got != 0 {
		t.Errorf("empty repo Percent() = %d, want 0 (no divide by zero)", got)
	}
}

// Measure reads the listing and nothing else. internal/check is I/O-free and
// this file must not be the exception.
func TestMeasureIsPure(t *testing.T) {
	before := Measure(repo, &config.Config{Discover: []string{"app/services/*/"}})
	after := Measure(repo, &config.Config{Discover: []string{"app/services/*/"}})
	if before != after {
		t.Errorf("not deterministic: %+v then %+v", before, after)
	}
}
