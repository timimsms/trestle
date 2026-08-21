package walk

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func mapFS(paths ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, p := range paths {
		m[p] = &fstest.MapFile{Data: []byte("x")}
	}
	return m
}

func TestWalkListsFilesAndDirs(t *testing.T) {
	l, err := Walk(Options{Root: "/repo", FS: mapFS(
		"app/services/billing/charge.rb",
		"app/services/orders/order.rb",
		"README.md",
	)})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	want := []string{
		"README.md",
		"app",
		"app/services",
		"app/services/billing",
		"app/services/billing/charge.rb",
		"app/services/orders",
		"app/services/orders/order.rb",
	}
	if got := l.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths()\n got: %v\nwant: %v", got, want)
	}
	if l.Root != "/repo" {
		t.Errorf("Root = %q, want /repo", l.Root)
	}
	if l.Len() != len(want) {
		t.Errorf("Len = %d, want %d", l.Len(), len(want))
	}

	// The dir/file distinction is load-bearing: `discover: app/services/*/`
	// matches directories, `@bind app/services/billing/**` matches files.
	wantDirs := []string{"app", "app/services", "app/services/billing", "app/services/orders"}
	if got := l.Dirs(); !reflect.DeepEqual(got, wantDirs) {
		t.Errorf("Dirs()\n got: %v\nwant: %v", got, wantDirs)
	}
	wantFiles := []string{"README.md", "app/services/billing/charge.rb", "app/services/orders/order.rb"}
	if got := l.Files(); !reflect.DeepEqual(got, wantFiles) {
		t.Errorf("Files()\n got: %v\nwant: %v", got, wantFiles)
	}
}

// fs.WalkDir emits depth-first, which is not sorted order: "a/b" precedes
// "a.txt" in a walk but follows it in a bytewise sort. The listing must be
// sorted or golden output reorders itself.
func TestListingIsSorted(t *testing.T) {
	l, err := Walk(Options{FS: mapFS("a/b.txt", "a.txt", "a-1.txt", "z/y/x.txt", "b.txt")})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	paths := l.Paths()
	if !slices.IsSorted(paths) {
		t.Errorf("listing is not sorted: %v", paths)
	}
}

func TestRootIsNotAnEntry(t *testing.T) {
	l, err := Walk(Options{FS: mapFS("a.txt")})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, e := range l.Entries {
		if e.Path == "." || e.Path == "" || strings.HasPrefix(e.Path, "./") {
			t.Errorf("bad path in listing: %q", e.Path)
		}
	}
}

func TestGitIsSkippedUnconditionally(t *testing.T) {
	// No exclude patterns at all: .git must still not appear.
	l, err := Walk(Options{FS: mapFS(
		".git/config",
		".git/objects/ab/cdef",
		"vendor/.git/config",
		"sub/.git", // a worktree/submodule pointer file, not a directory
		"src/main.go",
	)})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	for _, p := range l.Paths() {
		if strings.Contains(p, ".git") {
			t.Errorf("%q leaked into the listing", p)
		}
	}
	if !slices.Contains(l.Paths(), "src/main.go") {
		t.Error("src/main.go missing")
	}
}

func TestExclude(t *testing.T) {
	tests := []struct {
		name     string
		exclude  []string
		files    []string
		excluded []string // paths that must NOT appear
		kept     []string // paths that must appear
	}{
		{
			name:     "bare directory name prunes the subtree",
			exclude:  []string{"node_modules"},
			files:    []string{"node_modules/a/b/c.js", "src/app.js"},
			excluded: []string{"node_modules", "node_modules/a", "node_modules/a/b/c.js"},
			kept:     []string{"src", "src/app.js"},
		},
		{
			name:     "doublestar crosses directories",
			exclude:  []string{"**/vendor/**"},
			files:    []string{"vendor/x.go", "a/vendor/y.go", "a/keep.go"},
			excluded: []string{"vendor", "vendor/x.go", "a/vendor", "a/vendor/y.go"},
			kept:     []string{"a", "a/keep.go"},
		},
		{
			name:     "file pattern leaves the directory intact",
			exclude:  []string{"**/*_test.*"},
			files:    []string{"app/foo.rb", "app/foo_test.rb", "app/sub/bar_test.go"},
			excluded: []string{"app/foo_test.rb", "app/sub/bar_test.go"},
			kept:     []string{"app", "app/foo.rb", "app/sub"},
		},
		{
			name:     "trailing slash is tolerated",
			exclude:  []string{"tmp/"},
			files:    []string{"tmp/a.txt", "keep.txt"},
			excluded: []string{"tmp", "tmp/a.txt"},
			kept:     []string{"keep.txt"},
		},
		{
			name:     "anchored pattern does not match a nested directory",
			exclude:  []string{"vendor/**"},
			files:    []string{"vendor/x.go", "a/vendor/y.go"},
			excluded: []string{"vendor", "vendor/x.go"},
			kept:     []string{"a/vendor", "a/vendor/y.go"},
		},
		{
			name:     "multiple patterns are ORed",
			exclude:  []string{"**/*_spec.rb", "**/vendor/**"},
			files:    []string{"a/b_spec.rb", "a/b.rb", "vendor/c.rb"},
			excluded: []string{"a/b_spec.rb", "vendor", "vendor/c.rb"},
			kept:     []string{"a", "a/b.rb"},
		},
		{
			name:     "empty pattern is ignored, not treated as match-all",
			exclude:  []string{""},
			files:    []string{"a.txt"},
			excluded: nil,
			kept:     []string{"a.txt"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, err := Walk(Options{FS: mapFS(tc.files...), Exclude: tc.exclude})
			if err != nil {
				t.Fatalf("Walk: %v", err)
			}
			got := l.Paths()
			for _, p := range tc.excluded {
				if slices.Contains(got, p) {
					t.Errorf("%q should have been excluded; got %v", p, got)
				}
			}
			for _, p := range tc.kept {
				if !slices.Contains(got, p) {
					t.Errorf("%q should have been kept; got %v", p, got)
				}
			}
		})
	}
}

// Pruning is not merely an optimization — it must produce the same listing as
// filtering after the fact would for the same patterns. This guards the case
// the perf strategy depends on.
func TestPruningMatchesPostFiltering(t *testing.T) {
	files := []string{
		"node_modules/pkg/index.js",
		"node_modules/pkg/sub/deep.js",
		"app/a.rb",
		"app/vendor/gem.rb",
	}
	exclude := []string{"node_modules", "**/vendor/**"}

	pruned, err := Walk(Options{FS: mapFS(files...), Exclude: exclude})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	full, err := Walk(Options{FS: mapFS(files...)})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	var want []string
	for _, p := range full.Paths() {
		if strings.HasPrefix(p, "node_modules") || strings.Contains(p, "vendor") {
			continue
		}
		want = append(want, p)
	}
	if got := pruned.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("pruned walk != post-filtered walk\n got: %v\nwant: %v", got, want)
	}
}

func TestInvalidPatternIsRejectedBeforeWalking(t *testing.T) {
	_, err := Walk(Options{FS: mapFS("a.txt"), Exclude: []string{"a[b"}})
	if err == nil {
		t.Fatal("expected an error for an invalid glob")
	}
	var pe *PatternError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As(*PatternError) = false; err type %T: %v", err, err)
	}
	if pe.Pattern != "a[b" {
		t.Errorf("Pattern = %q, want %q", pe.Pattern, "a[b")
	}
}

func TestWalkRealDirectory(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "app/services/billing/charge.rb")
	mustWrite(t, root, "app/services/billing/charge_spec.rb")
	mustWrite(t, root, "node_modules/left-pad/index.js")
	mustWrite(t, root, ".git/HEAD")

	l, err := Walk(Options{Root: root, Exclude: []string{"**/*_spec.rb", "node_modules"}})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	want := []string{
		"app",
		"app/services",
		"app/services/billing",
		"app/services/billing/charge.rb",
	}
	if got := l.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths()\n got: %v\nwant: %v", got, want)
	}
	if l.Root != root {
		t.Errorf("Root = %q, want %q", l.Root, root)
	}
}

// A symlink must not be followed: a link back up the tree would otherwise make
// the walk unbounded, and the perf target assumes it is bounded by the tree.
func TestSymlinksAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "real/deep/file.txt")
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	l, err := Walk(Options{Root: root})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	want := []string{"loop", "real", "real/deep", "real/deep/file.txt"}
	if got := l.Paths(); !reflect.DeepEqual(got, want) {
		t.Errorf("Paths()\n got: %v\nwant: %v", got, want)
	}
	for _, e := range l.Entries {
		if e.Path == "loop" && e.IsDir {
			t.Error("symlinked directory reported as a directory; the walk would follow it")
		}
	}
}

func TestEmptyRoot(t *testing.T) {
	l, err := Walk(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if l.Len() != 0 {
		t.Errorf("Len = %d, want 0: %v", l.Len(), l.Paths())
	}
	if got := l.Files(); len(got) != 0 {
		t.Errorf("Files = %v, want empty", got)
	}
}

func TestMissingRootIsAnError(t *testing.T) {
	_, err := Walk(Options{Root: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected an error for a missing root")
	}
	var we *Error
	if !errors.As(err, &we) {
		t.Fatalf("errors.As(*Error) = false; err type %T: %v", err, err)
	}
}

// An unreadable directory fails the walk rather than silently shrinking the
// listing. A short listing turns into phantom ORPHAN/UNMAPPED violations that
// look exactly like real drift.
func TestUnreadableDirectoryFailsLoudly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	mustWrite(t, root, "locked/secret.txt")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Skipf("chmod unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := Walk(Options{Root: root}); err == nil {
		t.Error("expected an error for an unreadable directory")
	}
}

func mustWrite(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
