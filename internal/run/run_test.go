package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/nodes"
	"github.com/timimsms/trestle/internal/run"
	"github.com/timimsms/trestle/internal/walk"
)

const repos = "../../testdata/repos"

// tree writes files into a temp dir and returns the root. Content is keyed by
// repo-relative path.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadResolvesFixture(t *testing.T) {
	ctx, err := run.Load(filepath.Join(repos, "clean"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if filepath.Base(ctx.Config.Root) != "clean" {
		t.Errorf("root = %q, want the clean fixture", ctx.Config.Root)
	}
	if want := []string{"docs/architecture/system.d2"}; len(ctx.Paths) != 1 || ctx.Paths[0] != want[0] {
		t.Errorf("paths = %v, want %v", ctx.Paths, want)
	}
	if len(ctx.Diagrams) != len(ctx.Paths) {
		t.Errorf("%d diagrams for %d paths", len(ctx.Diagrams), len(ctx.Paths))
	}
	if ctx.Listing == nil || ctx.Listing.Len() == 0 {
		t.Error("listing is empty")
	}
	if vs := ctx.Check(); len(vs) != 0 {
		t.Errorf("clean fixture produced %d violations", len(vs))
	}
}

// Discovery starts at the given directory and walks up, so running from a
// subdirectory of a repo checks the whole repo — not the subdirectory.
func TestLoadFindsConfigFromASubdirectory(t *testing.T) {
	ctx, err := run.Load(filepath.Join(repos, "clean", "app", "services", "billing"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if filepath.Base(ctx.Config.Root) != "clean" {
		t.Errorf("root = %q, want the clean fixture root", ctx.Config.Root)
	}
}

// Every path a violation reports has to be repo-relative or output is not
// portable. The diagram is read from an absolute path and parsed under the
// relative one, and both parsers must agree on which they were given —
// otherwise UNBOUND (sourced from the D2 AST) and ORPHAN (sourced from the
// directive scan) group under two different headers for the same file.
func TestDiagramSourcePathsAreRepoRelative(t *testing.T) {
	root := tree(t, map[string]string{
		config.Filename:      "version: 1\ndiagrams: [docs/*.d2]\n",
		"docs/system.d2":     "# @bind svc_a app/a/**\nsvc_a: A\nqueue: Q\n",
		"app/a/thing.go":     "package a\n",
		"docs/other/keep.md": "x\n",
	})
	cfg, err := config.Load(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := run.LoadConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Diagrams[0].Nodes.Path != "docs/system.d2" {
		t.Errorf("nodes parsed under %q, want the repo-relative path", ctx.Diagrams[0].Nodes.Path)
	}
	for _, v := range ctx.Check() {
		if filepath.IsAbs(v.Source.File) && v.Source.File != cfg.Path {
			t.Errorf("%s %s has an absolute source %q", v.Code, v.Target(), v.Source.File)
		}
	}
}

// A check with nothing to check is a broken setup, not a clean repo. This is
// the failure mode that silently reports success if it is not an error.
func TestZeroDiagramsIsAToolError(t *testing.T) {
	root := tree(t, map[string]string{
		config.Filename:  "version: 1\ndiagrams: [docs/architecture/*.d2]\n",
		"app/a/thing.go": "package a\n",
	})
	_, err := run.Load(root)
	if err == nil {
		t.Fatal("zero matched diagrams must be an error")
	}
	var nd *run.NoDiagramsError
	if !errors.As(err, &nd) {
		t.Fatalf("error is %T, want *run.NoDiagramsError: %v", err, err)
	}
	if !strings.Contains(err.Error(), "hint:") {
		t.Errorf("NoDiagramsError carries no hint: %v", err)
	}
}

// `exclude:` prunes the listing before `diagrams:` is resolved against it, so
// an excluded diagram is invisible rather than half-visible. That is a sharp
// edge, and it fails loudly for exactly that reason.
func TestExcludedDiagramIsNotFound(t *testing.T) {
	root := tree(t, map[string]string{
		config.Filename:  "version: 1\ndiagrams: [docs/*.d2]\nexclude: [\"**/docs/**\"]\n",
		"docs/system.d2": "a: A\n",
	})
	if _, err := run.Load(root); err == nil {
		t.Error("an excluded diagram should not be found")
	}
}

// A diagram D2 cannot compile is a tool error: Trestle could not do its job,
// which is a different thing from the repo disagreeing with the diagram.
func TestUnparseableDiagramIsACompileError(t *testing.T) {
	root := tree(t, map[string]string{
		config.Filename:  "version: 1\ndiagrams: [docs/*.d2]\n",
		"docs/system.d2": "a: A {\n",
	})
	_, err := run.Load(root)
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !errors.Is(err, nodes.ErrCompile) {
		t.Errorf("error does not match nodes.ErrCompile: %v", err)
	}
}

// Two patterns matching the same file yield one diagram, not two — otherwise
// every node in it would be parsed twice and every directive counted twice.
func TestMatchDiagramsDeduplicatesAndSorts(t *testing.T) {
	l := &walk.Listing{Entries: []walk.Entry{
		{Path: "docs", IsDir: true},
		{Path: "docs/b.d2"},
		{Path: "docs/a.d2"},
		{Path: "docs/readme.md"},
	}}
	got := run.MatchDiagrams([]string{"docs/*.d2", "**/*.d2", "  "}, l)
	want := []string{"docs/a.d2", "docs/b.d2"}
	if len(got) != len(want) {
		t.Fatalf("MatchDiagrams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MatchDiagrams = %v, want %v", got, want)
		}
	}
}

// Directories never match `diagrams:`. A directory named `architecture.d2`
// would otherwise be handed to the D2 compiler as a file.
func TestMatchDiagramsIgnoresDirectories(t *testing.T) {
	l := &walk.Listing{Entries: []walk.Entry{{Path: "docs/architecture.d2", IsDir: true}}}
	if got := run.MatchDiagrams([]string{"docs/*.d2"}, l); len(got) != 0 {
		t.Errorf("MatchDiagrams matched a directory: %v", got)
	}
}
