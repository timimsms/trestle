// Package expected parses the EXPECTED contract files that accompany every
// fixture repo under testdata/repos/.
//
// One EXPECTED file per fixture. It states what `trestle check` must produce
// when run with that fixture directory as the root. Phase 3 (the check engine)
// and Phase 4 (the CLI) both consume these files; neither reimplements the
// parse.
//
// # Format
//
//	exit: 1
//	violations:
//	  ORPHAN  svc_billing  app/services/billing/**
//	warnings: 0
//	strict_exit: 1
//
// Keys:
//
//   - exit         — expected process exit code with default flags: 0, 1 or 2.
//   - violations   — one indented line per expected violation, or the literal
//     `violations: none`. Order-independent; the comparison is
//     set-based.
//   - warnings     — count of warning-severity violations. Those violations also
//     appear under `violations:`; this is a redundant
//     cross-check and it is deliberate.
//   - strict_exit  — expected exit code under `--strict`, which promotes every
//     warning to a failure. Optional, but present in every
//     committed fixture. Absence is reported via HasStrictExit
//     rather than guessed at, because inferring it would mean
//     this package deciding severity, which is the engine's job.
//
// A violation line is `CODE  target  detail...`:
//
//   - CODE   — one of the five, and only the five: ORPHAN, UNMAPPED, DANGLING,
//     UNBOUND, SYNTAX. An unknown code is a parse error, which is what
//     stops a sixth code from arriving by accident.
//   - target — the node ID when the violation is about a node, otherwise the
//     path or glob it is about. Required.
//   - detail — free-form remainder, whitespace-collapsed. Optional, and
//     advisory: it exists so a failing fixture is legible in a diff.
//     Compare on it only where the exact string is part of the
//     contract; Diff compares all three fields, DiffCodes does not.
//
// Blank lines are ignored, and so is any line whose first non-space character
// is `#`.
package expected

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Filename is the name every fixture uses for its contract file.
const Filename = "EXPECTED"

// Codes is the closed set of violation codes. There are five. Do not add a
// sixth without a ledger entry — see GAMEPLAN §6.
var Codes = []string{"ORPHAN", "UNMAPPED", "DANGLING", "UNBOUND", "SYNTAX"}

func validCode(c string) bool {
	for _, k := range Codes {
		if k == c {
			return true
		}
	}
	return false
}

// Violation is one expected violation line.
type Violation struct {
	Code   string
	Target string
	Detail string
}

func (v Violation) String() string {
	if v.Detail == "" {
		return v.Code + "  " + v.Target
	}
	return v.Code + "  " + v.Target + "  " + v.Detail
}

// Expectation is a parsed EXPECTED file.
type Expectation struct {
	Exit       int
	Violations []Violation
	Warnings   int

	// StrictExit is the expected exit code under `--strict`. Valid only when
	// HasStrictExit is true.
	StrictExit    int
	HasStrictExit bool
}

// Load reads dir/EXPECTED.
func Load(dir string) (Expectation, error) {
	return ParseFile(filepath.Join(dir, Filename))
}

// ParseFile reads and parses a single EXPECTED file.
func ParseFile(path string) (Expectation, error) {
	f, err := os.Open(path)
	if err != nil {
		return Expectation{}, err
	}
	defer func() { _ = f.Close() }()

	e, err := Parse(f)
	if err != nil {
		return Expectation{}, fmt.Errorf("%s: %w", path, err)
	}
	return e, nil
}

// Parse reads an EXPECTED file from r.
func Parse(r io.Reader) (Expectation, error) {
	var (
		e            Expectation
		inViolations bool
		seen         = map[string]bool{}
		sc           = bufio.NewScanner(r)
		lineNo       int
	)

	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// An indented line inside the violations block is a violation.
		// Anything flush-left ends the block.
		indented := raw != line
		if inViolations && indented {
			v, err := parseViolation(line)
			if err != nil {
				return Expectation{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			e.Violations = append(e.Violations, v)
			continue
		}
		inViolations = false

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Expectation{}, fmt.Errorf("line %d: not a `key: value` line: %q", lineNo, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if seen[key] {
			return Expectation{}, fmt.Errorf("line %d: duplicate key %q", lineNo, key)
		}
		seen[key] = true

		switch key {
		case "exit", "strict_exit":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Expectation{}, fmt.Errorf("line %d: %s: not a number: %q", lineNo, key, value)
			}
			if n < 0 || n > 2 {
				return Expectation{}, fmt.Errorf("line %d: %s: must be 0, 1 or 2, got %d", lineNo, key, n)
			}
			if key == "exit" {
				e.Exit = n
			} else {
				e.StrictExit, e.HasStrictExit = n, true
			}
		case "warnings":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Expectation{}, fmt.Errorf("line %d: warnings: not a number: %q", lineNo, value)
			}
			if n < 0 {
				return Expectation{}, fmt.Errorf("line %d: warnings: must not be negative", lineNo)
			}
			e.Warnings = n
		case "violations":
			switch value {
			case "none":
			case "":
				inViolations = true
			default:
				return Expectation{}, fmt.Errorf(
					"line %d: violations: expected `none` or an indented list, got %q", lineNo, value)
			}
		default:
			return Expectation{}, fmt.Errorf("line %d: unknown key %q", lineNo, key)
		}
	}
	if err := sc.Err(); err != nil {
		return Expectation{}, err
	}

	if !seen["exit"] {
		return Expectation{}, fmt.Errorf("missing required key %q", "exit")
	}
	if !seen["violations"] {
		return Expectation{}, fmt.Errorf("missing required key %q", "violations")
	}
	if !seen["warnings"] {
		return Expectation{}, fmt.Errorf("missing required key %q", "warnings")
	}
	if e.Warnings > len(e.Violations) {
		return Expectation{}, fmt.Errorf(
			"warnings: %d warnings declared but only %d violations listed; warning-severity violations must also appear under `violations:`",
			e.Warnings, len(e.Violations))
	}
	return e, nil
}

func parseViolation(line string) (Violation, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Violation{}, fmt.Errorf("violation needs at least `CODE target`: %q", line)
	}
	if !validCode(fields[0]) {
		return Violation{}, fmt.Errorf("unknown violation code %q (the five are %s)",
			fields[0], strings.Join(Codes, ", "))
	}
	return Violation{
		Code:   fields[0],
		Target: fields[1],
		Detail: strings.Join(fields[2:], " "),
	}, nil
}

// Count returns how many expected violations carry the given code.
func (e Expectation) Count(code string) int {
	n := 0
	for _, v := range e.Violations {
		if v.Code == code {
			n++
		}
	}
	return n
}

// Diff compares got against the expectation as a multiset of
// (Code, Target, Detail) triples. Both return slices are sorted and empty when
// got matches exactly.
func (e Expectation) Diff(got []Violation) (missing, extra []Violation) {
	return diff(e.Violations, got, func(v Violation) Violation { return v })
}

// DiffCodes is Diff ignoring Detail. Use it when the engine's detail string is
// still settling and only the (Code, Target) pairs are contractual.
func (e Expectation) DiffCodes(got []Violation) (missing, extra []Violation) {
	return diff(e.Violations, got, func(v Violation) Violation {
		return Violation{Code: v.Code, Target: v.Target}
	})
}

func diff(want, got []Violation, key func(Violation) Violation) (missing, extra []Violation) {
	counts := map[Violation]int{}
	for _, v := range want {
		counts[key(v)]++
	}
	for _, v := range got {
		k := key(v)
		if counts[k] > 0 {
			counts[k]--
			continue
		}
		extra = append(extra, v)
	}
	for _, v := range want {
		k := key(v)
		if counts[k] > 0 {
			counts[k]--
			missing = append(missing, v)
		}
	}
	sortViolations(missing)
	sortViolations(extra)
	return missing, extra
}

func sortViolations(vs []Violation) {
	sort.Slice(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Detail < b.Detail
	})
}
