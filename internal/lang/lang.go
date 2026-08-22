// Package lang holds what Trestle knows about each ecosystem's conventions.
//
// It exists because that knowledge was accumulating in four places across three
// packages — the discover shapes in `scaffold`, the test-file globs in the
// emitted config, the node-ID prefixes inside `check`, and the exclude defaults
// in `config` — with the grouping expressed only in comments. Adding a language
// meant four edits, and nothing held them together.
//
// The scattering was already producing wrong behaviour rather than just
// friction. The node-ID prefixes (`svc_`, `job_`, `adp_`) are Rails vocabulary,
// and they live where nothing knows that: on a Go repo the hint suggests
// `svc_db` for a package whose name is `db`, which does not grep and collides
// with the `db_` prefix CONVENTIONS reserves for datastores.
//
// # Deliberately not a plugin system
//
// One file per ecosystem, each a [Lang] literal. No registry, no interface, no
// behaviour hooks — every field here is data some existing caller already
// needed, and there is field evidence for each one. Two languages is a thin
// basis for an abstraction, so this is the smallest thing that puts an
// ecosystem's facts in one place. A directory per language can come later if
// one ever needs assets rather than constants.
//
// This package imports nothing. That is what lets `internal/check` use the
// prefixes without widening what the engine depends on in any meaningful sense.
package lang

// Lang is one ecosystem's conventions.
type Lang struct {
	// Name is for messages, not matching.
	Name string

	// Markers are files whose presence means this ecosystem is in use.
	//
	// They gate detection rather than anchor it: a marker sits at the *module*
	// root and the *source* root is routinely somewhere else — a real Go repo
	// keeps go.mod at the top and every package under api/. So markers answer
	// "is this a Go repo", and anchoring is a separate question.
	//
	// Gating matters: without it, `cmd/*/` gets proposed in a Ruby repo that
	// happens to have a `cmd/` directory, and the author has to work out that
	// the tool guessed a language wrong before they can dismiss the rule.
	Markers []string

	// Discover are the layout shapes worth proposing, in the order a proposal
	// lists them. All depth 2, per Spike 01.
	//
	// Order is load-bearing where shapes nest: a specific shape must precede
	// the general one that contains it, so the specific one claims its
	// directories first.
	Discover []string

	// TestGlobs are exclude patterns the emitted config offers, commented out.
	//
	// Offered rather than applied, because excluding tests is a real decision
	// with a real cost, and at least one of these is a trap: uncommenting
	// `**/*_test.go` on a Go repo can *increase* failures, since an external
	// test package (`package foo_test`) may be the only file in its directory
	// and excluding it turns a healthy unit into an empty one.
	TestGlobs []string

	// Prefixes map a container directory name to the node-ID prefix
	// CONVENTIONS asks for: app/services/billing -> svc_billing.
	//
	// Empty for ecosystems where the convention does not apply. Go package
	// names are single lowercase words that are already the identifier, so a
	// prefix makes the ID stop matching the thing it names.
	Prefixes map[string]string
}

// All is every ecosystem Trestle recognizes.
//
// Order decides which shapes a proposal lists first when a repo uses more than
// one — a Rails app with a JS build is ordinary — and within that, Discover
// order decides which shape claims a directory both could match.
var All = []Lang{Rails, Go, Node}

// Detected returns the languages whose markers appear in the given file set.
//
// Falls back to everything when nothing is recognized. A repo with no marker
// file is not a repo with no layout, and proposing shapes that turn out not to
// match costs nothing: a rule is only ever offered when it matches a directory
// that has files in it.
func Detected(hasFile func(name string) bool) []Lang {
	var out []Lang
	for _, l := range All {
		for _, m := range l.Markers {
			if hasFile(m) {
				out = append(out, l)
				break
			}
		}
	}
	if len(out) == 0 {
		return All
	}
	return out
}

// Prefix returns the node-ID prefix for a container directory across every
// detected ecosystem, or "".
func Prefix(langs []Lang, container string) string {
	for _, l := range langs {
		if p, ok := l.Prefixes[container]; ok {
			return p
		}
	}
	return ""
}
