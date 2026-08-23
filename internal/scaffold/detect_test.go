package scaffold

import (
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/walk"
)

// listing builds a walk.Listing from file paths, synthesizing the directory
// entries a real walk would have produced. Detect is a pure function of the
// listing, so every case here runs without touching a disk — which is the point
// of the seam: the shapes `init` recognizes are testable against a tree nobody
// had to create.
func listing(files ...string) *walk.Listing {
	dirs := map[string]bool{}
	entries := make([]walk.Entry, 0, len(files)*2)
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

func globs(rules []Rule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Glob)
	}
	return out
}

func TestDetectRecognizesConventionalShapes(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "rails",
			files: []string{"Gemfile", "app/services/billing/billing.rb", "app/jobs/reconciler/job.rb"},
			want:  []string{"app/services/*/", "app/jobs/*/"},
		},
		{
			name:  "js monorepo",
			files: []string{"package.json", "packages/db/index.ts", "apps/web/main.ts"},
			want:  []string{"packages/*/", "apps/*/"},
		},
		{
			name:  "go",
			files: []string{"go.mod", "internal/check/check.go", "cmd/trestle/main.go", "pkg/api/api.go"},
			want:  []string{"internal/*/", "pkg/*/", "cmd/*/"},
		},
		{
			// A Go repo that also holds a `cmd/` directory full of Ruby would
			// still only be offered Go shapes. Marker gating is what stops the
			// tool guessing an ecosystem the repo is not using.
			name:  "markers gate the shapes",
			files: []string{"go.mod", "internal/db/db.go", "app/services/billing/billing.rb"},
			want:  []string{"internal/*/"},
		},
		{
			name:  "src and lib, no marker",
			files: []string{"src/renderer/index.js", "lib/http_client/client.rb"},
			// Nothing identifies the ecosystem, so every shape is tried and the
			// order is lang.All's. Proposing more than necessary is cheap here:
			// a rule is only offered when it matches a directory with files.
			want: []string{"lib/*/", "src/*/"},
		},
		{
			// Everything at the root, one level deep. Proposing `*/` here would
			// be inventing a convention rather than recognizing one, so nothing
			// is proposed and the config says so.
			name:  "no recognized shape",
			files: []string{"main.go", "handlers/http.go"},
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := globs(Detect(listing(tc.files...)))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("Detect = %v, want %v", got, tc.want)
			}
		})
	}
}

// A rule that matches only empty directories would be an ORPHAN the moment it
// was written — a `discover:` rule matching nothing fails by design — so `init`
// would be scaffolding a config that fails its own check.
func TestDetectSkipsShapesWithNoFiles(t *testing.T) {
	l := listing("app/services/billing/billing.rb")
	l.Entries = append(l.Entries, walk.Entry{Path: "packages", IsDir: true}, walk.Entry{Path: "packages/db", IsDir: true})
	sortEntries(l)

	if got := globs(Detect(l)); len(got) != 1 || got[0] != "app/services/*/" {
		t.Errorf("Detect = %v, want only app/services/*/", got)
	}
}

func TestDetectCountsFilesBeneathEachUnit(t *testing.T) {
	rules := Detect(listing(
		"packages/db/index.ts",
		"packages/db/migrations/001.sql",
		"packages/db/migrations/002.sql",
		"packages/adapters/stripe.ts",
	))
	if len(rules) != 1 {
		t.Fatalf("rules = %v, want one", globs(rules))
	}

	want := map[string]int{"packages/db": 3, "packages/adapters": 1}
	for _, u := range rules[0].Units {
		if want[u.Path] != u.Files {
			t.Errorf("%s: %d files, want %d", u.Path, u.Files, want[u.Path])
		}
	}
}

// `*` matches a leading dot, so a virtualenv or a build cache under a matched
// root becomes something the repo has to account for. The proposal has to say
// so at the prompt; discovering it as an UNMAPPED afterwards is how a user
// concludes the rule was a bad idea.
func TestDetectFlagsDotDirectories(t *testing.T) {
	rules := Detect(listing("src/app/main.ts", "src/.cache/blob"))
	if len(rules) != 1 {
		t.Fatalf("rules = %v, want one", globs(rules))
	}
	if got := rules[0].Hidden(); got != 1 {
		t.Errorf("Hidden() = %d, want 1", got)
	}
}

// No two shapes in the current list can match the same directory, so this holds
// by construction — which is exactly why it is worth pinning. Adding a broader
// shape later (`app/*/`, say) would silently make it false: the directory would
// be counted twice in the predicted UNMAPPED total, and the user would be asked
// to approve the same rule under two names.
func TestDetectClaimsEachDirectoryOnce(t *testing.T) {
	rules := Detect(listing("app/services/billing/billing.rb", "services/legacy/old.rb"))
	seen := map[string]bool{}
	for _, r := range rules {
		for _, u := range r.Units {
			if seen[u.Path] {
				t.Errorf("%s proposed twice", u.Path)
			}
			seen[u.Path] = true
		}
	}
}

func TestDetectHandlesNilListing(t *testing.T) {
	if got := Detect(nil); got != nil {
		t.Errorf("Detect(nil) = %v, want nil", got)
	}
}

func sortEntries(l *walk.Listing) {
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].Path < l.Entries[j].Path })
}

// A source tree that does not sit at the repo root is ordinary, and anchoring
// only there is how a real Go repo got `discover: []` — go.mod at the top,
// every package under api/, so `internal/*/` and `cmd/*/` matched nothing and
// `init` proposed no rules at all. The shapes were right; they were one
// directory too high.
func TestDetectFindsShapesBelowTheRepoRoot(t *testing.T) {
	l := listing(
		"go.mod",
		"api/internal/db/db.go",
		"api/internal/auth/auth.go",
		"api/cmd/server/main.go",
		"README.md",
	)

	rules := Detect(l)

	got := map[string]int{}
	for _, r := range rules {
		got[r.Glob] = len(r.Units)
	}
	if got["api/internal/*/"] != 2 {
		t.Errorf("api/internal/*/ matched %d units, want 2 — rules: %v", got["api/internal/*/"], got)
	}
	if got["api/cmd/*/"] != 1 {
		t.Errorf("api/cmd/*/ matched %d units, want 1 — rules: %v", got["api/cmd/*/"], got)
	}
}

// Canonical Rails is one directory per layer under app/ with flat files inside.
// `app/services/` is a community pattern `rails new` never creates, so a repo
// following the framework matched nothing until `app/*/` existed — a real
// 600-file app produced one rule covering 27 files and a green check over 4%
// of itself.
func TestDetectRecognizesCanonicalRails(t *testing.T) {
	l := listing(
		"Gemfile",
		"app/models/work_order.rb",
		"app/controllers/orders_controller.rb",
		"app/agents/planner.rb",
		"config/routes.rb",
	)

	var units int
	for _, r := range Detect(l) {
		if r.Glob == "app/*/" {
			units = len(r.Units)
		}
	}
	if units != 3 {
		t.Errorf("app/*/ matched %d units, want 3 (models, controllers, agents)", units)
	}
}

// `discover:` units must not nest. With both the specific and the general Rails
// shape available, a repo holding app/services/billing/ would otherwise be
// offered `app/services` *and* `app/services/billing` — and the outer one can
// never be satisfied without claiming the inner one, so it reports UNMAPPED
// forever however the diagram is written.
func TestDetectDropsUnitsThatContainOtherUnits(t *testing.T) {
	l := listing(
		"Gemfile",
		"app/services/billing/billing.rb",
		"app/models/order.rb",
	)

	var all []string
	for _, r := range Detect(l) {
		for _, u := range r.Units {
			all = append(all, u.Path)
		}
	}

	for _, u := range all {
		for _, other := range all {
			if u != other && strings.HasPrefix(other, u+"/") {
				t.Errorf("%q contains %q; discover units must not nest (all: %v)", u, other, all)
			}
		}
	}
	// The deeper unit is the one Spike 01 measured as a box somebody would draw.
	var hasBilling bool
	for _, u := range all {
		if u == "app/services/billing" {
			hasBilling = true
		}
	}
	if !hasBilling {
		t.Errorf("app/services/billing was dropped in favour of its parent: %v", all)
	}
}

// Anchoring at every top-level directory must not start proposing rules for
// directories that merely exist. A shape is still only offered when it matches
// a directory with files beneath it.
func TestDetectDoesNotProposeShapesThatAreNotThere(t *testing.T) {
	l := listing("docs/notes.md", "scripts/deploy.sh", "README.md")

	if rules := Detect(l); len(rules) != 0 {
		t.Errorf("proposed %d rules for a repo with no recognized layout: %v", len(rules), rules)
	}
}

// A pnpm/npm workspace nests, and the container is not a unit.
//
// astro's `packages/integrations/` holds seventeen published packages and is
// not one itself, so `packages/*/` matched the container — and a single binding
// on `packages/integrations/**` would own every adapter, letting a new one land
// with nothing firing. That is the blindspot UNMAPPED exists to close, seeded
// by default, on the shape npm repos actually use: `pnpm-workspace.yaml` says
// `packages/**/*`, not `packages/*`.
func TestDetectReachesInsideWorkspaceContainers(t *testing.T) {
	l := listing(
		"package.json",
		"packages/astro/package.json", "packages/astro/src/index.ts",
		"packages/integrations/mdx/package.json", "packages/integrations/mdx/index.ts",
		"packages/integrations/sitemap/package.json", "packages/integrations/sitemap/index.ts",
	)

	got := globs(Detect(l))
	var hasInner, hasContainer bool
	for _, g := range got {
		switch g {
		case "packages/integrations/*/":
			hasInner = true
		}
	}
	for _, r := range Detect(l) {
		for _, u := range r.Units {
			if u.Path == "packages/integrations" {
				hasContainer = true
			}
		}
	}

	if !hasInner {
		t.Errorf("packages/integrations/*/ was not proposed: %v", got)
	}
	if hasContainer {
		t.Error("the container is still a unit; one binding on it would own every package inside")
	}
}

// The container rule is restricted to directories a base shape already matched.
// Run it over the whole listing and `examples/` qualifies — every example is a
// project with its own package.json, and none of them are architecture. astro
// has 25 of them.
func TestDetectDoesNotReachIntoExamples(t *testing.T) {
	l := listing(
		"package.json",
		"packages/astro/package.json", "packages/astro/src/index.ts",
		"examples/blog/package.json", "examples/blog/src/page.astro",
		"examples/portfolio/package.json", "examples/portfolio/src/page.astro",
	)

	for _, g := range globs(Detect(l)) {
		if strings.HasPrefix(g, "examples/") {
			t.Errorf("proposed %q; examples are not architecture", g)
		}
	}
}

// One child that happens to hold a package is a directory, not a container of
// packages. Two is a shape.
func TestDetectDoesNotExpandASingleChild(t *testing.T) {
	l := listing(
		"package.json",
		"packages/astro/package.json", "packages/astro/src/index.ts",
		"packages/tools/only/package.json", "packages/tools/only/index.ts",
	)

	for _, g := range globs(Detect(l)) {
		if g == "packages/tools/*/" {
			t.Error("expanded a container with a single child")
		}
	}
}
