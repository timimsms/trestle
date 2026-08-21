package scaffold

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle/internal/walk"
)

// candidates are the layout shapes `init` knows how to recognize, in the order
// a proposal lists them.
//
// Every one of them is **depth 2** — one level inside a container directory —
// because that is what Spike 01 measured. On a 4,007-file repo, depth 2 yielded
// 64 units that correspond to boxes somebody would actually draw (`packages/db`,
// `ui/src`); depth 3 fragmented the same repo into 118 units with 35 of them
// holding two files or fewer. That is authoring burden, not architecture.
//
// The list is deliberately short and literal. O2 resolved toward configuration
// over heuristics: `init` is allowed to guess once, visibly, at setup time, and
// the user confirms before anything is written. `check` never guesses. Adding a
// shape here that is merely plausible — `*/` at the repo root, say — moves the
// guessing from "recognize a convention" to "invent one", and the cost lands on
// whoever has to triage the UNMAPPED list.
//
// The trailing slash is not decoration: `discover:` rules match directories, and
// `app/services/*` without it matches nothing (GAMEPLAN §8, the trailing-slash
// trap).
var candidates = []string{
	// Rails and its descendants: services and jobs are the two directories that
	// hold things people draw boxes around.
	"app/services/*/",
	"app/jobs/*/",
	// The same shape with no `app/` wrapper.
	"services/*/",
	// JS/TS monorepos. `packages/` and `apps/` are the npm/turbo/nx convention.
	"packages/*/",
	"apps/*/",
	// One `src/` holding a directory per subsystem.
	"src/*/",
	// Shared layers. These usually resolve into `shared:` rather than into
	// nodes, which is exactly why they are worth proposing: an entry in
	// `shared:` is an accountable exemption, and code that is in neither place
	// is code nobody has decided about.
	"lib/*/",
	// Go.
	"internal/*/",
	"pkg/*/",
	"cmd/*/",
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

	// Every candidate's matches, indexed by unit path, so file counting is one
	// pass over the listing rather than one pass per rule.
	type slot struct {
		rule int
		unit int
	}
	index := make(map[string]slot)
	rules := make([]Rule, 0, len(candidates))

	for _, glob := range candidates {
		pat := strings.TrimSuffix(glob, "/")
		r := Rule{Glob: glob}
		for _, e := range l.Entries {
			if !e.IsDir {
				continue
			}
			if ok, err := doublestar.Match(pat, e.Path); err != nil || !ok {
				continue
			}
			// A directory matched by two candidates would be counted twice and,
			// worse, proposed twice. The first shape in the list wins.
			if _, taken := index[e.Path]; taken {
				continue
			}
			index[e.Path] = slot{rule: len(rules), unit: len(r.Units)}
			r.Units = append(r.Units, Unit{Path: e.Path})
		}
		if len(r.Units) > 0 {
			rules = append(rules, r)
		}
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
