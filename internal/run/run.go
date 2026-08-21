// Package run wires the parse, walk and check packages into the one pipeline
// every Trestle command needs: find the config, walk the repo once, resolve the
// configured diagram globs against that single listing, parse each diagram, and
// hand the result to the check engine.
//
// It exists so that `cmd/trestle` can stay what PHASE_4 says it must be — find
// root, load config, walk, parse, compile, call check, format, exit — without
// any of those steps being a decision made in `cmd/`. Two of them are decisions:
//
//   - `diagrams:` is resolved against the [walk.Listing], not by globbing the
//     filesystem a second time. DESIGN §7's 200ms budget is spent on one walk,
//     and a second traversal here would quietly undo that.
//   - Zero matched diagrams is an error, not an empty run. A check with nothing
//     to check exits 0 while inspecting nothing, which is the silent-pass
//     failure mode the whole tool exists to prevent.
//
// `explain`, `render` and `init` need the same context. Reinventing the loop in
// each command is how the per-diagram coverage bug GAMEPLAN §3 warns about gets
// reintroduced, so the loop lives here once.
//
// Every error this package returns is a tool error — exit 2 — never a
// violation. Violations come back from [Context.Check] as data.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
	"github.com/timimsms/trestle/internal/walk"
)

// Context is one loaded repo: the config, the single listing, and every
// configured diagram already parsed. It is the input to every command.
type Context struct {
	// Config is the validated .trestle.yml. Config.Root is the repo root.
	Config *config.Config

	// Listing is the one filesystem walk, with `exclude:` already pruned.
	Listing *walk.Listing

	// Paths holds the repo-relative, slash-separated path of every diagram
	// that matched `diagrams:`, sorted and deduplicated. It is never empty; a
	// zero match is reported as [NoDiagramsError] instead.
	Paths []string

	// Diagrams pairs each parsed diagram with the directives scanned from the
	// same bytes, in the same order as Paths.
	Diagrams []check.Diagram
}

// NoDiagramsError reports that `diagrams:` matched no file.
//
// This is exit 2 and not a clean run on purpose (PHASE_4 §"Command wiring"):
// with no diagrams there are no nodes, no directives and therefore no
// violations, so every other outcome would be a green check that read nothing.
type NoDiagramsError struct {
	// Patterns is `diagrams:` as authored.
	Patterns []string
	// ConfigPath is the .trestle.yml the patterns came from.
	ConfigPath string
}

func (e *NoDiagramsError) Error() string {
	quoted := make([]string, 0, len(e.Patterns))
	for _, p := range e.Patterns {
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	return fmt.Sprintf(
		"%s: diagrams: no file matches %s; there is nothing to check\n"+
			"  hint: create a diagram at one of those paths, or fix `diagrams:` — "+
			"and check `exclude:`, which hides a path from `diagrams:` as well",
		e.ConfigPath, strings.Join(quoted, ", "))
}

// Load finds `.trestle.yml` by walking up from startDir and loads everything
// the commands need. Every failure is a tool error.
func Load(startDir string) (*Context, error) {
	cfg, err := config.LoadFrom(startDir)
	if err != nil {
		return nil, err
	}
	return LoadConfig(cfg)
}

// LoadConfig is [Load] for a config that has already been read — the seam tests
// and future commands use to supply a config without a discovery walk.
func LoadConfig(cfg *config.Config) (*Context, error) {
	listing, err := walk.Walk(walk.Options{Root: cfg.Root, Exclude: cfg.Exclude})
	if err != nil {
		return nil, err
	}

	paths := MatchDiagrams(cfg.Diagrams, listing)
	if len(paths) == 0 {
		return nil, &NoDiagramsError{Patterns: cfg.Diagrams, ConfigPath: cfg.Path}
	}

	diagrams := make([]check.Diagram, 0, len(paths))
	for _, rel := range paths {
		src, err := os.ReadFile(filepath.Join(cfg.Root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read diagram %s: %w", rel, err)
		}
		// Both parsers get the repo-relative path, not the absolute one. Every
		// Source.File a violation carries is then already relative, which is
		// what makes output portable across machines and golden-testable.
		// Reading once and parsing the same bytes twice also means the D2 AST
		// and the directive scan can never disagree about the file's contents.
		d, err := nodes.Parse(rel, src)
		if err != nil {
			return nil, err
		}
		diagrams = append(diagrams, check.Diagram{
			Nodes:      d,
			Directives: directive.Parse(rel, src),
		})
	}

	return &Context{Config: cfg, Listing: listing, Paths: paths, Diagrams: diagrams}, nil
}

// Check runs the engine over the loaded context. It makes no decisions: the
// violations come back with severities resolved from config, and the caller
// decides what they mean for the exit code.
func (c *Context) Check() []check.Violation {
	files := make([]check.Entry, len(c.Listing.Entries))
	for i, e := range c.Listing.Entries {
		files[i] = check.Entry{Path: e.Path, IsDir: e.IsDir}
	}
	return check.Check(check.Input{Files: files, Diagrams: c.Diagrams, Config: c.Config})
}

// MatchDiagrams resolves `diagrams:` patterns against the one listing and
// returns the matching file paths, sorted and deduplicated.
//
// It never touches the filesystem. A pattern that matches nothing contributes
// nothing; only the empty union is an error, and that is [LoadConfig]'s call.
func MatchDiagrams(patterns []string, l *walk.Listing) []string {
	if l == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		for _, e := range l.Entries {
			if e.IsDir || seen[e.Path] {
				continue
			}
			if ok, err := doublestar.Match(pat, e.Path); err == nil && ok {
				seen[e.Path] = true
				out = append(out, e.Path)
			}
		}
	}
	sort.Strings(out)
	return out
}
