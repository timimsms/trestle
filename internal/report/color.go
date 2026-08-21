package report

import (
	"io"
	"os"

	"github.com/timimsms/trestle/internal/config"
)

// Color is opt-in on evidence, never on assumption. PHASE_4 is blunt about why:
// CI logs full of ANSI escapes are how people stop reading CI logs. So the only
// thing that turns color on is a writer that is demonstrably a terminal, and
// two environment conventions can still turn it off.
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
)

// UseColor reports whether human output written to w should be colorized.
//
// It is true only when w is a character device — a terminal. A pipe, a file, a
// bytes.Buffer and a CI log all read as not-a-terminal and get plain text.
// NO_COLOR (any non-empty value) and TERM=dumb override it to false, because
// both are conventions users already have set for exactly this purpose.
func UseColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// palette renders the four things human output colorizes, or renders them
// unchanged when color is off. With color off every method is the identity
// function, which is what makes the golden files the plain text they are.
type palette struct{ on bool }

func newPalette(on bool) palette { return palette{on: on} }

func (p palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return code + s + ansiReset
}

// code colors a violation code by severity: failures red, warnings yellow.
// Anything else — an `off` code should never reach output — stays plain.
func (p palette) code(sev config.Severity, s string) string {
	switch sev {
	case config.SeverityFail:
		return p.wrap(ansiRed, s)
	case config.SeverityWarn:
		return p.wrap(ansiYellow, s)
	default:
		return s
	}
}

// file renders a group header.
func (p palette) file(s string) string { return p.wrap(ansiBold, s) }

// dim renders secondary text — the `hint:` label and the --strict note.
func (p palette) dim(s string) string { return p.wrap(ansiDim, s) }
