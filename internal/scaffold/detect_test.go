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
			files: []string{"app/services/billing/billing.rb", "app/jobs/reconciler/job.rb"},
			want:  []string{"app/services/*/", "app/jobs/*/"},
		},
		{
			name:  "js monorepo",
			files: []string{"packages/db/index.ts", "apps/web/main.ts"},
			want:  []string{"packages/*/", "apps/*/"},
		},
		{
			name:  "go",
			files: []string{"internal/check/check.go", "cmd/trestle/main.go", "pkg/api/api.go"},
			want:  []string{"internal/*/", "pkg/*/", "cmd/*/"},
		},
		{
			name:  "src and lib",
			files: []string{"src/renderer/index.js", "lib/http_client/client.rb"},
			want:  []string{"src/*/", "lib/*/"},
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
