package scaffold

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/lang"
	"github.com/timimsms/trestle/internal/walk"
)

// candidatesFor returns the layout shapes worth proposing for this repo, in
// proposal order, drawn from the ecosystems whose marker files are present.
//
// Gating on markers is a correctness fix, not tidying. Without it every shape
// is tried everywhere, so a Ruby repo that happens to hold a `cmd/` directory
// gets offered `cmd/*/` and the author has to work out that the tool guessed a
// language wrong before they can dismiss the rule.
//
// Every shape is depth 2, per Spike 01: on a 4,007-file repo depth 2 yielded
// units that correspond to boxes somebody would actually draw, while depth 3
// fragmented the same repo into 118 units with 35 holding two files or fewer.
// That is authoring burden, not architecture.
//
// The trailing slash is not decoration: `discover:` rules match directories,
// and `app/services/*` without it matches nothing (GAMEPLAN §8).
func candidatesFor(l *walk.Listing) []string {
	var out []string
	for _, lg := range DetectLangs(l) {
		out = append(out, lg.Discover...)
	}
	return out
}

// DetectLangs reports which ecosystems a repo appears to use, from its marker
// files. Pure — markers are just paths in the listing.
func DetectLangs(l *walk.Listing) []lang.Lang {
	if l == nil {
		return lang.All
	}
	present := make(map[string]bool, len(l.Entries))
	for _, e := range l.Entries {
		if !e.IsDir {
			present[e.Path] = true
		}
	}
	return lang.Detected(func(name string) bool { return present[name] })
}

// anchors returns the directories a layout shape could start at: the repo root
// and every top-level directory.
//
// Anchoring only at the repo root is wrong whenever the source tree is not
// there, which is ordinary. A real Go repo keeps `go.mod` at the top and its
// packages under `api/`, so `internal/*/` and `cmd/*/` matched nothing, `init`
// proposed zero rules, and it wrote `discover: []` — a config under which
// UNMAPPED can never fire. The shapes were right; they were one directory too
// high. `api/`, `server/`, `backend/`, `src/` as the home of the real tree is
// common in every language.
//
// A marker file like go.mod is not the signal, because it sits at the module
// root rather than at the source root, and those are routinely different.
//
// Over-proposal is bounded by what already governs every other shape: a rule is
// only offered when it matches a directory that has files beneath it, nested
// units are dropped, and the whole proposal is shown for confirmation before a
// byte is written. `init` is allowed to guess once, visibly (O2).
func anchors(l *walk.Listing) []string {
	out := []string{""}
	for _, e := range l.Entries {
		if !e.IsDir || strings.Contains(e.Path, "/") {
			continue
		}
		if strings.HasPrefix(e.Path, ".") {
			continue
		}
		out = append(out, e.Path)
	}
	return out
}

// Unit is one directory a proposed `discover:` rule matches today.
type Unit struct {
	// Path is repo-relative and slash-separated, with no trailing slash.
	Path string
	// Files counts the non-excluded files anywhere beneath Path. A unit with
	// zero of them can never be covered — O10 makes coverage file-level — so it
	// will report UNMAPPED however the diagram is written.
	Files int
}

// Name is the final path segment, which is what a proposal prints.
func (u Unit) Name() string { return path.Base(u.Path) }

// Hidden reports whether the unit is a dot-directory. `*` matches those, so a
// `.venv` or a `.cache` under a matched root becomes something the repo has to
// account for. A proposal says so rather than letting it surface as an UNMAPPED
// three minutes later.
func (u Unit) Hidden() bool { return strings.HasPrefix(u.Name(), ".") }

// Rule is one proposed `discover:` rule together with what it matches right now.
//
// The match list is the whole point of showing a proposal at all: a `discover:`
// rule nobody agreed to is a source of UNMAPPED noise, and a noisy check gets
// bypassed within a week.
type Rule struct {
	// Glob is the rule as it would be written into `.trestle.yml`, trailing
	// slash included.
	Glob string
	// Units holds the directories Glob matches, in listing order.
	Units []Unit
}

// Hidden counts the dot-directories among the matched units.
func (r Rule) Hidden() int {
	n := 0
	for _, u := range r.Units {
		if u.Hidden() {
			n++
		}
	}
	return n
}

// slot locates a matched unit: which rule proposed it, and where in that rule's
// list it sits. Kept so file counting is one pass over the listing.
type slot struct {
	rule int
	unit int
}

// matchGlobs turns globs into rules, claiming each directory once.
//
// index is shared across calls so a second pass cannot re-propose a directory
// the first already claimed; base is the rule offset those claims were recorded
// against, so the indices stay correct when rules are appended.
func matchGlobs(l *walk.Listing, globs []string, index map[string]slot, base int) []Rule {
	out := make([]Rule, 0, len(globs))
	for _, glob := range globs {
		pat := strings.TrimSuffix(glob, "/")
		r := Rule{Glob: glob}
		for _, e := range l.Entries {
			if !e.IsDir {
				continue
			}
			if ok, err := doublestar.Match(pat, e.Path); err != nil || !ok {
				continue
			}
			// A directory matched by two shapes would be counted twice and,
			// worse, proposed twice. The first shape wins.
			if _, taken := index[e.Path]; taken {
				continue
			}
			index[e.Path] = slot{rule: base + len(out), unit: len(r.Units)}
			r.Units = append(r.Units, Unit{Path: e.Path})
		}
		if len(r.Units) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// Detect proposes `discover:` rules for a repo, given the one walk.
//
// It is a pure function of the listing — no filesystem access, no config — so
// the shapes it recognizes can be tested against a synthetic tree. A shape is
// proposed only when it matches at least one directory that has at least one
// file somewhere beneath it: a rule matching nothing is an ORPHAN the moment it
// is written, and seeding one would mean `init` shipped a config that fails its
// own check.
func Detect(l *walk.Listing) []Rule {
	if l == nil {
		return nil
	}

	roots := anchors(l)
	candidates := candidatesFor(l)

	// Matches indexed by unit path, so file counting is one pass over the
	// listing rather than one pass per rule, and so a directory two shapes both
	// match is claimed once.
	index := make(map[string]slot)

	anchored := make([]string, 0, len(candidates)*len(roots))
	for _, root := range roots {
		for _, glob := range candidates {
			if root == "" {
				anchored = append(anchored, glob)
				continue
			}
			anchored = append(anchored, root+"/"+glob)
		}
	}

	rules := matchGlobs(l, anchored, index, 0)

	// Second pass: a workspace container matched above is not a unit, so
	// reach the packages inside it. dropNestedUnits then removes the container,
	// since it now contains proposed units.
	if extra := containerGlobs(l, rules, DetectLangs(l)); len(extra) > 0 {
		rules = append(rules, matchGlobs(l, extra, index, len(rules))...)
	}

	for _, e := range l.Entries {
		if e.IsDir {
			continue
		}
		for dir := path.Dir(e.Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if s, ok := index[dir]; ok {
				rules[s.rule].Units[s.unit].Files++
			}
		}
	}

	rules = dropNestedUnits(rules)

	// A shape whose every match is empty is a shape this repo does not really
	// use — a leftover directory, or one git is not even tracking.
	out := rules[:0]
	for _, r := range rules {
		total := 0
		for _, u := range r.Units {
			total += u.Files
		}
		if total > 0 {
			out = append(out, r)
		}
	}
	return out
}

// containerGlobs finds proposed units that are containers of units rather than
// units, and returns the deeper globs that reach the real ones.
//
// A pnpm/npm workspace nests: astro's `packages/integrations/` holds fourteen
// published packages and is not one itself. `packages/*/` matches the container,
// so a single binding on `packages/integrations/**` owns every adapter — and a
// new adapter lands with nothing firing. That is precisely the blindspot
// UNMAPPED exists to close, seeded by default on the shape npm repos actually
// use, since `pnpm-workspace.yaml` says `packages/**/*` rather than
// `packages/*`.
//
// The test is a marker file, which is exactly what makes it safe: a directory
// that has no package.json while its children do is a container by the
// ecosystem's own definition, not by a guess about names.
//
// Deliberately restricted to directories a base shape already matched. Run it
// over the whole listing and `examples/` qualifies — twenty-five example
// projects, each with a package.json, none of them architecture. Astro's
// detection correctly leaves those alone, and this must not undo that.
func containerGlobs(l *walk.Listing, rules []Rule, langs []lang.Lang) []string {
	var markers []string
	for _, lg := range langs {
		markers = append(markers, lg.Markers...)
	}
	if len(markers) == 0 {
		return nil
	}

	files := make(map[string]bool, len(l.Entries))
	for _, e := range l.Entries {
		if !e.IsDir {
			files[e.Path] = true
		}
	}
	hasMarker := func(dir string) bool {
		for _, m := range markers {
			if files[dir+"/"+m] {
				return true
			}
		}
		return false
	}

	dirs := make([]string, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.IsDir {
			dirs = append(dirs, e.Path)
		}
	}

	seen := make(map[string]bool)
	var out []string
	for _, r := range rules {
		for _, u := range r.Units {
			if hasMarker(u.Path) {
				continue // a package in its own right
			}
			children := 0
			for _, d := range dirs {
				if path.Dir(d) == u.Path && hasMarker(d) {
					children++
				}
			}
			// One child is a directory that happens to hold a package, not a
			// container of them. Two is a shape.
			if children < 2 {
				continue
			}
			glob := u.Path + "/*/"
			if !seen[glob] {
				seen[glob] = true
				out = append(out, glob)
			}
		}
	}
	return out
}

// dropNestedUnits removes any proposed unit that contains another proposed
// unit, and any rule left with none.
//
// `discover:` units must not nest. A repo with both `app/services/` and the
// broader `app/*/` shape would otherwise be offered `app/services` *and*
// `app/services/billing` as separate things needing owners — the outer one can
// never be satisfied without also claiming the inner one, so it reports
// UNMAPPED forever no matter how the diagram is written.
//
// The deeper unit wins, which is what Spike 01 measured: at depth 2 a unit is a
// box somebody would draw, and its parent is the container it lives in.
func dropNestedUnits(rules []Rule) []Rule {
	all := make(map[string]bool)
	for _, r := range rules {
		for _, u := range r.Units {
			all[u.Path] = true
		}
	}

	contains := func(p string) bool {
		for other := range all {
			if len(other) > len(p) && strings.HasPrefix(other, p+"/") {
				return true
			}
		}
		return false
	}

	out := rules[:0]
	for _, r := range rules {
		kept := r.Units[:0]
		for _, u := range r.Units {
			if contains(u.Path) {
				continue
			}
			kept = append(kept, u)
		}
		r.Units = kept
		if len(r.Units) > 0 {
			out = append(out, r)
		}
	}
	return out
}
