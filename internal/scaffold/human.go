package scaffold

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// unitsShown caps the directory names printed next to a proposed rule. The
// count is the number that matters; the names are there so the reader can tell
// at a glance whether the rule found architecture or found `node_modules`, and
// six is enough for that.
const unitsShown = 6

// printer buffers output the way internal/report does: bufio.Writer latches the
// first write error and hands it back from Flush, so the individual writes have
// nothing to handle and the `_, _ =` noise stays out of the prose below.
type printer struct{ w *bufio.Writer }

func (o printer) printf(format string, a ...any) { _, _ = fmt.Fprintf(o.w, format, a...) }

// WriteProposal prints what `init` intends to do, before it does any of it.
//
// This is the whole reason `init` is interactive. O2 resolved `discover:`
// toward configuration over heuristics because a rule the user did not agree to
// is a source of UNMAPPED noise, and a noisy check gets bypassed inside a week.
// `init` is allowed to guess once, visibly, at setup time — so the guess has to
// be legible: the rule, how many directories it matches *today*, and which ones.
func (p *Plan) WriteProposal(w io.Writer) error {
	o := printer{w: bufio.NewWriter(w)}
	o.printf("trestle init — %s\n", p.Root)

	if len(p.Rules) == 0 {
		o.printf("\nNo conventional layout found here — none of the shapes `init` recognizes\n" +
			"(app/services/*/, packages/*/, src/*/, lib/*/, internal/*/, cmd/*/ and a few\n" +
			"others) matched a directory with files in it. `discover:` will be written\n" +
			"empty, which means UNMAPPED never fires and nothing will tell you when new\n" +
			"code appears with no box. Fill it in by hand once you know the shape.\n")
	} else {
		if p.Existing != nil {
			o.printf("\nThe `discover:` rules `init` would seed from this layout. Your `.trestle.yml`\n" +
				"is kept exactly as it is — this is here so a second run after the repo grows\n" +
				"a new shape is worth something.\n\n")
		} else {
			o.printf("\n`discover:` rules seeded from the layout found here. Every directory one of\n" +
				"these matches needs an owner — a `@bind` on some diagram, or an entry in\n" +
				"`shared:` — so these rules decide how much the first `trestle check` has to\n" +
				"say. Delete the ones that are not architecture.\n\n")
		}
		width, countWidth := 0, 0
		for _, r := range p.Rules {
			if len(r.Glob) > width {
				width = len(r.Glob)
			}
			if n := len(plural(len(r.Units), "directory", "directories")); n > countWidth {
				countWidth = n
			}
		}
		for _, r := range p.Rules {
			count := plural(len(r.Units), "directory", "directories")
			suffix := ""
			if p.Existing != nil && !p.covered(r) {
				suffix = "   <- not in your `discover:`"
			}
			o.printf("  %-*s  %-*s  %s%s\n", width, r.Glob, countWidth, count, names(r.Units), suffix)
			if n := r.Hidden(); n > 0 {
				what := "a dot-directory"
				if n > 1 {
					what = "dot-directories"
				}
				o.printf("  %-*s  (%d of those %s %s — `exclude:` them if they are not architecture)\n",
					width, "", n, verb(n), what)
			}
		}
	}

	o.printf("\nFiles:\n\n")
	for _, a := range p.Artifacts {
		o.printf("  %-9s %s%s\n", a.Action, a.Path, because(a.Why))
	}

	if p.startsDiagram() {
		o.printf("\nThe starter diagram is written with no nodes in it. Trestle does not invent\n" +
			"boxes: a diagram generated from your directory listing would pass its own\n" +
			"check while telling you nothing.\n")

		// "0 UNMAPPED — one per directory above" is nonsense when there is no
		// directory above, and it was what a real repo saw: detection found
		// nothing, and the transcript still promised a to-do list. A run that
		// watches nothing needs to say so, not describe a list it cannot produce.
		if p.Units() > 0 {
			o.printf("So the first `trestle check` will report %d UNMAPPED — one per\n"+
				"directory above — and each one carries the exact `@bind` line to paste\n"+
				"into the diagram. That is the to-do list, not a verdict on the repo.\n",
				p.Units())
		} else {
			o.printf("With no `discover:` rules, the first `trestle check` will report nothing\n" +
				"at all — not because the repo is clean, but because nothing is being\n" +
				"watched. `check` says so on its summary line every run until you fill\n" +
				"`discover:` in.\n")
		}
	}
	return o.w.Flush()
}

// WriteResult prints what happened, then what to run next.
func (p *Plan) WriteResult(w io.Writer) error {
	o := printer{w: bufio.NewWriter(w)}
	for _, a := range p.Artifacts {
		did := map[Action]string{Create: "wrote", Append: "appended to", Keep: "kept", Unchanged: "unchanged"}[a.Action]
		o.printf("  %-12s %s%s\n", did, a.Path, because(a.Why))
	}

	o.printf("\nNext:\n\n")
	if p.Units() > 0 && p.startsDiagram() {
		o.printf("  trestle check      %s to account for. Each UNMAPPED names the\n"+
			"                     `@bind` line that fixes it; paste it into the diagram, or\n"+
			"                     put the path in `shared:` if it is plumbing nobody draws.\n",
			plural(p.Units(), "directory", "directories"))
	} else {
		o.printf("  trestle check      validate every binding against the files on disk\n")
	}
	o.printf("  trestle explain    every node Trestle parsed, and what each binding matches\n")
	o.printf("  trestle render     SVGs into %s\n", RenderOut)
	o.printf("\nRead CONVENTIONS.md before editing a diagram. It is the contract.\n")
	return o.w.Flush()
}

// because renders the reason attached to an artifact, or nothing.
func because(why string) string {
	if why == "" {
		return ""
	}
	return "  — " + why
}

// covered reports whether the repo's existing `discover:` list already contains
// a proposed rule. The trailing slash is optional on both sides: `check` treats
// `app/services/*` and `app/services/*/` as the same rule, and a report that
// called them different would be reporting on Trestle rather than on the repo.
func (p *Plan) covered(r Rule) bool {
	want := strings.TrimSuffix(r.Glob, "/")
	for _, have := range p.Existing {
		if strings.TrimSuffix(strings.TrimSpace(have), "/") == want {
			return true
		}
	}
	return false
}

// startsDiagram reports whether this run creates the empty starter diagram —
// the case where the first check is an inventory rather than a verdict.
func (p *Plan) startsDiagram() bool {
	for _, a := range p.Artifacts {
		if a.Path == DiagramPath && a.Action == Create {
			return true
		}
	}
	return false
}

func names(units []Unit) string {
	shown := units
	suffix := ""
	if len(units) > unitsShown {
		shown = units[:unitsShown]
		suffix = fmt.Sprintf(", and %d more", len(units)-unitsShown)
	}
	out := make([]string, 0, len(shown))
	for _, u := range shown {
		out = append(out, u.Name())
	}
	return strings.Join(out, ", ") + suffix
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func verb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
