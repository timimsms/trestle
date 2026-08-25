// Package report formats check results and classifies them into an exit code.
//
// It is the only place that turns violations into something a human or a
// machine reads, and the only place that knows what `--strict` means. The check
// engine reports severity; this package decides consequences. Keeping the two
// apart is what lets `--strict` change the exit code without rewriting the
// severity field a JSON consumer relies on.
//
// # Paths are relativized here
//
// Violations sourced from a diagram already carry a repo-relative
// Source.File. Violations sourced from the config — UNMAPPED, and the ORPHANs
// raised by `shared:` and `discover:` — carry config.Path, which is absolute.
// Printing that verbatim makes output machine-specific: a golden file recorded
// on one checkout will not match on another. [Options.Root] is stripped here,
// once, and the result is re-sorted so ordering is a property of what was
// printed rather than of where the repo happens to live.
package report

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
)

// SchemaVersion is the `version` field of the JSON document. It is 1 from day
// one (docs/DESIGN.md §5): this output is consumed by CI and by agents,
// and adding a version after the fact means guessing what produced a payload.
const SchemaVersion = 1

// Format is an output format. There are two, and `--format` accepts no others.
type Format string

// The two output formats.
const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

// Formats lists every valid `--format` value, in the order help text and error
// messages should name them.
var Formats = []Format{FormatHuman, FormatJSON}

// ParseFormat validates a `--format` value.
func ParseFormat(s string) (Format, error) {
	for _, f := range Formats {
		if string(f) == s {
			return f, nil
		}
	}
	names := make([]string, 0, len(Formats))
	for _, f := range Formats {
		names = append(names, string(f))
	}
	return "", fmt.Errorf("unknown format %q; valid formats are %s", s, strings.Join(names, ", "))
}

// Options controls how a result is rendered.
type Options struct {
	// Root is the repo root. Absolute source paths are made relative to it.
	Root string

	// Strict reports that warnings count as failures for the exit code. It
	// never changes a violation's severity: in JSON it surfaces as the
	// top-level `strict` field, and in human output as a note under the
	// summary. A flag that rewrote the data would leave a JSON consumer unable
	// to tell a warning from a failure.
	Strict bool

	// Color enables ANSI colorization of human output. Callers set it from
	// [UseColor]; it is off whenever stdout is not a terminal, because CI logs
	// full of escape sequences are how people stop reading CI logs.
	Color bool

	// Disabled lists codes turned off in `.trestle.yml`, from
	// [check.DisabledCodes]. Rendering it is not decoration: a repo that sets
	// ORPHAN and UNMAPPED to `off` otherwise gets an unqualified `0 failures, 0
	// warnings` from a check that inspected nothing. A green result must never
	// be able to mean "nothing was looked at".
	Disabled []check.Code

	// Coverage reports how much of the repo `discover:` actually watches, from
	// [check.Measure]. Printing it is the whole point: the first two real repos
	// Trestle met both produced a green check over near-zero coverage — one
	// watching 0 files, one watching 27 of 600 — and nothing in any output said
	// so. The scope of a result belongs next to the result.
	Coverage check.Coverage
}

// coverageNote renders the discover-scope clause for the summary line.
//
// It always says something when UNMAPPED cannot fire, including when no rules
// are configured at all. DESIGN allows that state and it is silent by nature,
// which is exactly the problem: `init` writes `discover: []` whenever its
// layout detection comes up empty, and the first real Go repo Trestle met got
// precisely that — a green check, exit 0, watching nothing, with no output
// anywhere admitting it.
//
// A deliberate opt-out costs one clause per run. That is a fair price for the
// alternative, which is a permanently passing badge over zero coverage. Same
// reasoning as [DisabledCodes]: turning a check off is legal, doing it
// invisibly is not.
func coverageNote(c check.Coverage) string {
	switch {
	case c.Rules == 0:
		return " · no discover rules — UNMAPPED cannot fire"
	case !c.Watched():
		// Rules exist and match nothing. UNMAPPED cannot fire, so a clean run
		// here is not evidence of anything.
		return " · discover: no directories matched — UNMAPPED cannot fire"
	default:
		return fmt.Sprintf(" · discover: %d of %d files",
			c.Files, c.TotalFiles)
	}
}

// disabledNote renders the parenthetical that qualifies a summary line, or ""
// when every code is live.
func disabledNote(disabled []check.Code) string {
	if len(disabled) == 0 {
		return ""
	}
	names := make([]string, len(disabled))
	for i, c := range disabled {
		names[i] = string(c)
	}
	return fmt.Sprintf(" (%s off)", strings.Join(names, ", "))
}

// Summary is the failure and warning count of a run. It is what the summary
// line and the JSON `summary` object both report, and it is computed from
// severity alone — `--strict` does not touch it.
type Summary struct {
	Failures int
	Warnings int
}

// Summarize counts violations by severity.
func Summarize(vs []check.Violation) Summary {
	var s Summary
	for _, v := range vs {
		switch v.Severity {
		case config.SeverityFail:
			s.Failures++
		case config.SeverityWarn:
			s.Warnings++
		}
	}
	return s
}

// ExitCode classifies a run: 0 when nothing failed, 1 when something did.
//
// Exit 2 is a tool error and never originates here — it means Trestle could not
// do its job, and the caller reaches it by returning an error instead of a
// result. Keeping 1 and 2 distinct is what lets CI tell "your diagram is wrong"
// from "Trestle is broken"; conflating them trains people to ignore both.
//
// `--strict` promotes warnings at exactly this point and nowhere else.
func ExitCode(vs []check.Violation, strict bool) int {
	for _, v := range vs {
		if v.Failing() || (strict && v.Severity == config.SeverityWarn) {
			return 1
		}
	}
	return 0
}

// Write renders violations in the requested format.
func Write(w io.Writer, vs []check.Violation, format Format, opt Options) error {
	prepared := prepare(vs, opt.Root)
	switch format {
	case FormatJSON:
		return writeJSON(w, prepared, opt)
	case FormatHuman:
		return writeHuman(w, prepared, opt)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// prepare returns a copy of vs with every Source.File made repo-relative, in
// the order output presents it.
//
// The engine already sorts by (file, line, code, node, path) and that ordering
// is DESIGN §5's grouping. It sorts on the paths it was given, though, and one
// of those is absolute — so the sort has to be redone on the relativized paths
// or the grouping depends on where the repo is checked out. The comparator is
// the engine's, restated on public API.
func prepare(vs []check.Violation, root string) []check.Violation {
	out := make([]check.Violation, len(vs))
	copy(out, vs)
	for i := range out {
		out[i].Source.File = relPath(out[i].Source.File, root)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Source.File != b.Source.File {
			return a.Source.File < b.Source.File
		}
		if a.Source.Line != b.Source.Line {
			return a.Source.Line < b.Source.Line
		}
		if ra, rb := codeRank(a.Code), codeRank(b.Code); ra != rb {
			return ra < rb
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		return a.Path < b.Path
	})
	return out
}

// relPath makes an absolute source path relative to the repo root, and leaves
// anything else — including a path outside the root — exactly as it is. A path
// that cannot be expressed relative to the root is better shown in full than
// shown as a pile of "..".
func relPath(file, root string) string {
	if file == "" || root == "" || !filepath.IsAbs(file) {
		return file
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return file
	}
	return filepath.ToSlash(rel)
}

// codeRank orders codes the way check.Codes does: the directive-level problems
// that explain the others come first.
func codeRank(c check.Code) int {
	for i, k := range check.Codes {
		if k == c {
			return i
		}
	}
	return len(check.Codes)
}
