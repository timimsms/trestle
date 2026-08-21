package integration

// The check engine is a pure function and its unit tests live in
// internal/check, where they run without touching a disk. What cannot be tested
// there is whether the engine agrees with the fixtures — and the fixtures are
// the contract. They were written in Phase 1, before the engine existed,
// precisely so the engine would be pinned down by its contract rather than by
// its implementation.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/expected"
	"github.com/timimsms/trestle/internal/nodes"
	"github.com/timimsms/trestle/internal/walk"
)

const repos = "../../testdata/repos"

// runFixture wires Phases 1, 2 and 3 together exactly as Phase 4's `check`
// command will have to: load config, walk once, resolve the diagram globs
// against that one listing, parse each diagram, and hand it all to the engine.
//
// Doing the wiring here rather than in a helper inside internal/check is the
// point. Every I/O call in this function is one the engine does not make.
func runFixture(t *testing.T, dir string) ([]check.Violation, *config.Config) {
	t.Helper()

	cfg, err := config.Load(filepath.Join(dir, config.Filename))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	listing, err := walk.Walk(walk.Options{Root: cfg.Root, Exclude: cfg.Exclude})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	files := make([]check.Entry, len(listing.Entries))
	for i, e := range listing.Entries {
		files[i] = check.Entry{Path: e.Path, IsDir: e.IsDir}
	}

	var diagrams []check.Diagram
	for _, path := range diagramPaths(t, cfg, listing) {
		d, err := nodes.ParseFile(filepath.Join(cfg.Root, path))
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		src, err := os.ReadFile(filepath.Join(cfg.Root, path))
		if err != nil {
			t.Fatal(err)
		}
		diagrams = append(diagrams, check.Diagram{
			Nodes:      d,
			Directives: directive.Parse(path, src),
		})
	}

	return check.Check(check.Input{Files: files, Diagrams: diagrams, Config: cfg}), cfg
}

// diagramPaths resolves `diagrams:` against the single listing rather than
// globbing the filesystem a second time. One walk, all globs (DESIGN §7).
func diagramPaths(t *testing.T, cfg *config.Config, l *walk.Listing) []string {
	t.Helper()
	var out []string
	for _, pat := range cfg.Diagrams {
		for _, e := range l.Entries {
			if e.IsDir {
				continue
			}
			if ok, err := doublestar.Match(pat, e.Path); err == nil && ok {
				out = append(out, e.Path)
			}
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatalf("no diagram matched %v", cfg.Diagrams)
	}
	return out
}

// asExpected converts engine output into the EXPECTED file's vocabulary.
func asExpected(vs []check.Violation) []expected.Violation {
	out := make([]expected.Violation, 0, len(vs))
	for _, v := range vs {
		out = append(out, expected.Violation{
			Code:   string(v.Code),
			Target: v.Target(),
			Detail: v.Detail,
		})
	}
	return out
}

// exitCode is the classification Phase 4 will apply. It is computed here, not
// in the engine: `check` reports severity and the caller decides consequences.
func exitCode(vs []check.Violation, strict bool) int {
	for _, v := range vs {
		if v.Failing() || strict {
			return 1
		}
	}
	return 0
}

// Every fixture must produce exactly its EXPECTED violation set. The comparison
// is on (code, target) — expected.Violation's Detail field is documented as
// advisory, and DiffCodes exists for exactly this: the pairs are the contract,
// the prose is there to make a failing diff legible.
func TestFixturesProduceTheirExpectedViolations(t *testing.T) {
	entries, err := os.ReadDir(repos)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			dir := filepath.Join(repos, e.Name())
			want, err := expected.Load(dir)
			if err != nil {
				t.Fatalf("load EXPECTED: %v", err)
			}

			got, _ := runFixture(t, dir)
			missing, extra := want.DiffCodes(asExpected(got))
			for _, v := range missing {
				t.Errorf("missing violation: %s %s", v.Code, v.Target)
			}
			for _, v := range extra {
				t.Errorf("unexpected violation: %s %s", v.Code, v.Target)
			}
			if len(missing) > 0 || len(extra) > 0 {
				for _, v := range got {
					t.Logf("got: %-9s %-28s %s | hint: %s", v.Code, v.Target(), v.Detail, v.Hint)
				}
			}

			warnings := 0
			for _, v := range got {
				if v.Severity == config.SeverityWarn {
					warnings++
				}
			}
			if warnings != want.Warnings {
				t.Errorf("warnings = %d, want %d", warnings, want.Warnings)
			}
			if code := exitCode(got, false); code != want.Exit {
				t.Errorf("exit = %d, want %d", code, want.Exit)
			}
			if want.HasStrictExit {
				if code := exitCode(got, false); code == 0 {
					// --strict promotes warnings, so the strict exit differs
					// only when warnings exist and nothing already failed.
					strict := 0
					if len(got) > 0 {
						strict = 1
					}
					if strict != want.StrictExit {
						t.Errorf("strict exit = %d, want %d", strict, want.StrictExit)
					}
				} else if want.StrictExit != 1 {
					t.Errorf("strict exit = 1, want %d", want.StrictExit)
				}
			}
		})
	}
	if seen < 10 {
		t.Errorf("only %d fixtures found under %s; the acceptance set is ten", seen, repos)
	}
}

// Every violation carries a runnable next step. It is a contract, not a nicety,
// and it is asserted against the fixtures because that is where the real
// violation shapes are.
func TestEveryFixtureViolationCarriesAHint(t *testing.T) {
	entries, err := os.ReadDir(repos)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		got, _ := runFixture(t, filepath.Join(repos, e.Name()))
		for _, v := range got {
			if strings.TrimSpace(v.Hint) == "" {
				t.Errorf("%s: %s %s has no hint", e.Name(), v.Code, v.Target())
			}
			if v.Source.File == "" {
				t.Errorf("%s: %s %s has no source position", e.Name(), v.Code, v.Target())
			}
		}
	}
}

// The worked example is a live test input, not a document. O8 and O9 exist
// because Gate B found that under strict string matching every one of its six
// binds is DANGLING and its two containers are UNBOUND. If the example ever
// stops passing its own check, one of those resolutions has regressed.
func TestWorkedExampleResolvesUnderO8AndO9(t *testing.T) {
	src, err := os.ReadFile("../../examples/repairs-platform/system.d2")
	if err != nil {
		t.Fatal(err)
	}
	d, err := nodes.Parse("system.d2", src)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse("/repo/.trestle.yml", []byte("version: 1\ndiagrams: [docs/architecture/*.d2]\n"))
	if err != nil {
		t.Fatal(err)
	}

	got := check.Check(check.Input{
		Diagrams: []check.Diagram{{Nodes: d, Directives: directive.Parse("system.d2", src)}},
		Config:   cfg,
	})

	// With no listing every bind is ORPHAN, which is expected and not what this
	// test is about. What must not appear is DANGLING (O8) or UNBOUND on a
	// container (O9).
	for _, v := range got {
		switch v.Code {
		case check.CodeDangling:
			t.Errorf("O8 regression: %s is DANGLING; unqualified IDs must resolve by dot-segment suffix", v.Target())
		case check.CodeSyntax:
			t.Errorf("unexpected SYNTAX in the shipped example: %s (%s)", v.Target(), v.Detail)
		case check.CodeUnbound:
			if v.Node == "platform" {
				t.Errorf("O9 regression: container %q warned; a container whose descendants are accounted for is accounted for", v.Node)
			}
		}
	}

	// `tenant` is a leaf with no directive and must still warn — O9 suppresses
	// containers, not modeling gaps.
	found := false
	for _, v := range got {
		if v.Code == check.CodeUnbound && v.Node == "tenant" {
			found = true
		}
	}
	if !found {
		t.Error("tenant is a leaf with no directive and must warn UNBOUND")
	}
}
