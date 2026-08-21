package check

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// The budget is 200ms for the whole of `trestle check` on a 100k-file repo
// (DESIGN §7), and internal/walk already spends 29–59ms of it. The engine's
// share therefore has to be small, and "small" has to be a committed number
// rather than a feeling — a check slower than a lint rule gets moved to a
// nightly job, and a nightly job stops blocking the PR that broke it.
//
// Unlike walk's benchmark, nothing here touches a disk. The listing is a value,
// which is the whole point of the seam.

const (
	benchGroups        = 100 // app/services/g00 .. g99
	benchUnitsPerGroup = 10  // .../u0 .. u9
	benchFilesPerUnit  = 100
)

var (
	benchOnce    sync.Once
	benchListing []Entry
)

// syntheticListing builds a 100k-file listing shaped like a monorepo at the
// depth Gate A recommended: two levels of grouping under app/services, a
// hundred files in each leaf.
func syntheticListing() []Entry {
	benchOnce.Do(func() {
		total := benchGroups*benchUnitsPerGroup*benchFilesPerUnit + benchGroups*benchUnitsPerGroup + benchGroups + 2
		out := make([]Entry, 0, total)
		out = append(out, Entry{Path: "app", IsDir: true}, Entry{Path: "app/services", IsDir: true})
		for g := 0; g < benchGroups; g++ {
			gp := fmt.Sprintf("app/services/g%02d", g)
			out = append(out, Entry{Path: gp, IsDir: true})
			for u := 0; u < benchUnitsPerGroup; u++ {
				up := fmt.Sprintf("%s/u%d", gp, u)
				out = append(out, Entry{Path: up, IsDir: true})
				for f := 0; f < benchFilesPerUnit; f++ {
					out = append(out, Entry{Path: fmt.Sprintf("%s/f%03d.go", up, f)})
				}
			}
		}
		benchListing = newIndex(out).entries
	})
	return benchListing
}

// benchDiagram compiles a diagram with n bound nodes, once. Compiling D2 is
// Phase 2's cost and would otherwise swamp the measurement.
func benchDiagram(tb testing.TB, n int) Diagram {
	tb.Helper()
	var src strings.Builder
	for g := 0; g < n; g++ {
		fmt.Fprintf(&src, "# @bind svc_g%02d app/services/g%02d/**\n", g, g)
	}
	for g := 0; g < n; g++ {
		fmt.Fprintf(&src, "svc_g%02d: Group %d\n", g, g)
	}
	d, err := nodes.Parse("system.d2", []byte(src.String()))
	if err != nil {
		tb.Fatal(err)
	}
	return Diagram{Nodes: d, Directives: directive.Parse("system.d2", []byte(src.String()))}
}

func benchConfig() *config.Config {
	return &config.Config{
		Version:  1,
		Severity: config.DefaultSeverity(),
		Discover: []string{"app/services/*/"},
		Shared:   []string{"lib/http_client/**"},
		Root:     "/repo",
		Path:     "/repo/.trestle.yml",
	}
}

// BenchmarkCheck100k is the acceptance number: the whole engine, twenty
// bindings, a discover rule and a shared entry, against a 100k-file listing.
// Twenty bindings is the case PHASE_3 singles out — naive glob-per-path costs
// ~140ms there, which is most of the budget on its own.
func BenchmarkCheck100k(b *testing.B) {
	files := syntheticListing()
	in := Input{Files: files, Diagrams: []Diagram{benchDiagram(b, 20)}, Config: benchConfig()}

	var violations int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		violations = len(Check(in))
	}
	b.StopTimer()

	if len(files) < 100000 {
		b.Fatalf("listing has %d entries; the synthetic tree was not built as expected", len(files))
	}
	b.ReportMetric(float64(len(files)), "entries")
	b.ReportMetric(float64(violations), "violations")
}

// TestCheck100kWithinBudget is the benchmark asserted rather than admired. A
// number nobody checks is a number that regresses, and the failure mode here is
// gradual: the engine gets slower, the check gets moved to nightly, and nightly
// stops blocking the PR that broke it.
//
// The threshold is the whole-command budget, not the engine's share, so it has
// roughly fifty times headroom over the measured cost. It is a tripwire for a
// design regression — a reintroduced glob-per-path loop — not a microbenchmark,
// and it must never be tightened to the point where a loaded CI box fails it.
func TestCheck100kWithinBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 100k-entry listing")
	}
	files := syntheticListing()
	in := Input{Files: files, Diagrams: []Diagram{benchDiagram(t, 20)}, Config: benchConfig()}

	Check(in) // warm any lazily built state before timing
	start := time.Now()
	got := Check(in)
	elapsed := time.Since(start)

	if len(got) == 0 {
		t.Fatal("expected violations; the benchmark input is not exercising the engine")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Check over %d entries took %v, over the 200ms whole-command budget (DESIGN §7)",
			len(files), elapsed)
	}
}

// BenchmarkCheckBindingCount is the shape of the cost curve. The naive matcher
// is linear in bindings × listing size; narrowing by literal prefix makes each
// binding cost the directory it names, so this should be close to flat.
func BenchmarkCheckBindingCount(b *testing.B) {
	files := syntheticListing()
	for _, n := range []int{5, 20, 50, 100} {
		b.Run(fmt.Sprintf("bindings=%d", n), func(b *testing.B) {
			in := Input{Files: files, Diagrams: []Diagram{benchDiagram(b, n)}, Config: benchConfig()}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Check(in)
			}
		})
	}
}

// BenchmarkGlobFullScan is the design being argued against, kept so the
// narrowing claim is measured rather than asserted: one glob, every path,
// straight through doublestar. Multiply by the binding count to get what the
// naive engine would have cost.
func BenchmarkGlobFullScan(b *testing.B) {
	files := syntheticListing()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := 0
		for _, e := range files {
			if ok, _ := doublestar.Match("app/services/g07/**", e.Path); ok {
				n++
			}
		}
		if n == 0 {
			b.Fatal("pattern matched nothing")
		}
	}
}

// BenchmarkGlobNarrowed is the same question asked the way the engine asks it.
func BenchmarkGlobNarrowed(b *testing.B) {
	ix := newIndex(syntheticListing())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := ix.eachFile("app/services/g07/**", nil); n == 0 {
			b.Fatal("pattern matched nothing")
		}
	}
}
