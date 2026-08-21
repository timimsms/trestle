package report

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
)

// The human format is DESIGN §5, reproduced exactly. It is a golden file, not a
// suggestion:
//
//	docs/architecture/system.d2
//
//	  ORPHAN    svc_billing
//	            @bind app/services/billing/** matches 0 files
//	            hint: renamed? `git log --diff-filter=D -- app/services/billing`
//
//	2 failures, 0 warnings
//
// Violations are grouped by the file their source position names, the code
// column is padded to ten, and detail and hint align under the target. Every
// violation prints a hint line; an empty hint is a golden-test failure and that
// is the point.
const (
	indent    = "  "
	codeWidth = 10
	// hintLabel prefixes the third line of a block. It is part of the format,
	// not decoration: a check that does not say what to type is one people
	// learn to route around, so the word appears whether or not color does.
	hintLabel = "hint: "
)

// bodyIndent aligns detail and hint under the target column.
var bodyIndent = strings.Repeat(" ", len(indent)+codeWidth)

// out buffers human output.
//
// bufio.Writer latches the first write error and hands it back from Flush, so
// the individual writes genuinely have nothing to handle. This type says that
// once, in one place, instead of scattering `_, _ =` down the file.
type out struct{ w *bufio.Writer }

func (o out) printf(format string, a ...any) { _, _ = fmt.Fprintf(o.w, format, a...) }

// plural renders a count with its noun, pluralized by adding "s". Every noun it
// is used with is regular; it is not a general-purpose inflector.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func writeHuman(w io.Writer, vs []check.Violation, opt Options) error {
	o := out{w: bufio.NewWriter(w)}
	p := newPalette(opt.Color)

	file := ""
	first := true
	for _, v := range vs {
		if first || v.Source.File != file {
			file = v.Source.File
			// Each block already ends in a blank line, so a new header needs no
			// separator of its own.
			o.printf("%s\n\n", p.file(headerFor(v)))
			first = false
		}
		writeBlock(o, p, v)
	}

	s := Summarize(vs)
	// The disabled note rides on the summary line rather than sitting above it:
	// the summary is the one line everyone reads, and "0 failures" must never
	// be readable without the qualification that two codes were switched off.
	//
	// DESIGN §5 only ever shows plural counts, so it does not pin the singular.
	// Printing "1 warnings" reads as a bug in the tool every time someone sees
	// it, which is a bad property for the one line everybody reads.
	o.printf("%s, %s%s\n",
		plural(s.Failures, "failure"),
		plural(s.Warnings, "warning"),
		disabledNote(opt.Disabled))
	if opt.Strict && s.Warnings > 0 {
		// Without this line `--strict` prints "0 failures" and exits 1, which
		// reads as a bug in the tool rather than as the flag doing its job.
		o.printf("%s\n", p.dim("--strict: warnings count as failures"))
	}
	return o.w.Flush()
}

// headerFor is the group header: the file the violation's source position
// names. For a violation raised by the config — UNMAPPED, and the ORPHANs from
// `shared:` and `discover:` — that is `.trestle.yml`, because the config is
// what made the claim and no single diagram is more responsible than another.
func headerFor(v check.Violation) string {
	if v.Source.File != "" {
		return v.Source.File
	}
	return "(unknown source)"
}

func writeBlock(o out, p palette, v check.Violation) {
	code := string(v.Code)
	pad := codeWidth - len(code)
	if pad < 1 {
		pad = 1
	}
	o.printf("%s%s%s%s%s\n",
		indent, p.code(v.Severity, code), strings.Repeat(" ", pad), v.Target(), severityMark(p, v))

	if d := strings.TrimSpace(v.Detail); d != "" {
		o.printf("%s%s\n", bodyIndent, d)
	}
	// The hint line prints even when the hint is empty. Rendering it as a bare
	// `hint:` is the whole point: a missing hint has to be visible in the golden
	// file rather than absent from it. The label loses its trailing space in
	// that case, because a golden file full of invisible trailing whitespace is
	// its own kind of unreviewable.
	hint := strings.TrimSpace(v.Hint)
	label := hintLabel
	if hint == "" {
		label = strings.TrimRight(hintLabel, " ")
	}
	o.printf("%s%s%s\n\n", bodyIndent, p.dim(label), hint)
}

// severityMark marks warnings inline. Color alone cannot carry it — the format
// is plain whenever stdout is not a terminal, which is exactly the CI log where
// telling a warning from a failure matters most.
func severityMark(p palette, v check.Violation) string {
	if v.Severity == config.SeverityWarn {
		return "  " + p.code(v.Severity, "(warn)")
	}
	return ""
}
