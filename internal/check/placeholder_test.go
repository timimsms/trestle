package check

import (
	"testing"

	"github.com/timimsms/trestle/internal/config"
)

func TestIsPlaceholder(t *testing.T) {
	for _, p := range []string{
		"app/services/planned/.keep",
		"api/internal/rig/.gitkeep",
		"pkg/x/.placeholder",
		".keep",
	} {
		if !isPlaceholder(p) {
			t.Errorf("isPlaceholder(%q) = false", p)
		}
	}
	for _, p := range []string{
		"app/services/billing/billing.rb",
		"api/internal/db/db.go",
		"docs/keep.md",
		"src/keeper.ts",
		".keepalive",
	} {
		if isPlaceholder(p) {
			t.Errorf("isPlaceholder(%q) = true", p)
		}
	}
}

// The silent green this rule exists to close: a node bound to a directory
// holding nothing but `.keep` reported `matches 1 file` and passed, so a box
// claiming a service that does not exist looked identical to one backed by
// real code.
func TestBindingThatMatchesOnlyPlaceholdersIsAnOrphan(t *testing.T) {
	in := Input{
		Files: tree("app/services/planned/.keep"),
		Diagrams: []Diagram{diagram(t, `# @bind svc_planned app/services/planned/**
svc_planned: Planned`)},
		Config: cfg(nil),
	}

	vs := Check(in)
	if len(vs) == 0 {
		t.Fatal("a box backed only by a placeholder passed the check")
	}
	var orphan *Violation
	for i := range vs {
		if vs[i].Code == CodeOrphan {
			orphan = &vs[i]
		}
	}
	if orphan == nil {
		t.Fatalf("want ORPHAN, got %v", vs)
	}
	if orphan.Hint == "" {
		t.Error("no hint")
	}
	// "renamed?" would send the author looking for something never moved.
	if got := orphan.Detail; got != "@bind app/services/planned/** matches only placeholder files" {
		t.Errorf("detail = %q; it should say the directory is there and empty", got)
	}
}

// A discover unit holding only placeholders is a package declared and not
// written yet. UNMAPPED means code exists that the diagram never learned
// about; here no code exists, so there is nothing to report.
//
// This amends O10's corollary, which said an empty unit always fires. Git
// cannot commit an empty directory, so the placeholder is the only shape that
// case takes in a real repo — and a Go repo with 7 of 15 packages in it had no
// honest resolution available.
func TestDeclaredButUnbuiltUnitIsSilent(t *testing.T) {
	in := Input{
		Files: tree("api/internal/db/db.go", "api/internal/rig/.gitkeep"),
		Diagrams: []Diagram{diagram(t, `# @bind db api/internal/db/**
db: Database`)},
		Config: cfg(func(c *config.Config) { c.Discover = []string{"api/internal/*/"} }),
	}

	for _, v := range Check(in) {
		if v.Code == CodeUnmapped {
			t.Errorf("reported %s on a package with no code in it yet: %s", v.Code, v.Path)
		}
	}
}

// The signal that makes the silence acceptable: the moment real code lands in
// that directory, it is a normal UNMAPPED. Silence now, report later — which is
// what `exclude:` could never give, because it would stay quiet forever.
func TestUnitFiresOnceRealCodeLands(t *testing.T) {
	in := Input{
		Files: tree("api/internal/rig/.gitkeep", "api/internal/rig/rig.go"),
		Diagrams: []Diagram{diagram(t, `# @infra placeholder_free
placeholder_free: Nothing`)},
		Config: cfg(func(c *config.Config) { c.Discover = []string{"api/internal/*/"} }),
	}

	var found bool
	for _, v := range Check(in) {
		if v.Code == CodeUnmapped && v.Path == "api/internal/rig/" {
			found = true
		}
	}
	if !found {
		t.Error("no UNMAPPED after real code landed beside the placeholder")
	}
}

// Placeholders are not code, so they are not in the coverage denominator
// either. Counting them would make a repo of declared-but-empty packages look
// better covered than it is.
func TestCoverageIgnoresPlaceholders(t *testing.T) {
	cov := Measure(
		tree("app/services/billing/billing.rb", "app/services/planned/.keep"),
		cfg(func(c *config.Config) { c.Discover = []string{"app/services/*/"} }),
	)
	if cov.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1 — the .keep is not code", cov.TotalFiles)
	}
	if cov.Files != 1 {
		t.Errorf("Files = %d, want 1", cov.Files)
	}
}
