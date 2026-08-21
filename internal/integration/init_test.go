package integration

import (
	"os"
	"path"
	"sort"
	"testing"

	"github.com/timimsms/trestle"
	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/scaffold"
	"github.com/timimsms/trestle/internal/walk"
)

// oneFilePerShape holds a file under every layout shape `trestle init` knows
// how to recognize. Adding a shape to internal/scaffold without adding a path
// here makes TestScaffoldedRulesFireUnmapped stop covering it, which is why that
// test asserts the count.
var oneFilePerShape = []string{
	"app/services/billing/billing.rb",
	"app/jobs/reconciler/job.rb",
	"services/legacy/legacy.rb",
	"packages/db/index.ts",
	"apps/web/main.ts",
	"src/renderer/index.js",
	"lib/http_client/client.rb",
	"internal/check/check.go",
	"pkg/api/api.go",
	"cmd/trestle/main.go",
}

// TestScaffoldedRulesFireUnmapped is the guard between the command that writes
// `discover:` rules and the engine that evaluates them.
//
// The trap it exists to close is the trailing-slash one (GAMEPLAN §8):
// `app/services/*` matches nothing, while `app/services/*/` matches the
// directory. A `discover:` rule that matches nothing is silent — UNMAPPED stops
// firing and the check goes green while inspecting nothing — so an `init` that
// seeded the wrong form would hand out a config that quietly does not work. The
// only way to know is to run the rules `init` proposes through the engine that
// consumes them.
func TestScaffoldedRulesFireUnmapped(t *testing.T) {
	listing := syntheticListing(oneFilePerShape...)
	rules := scaffold.Detect(listing)

	if len(rules) != len(oneFilePerShape) {
		got := make([]string, 0, len(rules))
		for _, r := range rules {
			got = append(got, r.Glob)
		}
		t.Fatalf("Detect proposed %d rules for %d shapes (%v); every recognized shape needs a path in oneFilePerShape",
			len(rules), len(oneFilePerShape), got)
	}

	files := make([]check.Entry, len(listing.Entries))
	for i, e := range listing.Entries {
		files[i] = check.Entry{Path: e.Path, IsDir: e.IsDir}
	}

	for _, r := range rules {
		cfg := &config.Config{
			Version:  config.Version,
			Diagrams: []string{"docs/architecture/*.d2"},
			Discover: []string{r.Glob},
			Severity: config.DefaultSeverity(),
		}
		var unmapped, orphan int
		for _, v := range check.Check(check.Input{Files: files, Config: cfg}) {
			switch v.Code {
			case check.CodeUnmapped:
				unmapped++
			case check.CodeOrphan:
				orphan++
			}
		}
		if orphan > 0 {
			t.Errorf("%s: reported ORPHAN — the rule matches no directory at all", r.Glob)
		}
		if unmapped != len(r.Units) {
			t.Errorf("%s: %d UNMAPPED, want %d (one per proposed unit)", r.Glob, unmapped, len(r.Units))
		}
	}
}

// The contract ships in the binary and is written into every repo that runs
// `init`. Two copies of it would be a drift surface, which is the thing this
// tool exists to close, so the embedded copy has to be the repo's own file.
func TestEmbeddedConventionsIsTheRepoCopy(t *testing.T) {
	src, err := os.ReadFile("../../CONVENTIONS.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != trestle.Conventions {
		t.Error("the embedded CONVENTIONS.md is not the copy at the repo root")
	}
}

// syntheticListing builds the listing a walk would have produced for these
// files, directory entries included. IsDir is not a detail here: `discover:`
// matches directories and a listing of files alone cannot exercise it.
func syntheticListing(files ...string) *walk.Listing {
	dirs := map[string]bool{}
	entries := make([]walk.Entry, 0, len(files)*3)
	for _, f := range files {
		entries = append(entries, walk.Entry{Path: f})
		for d := path.Dir(f); d != "." && d != "/"; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	for d := range dirs {
		entries = append(entries, walk.Entry{Path: d, IsDir: true})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return &walk.Listing{Root: ".", Entries: entries}
}
