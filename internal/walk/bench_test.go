package walk

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The whole `trestle check` budget is 200ms on a 100k-file repo (DESIGN §7).
// The walk is one of four stages inside that, so it has to land well under the
// whole budget on its own — these benchmarks are the evidence, and the point of
// committing them is that a regression shows up as a number rather than as a
// vague "check feels slow" six months later.
//
// The tree is generated, never committed: 100k files in git would be a hostile
// thing to do to every clone. It is built once per `go test` process and reused
// by every benchmark here; TestMain removes it.

const (
	benchDirs         = 1000 // half under app/, half under node_modules/
	benchFilesPerDir  = 100  // 100k files total
	benchNestingDepth = 3    // app/services/g<NN>/u<NNN>/file
)

var (
	benchOnce sync.Once
	benchRoot string
	benchErr  error
)

// syntheticRepo builds (once) a tree shaped like a real repo that would be near
// the perf target: a large source tree plus a large vendored tree that config
// excludes.
func syntheticRepo(tb testing.TB) string {
	tb.Helper()
	benchOnce.Do(func() {
		benchRoot, benchErr = buildSyntheticRepo()
	})
	if benchErr != nil {
		tb.Fatalf("build synthetic repo: %v", benchErr)
	}
	return benchRoot
}

func buildSyntheticRepo() (string, error) {
	root, err := os.MkdirTemp("", "trestle-walk-bench-")
	if err != nil {
		return "", err
	}

	dirs := make([]string, 0, benchDirs)
	for i := 0; i < benchDirs/2; i++ {
		dirs = append(dirs, filepath.Join(root, "app", "services",
			fmt.Sprintf("g%02d", i/benchNestingDepth), fmt.Sprintf("u%03d", i)))
		dirs = append(dirs, filepath.Join(root, "node_modules",
			fmt.Sprintf("pkg%03d", i), "lib"))
	}

	// Creating 100k files serially dominates the setup; fan out across the
	// available cores so the benchmark is usable interactively.
	workers := 8
	jobs := make(chan string, len(dirs))
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dir := range jobs {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					errs <- err
					return
				}
				for f := 0; f < benchFilesPerDir; f++ {
					p := filepath.Join(dir, fmt.Sprintf("f%03d.go", f))
					if err := os.WriteFile(p, nil, 0o644); err != nil {
						errs <- err
						return
					}
				}
			}
		}()
	}
	for _, d := range dirs {
		jobs <- d
	}
	close(jobs)
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}

	// A .git directory large enough that failing to skip it would show up.
	for i := 0; i < 100; i++ {
		d := filepath.Join(root, ".git", "objects", fmt.Sprintf("%02x", i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
		for f := 0; f < 20; f++ {
			if err := os.WriteFile(filepath.Join(d, fmt.Sprintf("o%02d", f)), nil, 0o644); err != nil {
				return "", err
			}
		}
	}
	return root, nil
}

func TestMain(m *testing.M) {
	code := m.Run()
	if benchRoot != "" {
		_ = os.RemoveAll(benchRoot)
	}
	os.Exit(code)
}

// BenchmarkWalk100k walks the full synthetic tree with no exclude patterns —
// the worst case, and the number to compare the others against.
func BenchmarkWalk100k(b *testing.B) {
	root := syntheticRepo(b)
	entries := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := Walk(Options{Root: root})
		if err != nil {
			b.Fatal(err)
		}
		entries = l.Len()
	}
	b.StopTimer()
	if entries < 100000 {
		b.Fatalf("listing has %d entries; the synthetic tree was not built as expected", entries)
	}
	b.ReportMetric(float64(entries), "entries")
}

// BenchmarkWalk100kPruned is the case the design actually cares about: half the
// tree is vendored and excluded, and pruning during the walk means those 50k
// files are never stat'd. Compare against BenchmarkWalk100kFiltered.
func BenchmarkWalk100kPruned(b *testing.B) {
	root := syntheticRepo(b)
	entries := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := Walk(Options{Root: root, Exclude: []string{"node_modules", "**/*_test.*"}})
		if err != nil {
			b.Fatal(err)
		}
		entries = l.Len()
	}
	b.StopTimer()
	b.ReportMetric(float64(entries), "entries")
}

// BenchmarkWalk100kFiltered is the design being argued against: walk everything,
// then throw half of it away. It exists so the pruning claim is measured rather
// than asserted.
func BenchmarkWalk100kFiltered(b *testing.B) {
	root := syntheticRepo(b)
	entries := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := Walk(Options{Root: root})
		if err != nil {
			b.Fatal(err)
		}
		// Note the pattern differs from the pruned benchmark: post-filtering has
		// to spell out "node_modules/**" because filtering sees paths one at a
		// time and a bare "node_modules" would drop only the directory entry.
		// Pruning gets the subtree for free. That asymmetry is the semantics,
		// not an accident — see Walk's doc comment.
		m, err := newMatcher([]string{"node_modules/**", "**/*_test.*"})
		if err != nil {
			b.Fatal(err)
		}
		kept := l.Entries[:0]
		for _, e := range l.Entries {
			if !m.match(e.Path) {
				kept = append(kept, e)
			}
		}
		entries = len(kept)
	}
	b.StopTimer()
	b.ReportMetric(float64(entries), "entries")
}

// BenchmarkWalkExcludePatternCount isolates the per-path cost of the exclude
// matcher: patterns are checked against every path, so a config with a long
// exclude list pays linearly.
func BenchmarkWalkExcludePatternCount(b *testing.B) {
	root := syntheticRepo(b)
	pats := []string{
		"node_modules", "**/*_test.*", "**/vendor/**", "**/*.min.js",
		"**/dist/**", "**/*.generated.*", "tmp/**", "**/*.snap",
	}
	for _, n := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("patterns=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := Walk(Options{Root: root, Exclude: pats[:n]}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
