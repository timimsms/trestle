package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/report"
	"github.com/timimsms/trestle/internal/run"
)

// DESIGN §7 puts the whole of `trestle check` under 200ms on a 100k-file repo.
// internal/walk and internal/check each benchmark their own stage; this is the
// number that actually matters, because it is the one a user waits for: config
// discovery, one walk, diagram resolution, D2 compilation, the engine, and
// formatting.
//
// The tree is generated, never committed — 100k files in git would be a hostile
// thing to do to every clone. It is built once per process and removed by
// TestMain in walk's benchmark style.

const (
	benchServices = 500 // app/services/svc_NNN/, each bound by one @bind
	benchFiles    = 100 // files per directory: 50k source files
	benchVendored = 500 // node_modules/pkg_NNN/, excluded and pruned
)

var (
	benchOnce sync.Once
	benchRoot string
	benchErr  error
)

func syntheticRepo(tb testing.TB) string {
	tb.Helper()
	benchOnce.Do(func() { benchRoot, benchErr = buildSyntheticRepo() })
	if benchErr != nil {
		tb.Fatalf("build synthetic repo: %v", benchErr)
	}
	return benchRoot
}

func buildSyntheticRepo() (string, error) {
	root, err := os.MkdirTemp("", "trestle-cli-bench-")
	if err != nil {
		return "", err
	}

	var diagram strings.Builder
	diagram.WriteString("# Synthetic repo, 500 services.\n")
	for i := 0; i < benchServices; i++ {
		fmt.Fprintf(&diagram, "# @bind svc_%03d app/services/svc_%03d/**\n", i, i)
	}
	diagram.WriteString("\nplatform: Platform {\n")
	for i := 0; i < benchServices; i++ {
		fmt.Fprintf(&diagram, "  svc_%03d: Service %d\n", i, i)
	}
	diagram.WriteString("}\n")

	files := map[string]string{
		".trestle.yml": "version: 1\n" +
			"diagrams:\n  - docs/architecture/*.d2\n" +
			"discover:\n  - app/services/*/\n" +
			"exclude:\n  - node_modules\n  - \"**/*_test.go\"\n",
		"docs/architecture/system.d2": diagram.String(),
	}
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			return "", err
		}
	}

	dirs := make([]string, 0, benchServices+benchVendored)
	for i := 0; i < benchServices; i++ {
		dirs = append(dirs, filepath.Join(root, "app", "services", fmt.Sprintf("svc_%03d", i)))
	}
	for i := 0; i < benchVendored; i++ {
		dirs = append(dirs, filepath.Join(root, "node_modules", fmt.Sprintf("pkg_%03d", i), "lib"))
	}

	jobs := make(chan string, len(dirs))
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dir := range jobs {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					errs <- err
					return
				}
				for f := 0; f < benchFiles; f++ {
					p := filepath.Join(dir, fmt.Sprintf("f%03d.go", f))
					if err := os.WriteFile(p, nil, 0o600); err != nil {
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
	return root, nil
}

// BenchmarkCheckEndToEnd is the 200ms budget, measured on the whole command
// minus process startup: 100k files on disk, half of them pruned by `exclude:`,
// 500 bound services and a 500-node diagram.
func BenchmarkCheckEndToEnd(b *testing.B) {
	root := syntheticRepo(b)
	files := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, err := run.Load(root)
		if err != nil {
			b.Fatal(err)
		}
		vs := ctx.Check()
		if err := report.Write(io.Discard, vs, report.FormatHuman, report.Options{Root: ctx.Config.Root}); err != nil {
			b.Fatal(err)
		}
		files = ctx.Listing.Len()
		if len(vs) != 0 {
			b.Fatalf("synthetic repo should be clean, got %d violations: %v", len(vs), vs[0])
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(files), "entries")
}

// BenchmarkCheckEndToEndNoExclude removes `exclude:` so the listing really is
// 100k entries rather than 100k files half of which are pruned. It is the
// pessimistic reading of the budget, and the one to quote if anyone asks
// whether the 200ms holds without a vendored tree to skip.
func BenchmarkCheckEndToEndNoExclude(b *testing.B) {
	root := syntheticRepo(b)
	cfg, err := config.Parse(filepath.Join(root, config.Filename),
		[]byte("version: 1\ndiagrams:\n  - docs/architecture/*.d2\ndiscover:\n  - app/services/*/\nexclude: []\n"))
	if err != nil {
		b.Fatal(err)
	}

	files := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, err := run.LoadConfig(cfg)
		if err != nil {
			b.Fatal(err)
		}
		vs := ctx.Check()
		if err := report.Write(io.Discard, vs, report.FormatHuman, report.Options{Root: cfg.Root}); err != nil {
			b.Fatal(err)
		}
		files = ctx.Listing.Len()
	}
	b.StopTimer()
	b.ReportMetric(float64(files), "entries")
}
