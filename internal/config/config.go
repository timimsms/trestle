// Package config loads, validates and defaults `.trestle.yml`.
//
// Discovery walks *up* from a starting directory to find the file. The
// directory containing it is the root, and every path in the system — globs in
// `@bind` directives, `discover:`, `shared:`, `exclude:` — is relative to it.
//
// Config errors are user-facing and are a tool error (exit 2), never a
// violation. Every error carries the line number and the offending key; this
// package never panics on malformed input.
package config

import "fmt"

// Filename is the config file Trestle looks for when walking up from CWD.
const Filename = ".trestle.yml"

// Version is the only supported `version:` value.
const Version = 1

// Severity is the consequence attached to a violation code.
type Severity string

// The three severity values. `off` suppresses a code entirely; it is legal, and
// it is the kind of thing `trestle explain` should surface.
const (
	SeverityFail Severity = "fail"
	SeverityWarn Severity = "warn"
	SeverityOff  Severity = "off"
)

// Severities lists every valid severity value, in the order error messages
// should name them.
var Severities = []Severity{SeverityFail, SeverityWarn, SeverityOff}

// Valid reports whether s is one of [Severities].
func (s Severity) Valid() bool {
	for _, v := range Severities {
		if s == v {
			return true
		}
	}
	return false
}

// The five violation codes, as they may appear as keys under `severity:`.
//
// The taxonomy is closed at five; do not add a sixth. These are restated here
// as plain strings rather than imported from internal/check because the
// dependency direction is check -> config and must stay that way.
const (
	CodeOrphan   = "ORPHAN"
	CodeUnmapped = "UNMAPPED"
	CodeDangling = "DANGLING"
	CodeUnbound  = "UNBOUND"
	CodeSyntax   = "SYNTAX"
)

// Codes lists every valid `severity:` key, in the order error messages name
// them.
var Codes = []string{CodeOrphan, CodeUnmapped, CodeDangling, CodeUnbound, CodeSyntax}

// DefaultSeverity is the severity of each code when `severity:` is absent or
// does not mention it. UNBOUND warns by design (O3): in the worked example it
// fired on a queue node, which was a modeling gap rather than an error, and
// failing by default would have trained a suppression reflex on the first
// diagram anyone wrote.
func DefaultSeverity() map[string]Severity {
	return map[string]Severity{
		CodeOrphan:   SeverityFail,
		CodeUnmapped: SeverityFail,
		CodeDangling: SeverityFail,
		CodeUnbound:  SeverityWarn,
		CodeSyntax:   SeverityFail,
	}
}

// DefaultExclude is applied when `exclude:` is absent. The patterns match the
// directory entry itself at any depth so that internal/walk can prune rather
// than descend — `**/x` matches `x` at the root as well as `a/b/x`.
func DefaultExclude() []string {
	return []string{"**/.git", "**/node_modules", "**/vendor"}
}

// Render holds the `render:` block. Trestle does not default these; Phase 6
// owns render behavior, and inventing an output directory here would put a
// path in the config that the author never wrote.
type Render struct {
	Out    string
	Layout string
	Theme  int
}

// Config is a validated `.trestle.yml` with defaults applied.
//
// Glob fields are stored exactly as authored, including trailing slashes:
// `discover: app/services/*/` names directories and `@bind app/services/x/**`
// names files, and conflating the two breaks UNMAPPED.
type Config struct {
	Version  int
	Diagrams []string
	Discover []string // absent is legal and means UNMAPPED never fires
	Shared   []string // enumerated, never blanket (L11)
	Exclude  []string
	Severity map[string]Severity // always populated for all five codes
	Render   Render

	// Root is the absolute directory containing the config file. Every
	// relative path in the system resolves against it.
	Root string
	// Path is the absolute path of the config file itself.
	Path string
}

// SeverityFor returns the configured severity of a violation code. Unknown
// codes — which validation rejects, so this is belt and braces — fail.
func (c *Config) SeverityFor(code string) Severity {
	if s, ok := c.Severity[code]; ok {
		return s
	}
	return SeverityFail
}

// String renders a one-line summary, for `explain` and for test failures.
func (c *Config) String() string {
	return fmt.Sprintf("config{root: %s, diagrams: %d, discover: %d, shared: %d, exclude: %d}",
		c.Root, len(c.Diagrams), len(c.Discover), len(c.Shared), len(c.Exclude))
}
