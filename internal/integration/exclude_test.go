// Package integration holds tests that span package seams. Nothing here tests
// a single package's logic — it tests that two packages written against the
// same spec actually agree, which is the class of bug that unit tests in each
// package will happily both pass while the product is broken.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/walk"
)

func tree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(l *walk.Listing) map[string]bool {
	m := make(map[string]bool, len(l.Entries))
	for _, e := range l.Entries {
		m[e.Path] = true
	}
	return m
}

// config.DefaultExclude() and walk's pruning were written by different authors
// against the same prose. They must agree, or the walk descends into
// node_modules and the 200ms budget is gone.
func TestDefaultExcludePrunes(t *testing.T) {
	root := tree(t,
		"app/services/billing/billing.rb",
		"node_modules/pkg/index.js",
		"app/vendor/gem/thing.rb",
		"vendor/gem/other.rb",
		".git/objects/aa/bb",
		"lib/http_client/client.rb",
	)

	l, err := walk.Walk(walk.Options{Root: root, Exclude: config.DefaultExclude()})
	if err != nil {
		t.Fatal(err)
	}
	got := paths(l)

	for _, want := range []string{"app/services/billing/billing.rb", "lib/http_client/client.rb"} {
		if !got[want] {
			t.Errorf("real code was pruned: %s missing from listing", want)
		}
	}
	for _, unwanted := range []string{
		"node_modules", "node_modules/pkg/index.js",
		"vendor", "vendor/gem/other.rb",
		"app/vendor", "app/vendor/gem/thing.rb",
		".git", ".git/objects/aa/bb",
	} {
		if got[unwanted] {
			t.Errorf("exclude did not prune %q; walk descended into an excluded tree", unwanted)
		}
	}
}

// DESIGN §4 ships "**/vendor/**" as an example. Under plain per-path glob
// semantics that pattern does not match the *directory* app/vendor, only its
// contents — so a prune-on-match walk could descend anyway. Assert the
// documented pattern does what a user reading DESIGN would expect.
func TestDesignExampleExcludePattern(t *testing.T) {
	root := tree(t, "app/vendor/gem/thing.rb", "app/services/billing/b.rb")

	l, err := walk.Walk(walk.Options{Root: root, Exclude: []string{"**/vendor/**"}})
	if err != nil {
		t.Fatal(err)
	}
	got := paths(l)

	if !got["app/services/billing/b.rb"] {
		t.Error("real code was pruned")
	}
	for _, unwanted := range []string{"app/vendor", "app/vendor/gem/thing.rb"} {
		if got[unwanted] {
			t.Errorf("DESIGN's own example pattern failed to prune %q", unwanted)
		}
	}
}

// The silent-failure guard. `discover: app/services/*/` carries a trailing
// slash in the shipped example config, and doublestar does NOT match it
// against a bare directory path. walk emits directories flagged rather than
// slash-suffixed, so the check engine MUST synthesize the trailing slash
// before matching a discover rule. If it does not, every discover rule matches
// nothing, UNMAPPED never fires, and `trestle check` passes while seeing
// nothing — a green check that inspects zero code.
func TestDiscoverGlobNeedsTrailingSlash(t *testing.T) {
	const pattern = "app/services/*/"
	const dir = "app/services/billing"

	bare, err := doublestar.Match(pattern, dir)
	if err != nil {
		t.Fatal(err)
	}
	if bare {
		t.Fatalf("premise changed: %q now matches bare %q. "+
			"Re-check the check engine's discover matching — it synthesizes a "+
			"trailing slash on the assumption this is false.", pattern, dir)
	}

	withSlash, err := doublestar.Match(pattern, dir+"/")
	if err != nil {
		t.Fatal(err)
	}
	if !withSlash {
		t.Fatalf("%q must match %q — discover rules depend on it", pattern, dir+"/")
	}
}

// walk must flag directories, since that flag is the only thing that lets the
// check engine tell a discover unit from a bound file.
func TestWalkFlagsDirectories(t *testing.T) {
	root := tree(t, "app/services/billing/billing.rb")

	l, err := walk.Walk(walk.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, e := range l.Entries {
		seen[e.Path] = e.IsDir
	}
	if isDir, ok := seen["app/services/billing"]; !ok || !isDir {
		t.Errorf("app/services/billing: want flagged as directory, got isDir=%v present=%v", isDir, ok)
	}
	if isDir, ok := seen["app/services/billing/billing.rb"]; !ok || isDir {
		t.Errorf("billing.rb: want flagged as file, got isDir=%v present=%v", isDir, ok)
	}
}
