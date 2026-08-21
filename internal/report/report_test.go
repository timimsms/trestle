package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/report"
)

func render(t *testing.T, vs []check.Violation, f report.Format, opt report.Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := report.Write(&buf, vs, f, opt); err != nil {
		t.Fatalf("write %s: %v", f, err)
	}
	return buf.String()
}

// TestHumanMatchesDesignSection5 pins the format against the specification
// verbatim. DESIGN §5 is a golden file, not a suggestion: the two-space indent,
// the ten-column code field, the detail and hint aligned under the target, the
// blank line between blocks and the summary at the end are all load-bearing.
//
// One deliberate difference from the document: the UNMAPPED violation below is
// given the diagram as its source. The real engine sources UNMAPPED from
// `.trestle.yml` — the `discover:` rule is what went unsatisfied, and no single
// diagram is more responsible than another — so it groups under the config file
// instead. That is a difference in the data, not in the format, and this test is
// about the format.
func TestHumanMatchesDesignSection5(t *testing.T) {
	src := directive.Position{File: "docs/architecture/system.d2", Line: 4}
	vs := []check.Violation{
		{
			Code:     check.CodeOrphan,
			Severity: config.SeverityFail,
			Node:     "svc_billing",
			Path:     "app/services/billing/**",
			Source:   src,
			Detail:   "@bind app/services/billing/** matches 0 files",
			Hint:     "renamed? `git log --diff-filter=D -- app/services/billing`",
		},
		{
			Code:     check.CodeUnmapped,
			Severity: config.SeverityFail,
			Path:     "app/services/notifications/",
			Source:   directive.Position{File: "docs/architecture/system.d2", Line: 9},
			Detail:   "no @bind glob covers this path",
			Hint:     "add `# @bind svc_notifications app/services/notifications/**`",
		},
	}

	want := "docs/architecture/system.d2\n" +
		"\n" +
		"  ORPHAN    svc_billing\n" +
		"            @bind app/services/billing/** matches 0 files\n" +
		"            hint: renamed? `git log --diff-filter=D -- app/services/billing`\n" +
		"\n" +
		"  UNMAPPED  app/services/notifications/\n" +
		"            no @bind glob covers this path\n" +
		"            hint: add `# @bind svc_notifications app/services/notifications/**`\n" +
		"\n" +
		"2 failures, 0 warnings\n"

	if got := render(t, vs, report.FormatHuman, report.Options{}); got != want {
		t.Errorf("human output does not match DESIGN §5\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// The summary prints on a clean run too. Silence on success reads as "did not
// run", and a check nobody believes ran is a check nobody trusts.
func TestSummaryPrintsOnSuccess(t *testing.T) {
	got := render(t, nil, report.FormatHuman, report.Options{})
	if got != "0 failures, 0 warnings\n" {
		t.Errorf("clean run printed %q, want the summary line", got)
	}
}

// Warnings print alongside failures and are marked in the text, not only in the
// color. Color is off in every CI log, which is exactly where telling the two
// apart matters.
func TestWarningsAreMarkedInPlainText(t *testing.T) {
	vs := []check.Violation{{
		Code:     check.CodeUnbound,
		Severity: config.SeverityWarn,
		Node:     "queue_dispatch",
		Source:   directive.Position{File: "a.d2", Line: 3},
		Detail:   "no @bind, @external, @infra or @ignore",
		Hint:     "add `# @infra queue_dispatch`",
	}}
	got := render(t, vs, report.FormatHuman, report.Options{})
	if !strings.Contains(got, "queue_dispatch  (warn)") {
		t.Errorf("warning not marked in plain text:\n%s", got)
	}
	if !strings.Contains(got, "0 failures, 1 warning") {
		t.Errorf("summary did not count the warning:\n%s", got)
	}
}

// An empty hint has to be visible. The hint line prints unconditionally so that
// a violation without one shows up as a bare `hint:` in the golden file rather
// than as a line that quietly is not there.
func TestEmptyHintStillPrintsTheLabel(t *testing.T) {
	vs := []check.Violation{{
		Code:     check.CodeOrphan,
		Severity: config.SeverityFail,
		Node:     "x",
		Source:   directive.Position{File: "a.d2", Line: 1},
		Detail:   "something",
	}}
	got := render(t, vs, report.FormatHuman, report.Options{})
	if !strings.Contains(got, "            hint:\n") {
		t.Errorf("empty hint did not print a visible label:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("output line has trailing whitespace: %q", line)
		}
	}
}

// Absolute source paths — every config-sourced violation carries one, because
// config.Path is absolute — are made repo-relative before printing. Without
// this a golden file records the machine it was generated on.
func TestAbsoluteConfigPathsAreRelativized(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "somerepo")
	vs := []check.Violation{{
		Code:     check.CodeUnmapped,
		Severity: config.SeverityFail,
		Path:     "app/services/notifications/",
		Source:   directive.Position{File: filepath.Join(root, config.Filename)},
		Detail:   "no @bind glob covers this path",
		Hint:     "add a @bind",
	}}

	human := render(t, vs, report.FormatHuman, report.Options{Root: root})
	if strings.Contains(human, root) {
		t.Errorf("human output leaked the absolute root:\n%s", human)
	}
	if !strings.HasPrefix(human, config.Filename+"\n") {
		t.Errorf("group header is not the relative config path:\n%s", human)
	}

	doc := decode(t, render(t, vs, report.FormatJSON, report.Options{Root: root}))
	if got := doc.Violations[0].Source.File; got != config.Filename {
		t.Errorf("json source.file = %q, want %q", got, config.Filename)
	}
}

// A path that cannot be expressed under the root is shown in full rather than
// as a pile of "..".
func TestPathOutsideRootIsLeftAlone(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "elsewhere", "other.yml")
	vs := []check.Violation{{
		Code: check.CodeOrphan, Severity: config.SeverityFail, Path: "x",
		Source: directive.Position{File: abs}, Detail: "d", Hint: "h",
	}}
	root := filepath.Join(string(filepath.Separator), "tmp", "somerepo")
	if got := render(t, vs, report.FormatHuman, report.Options{Root: root}); !strings.Contains(got, abs) {
		t.Errorf("path outside the root was rewritten:\n%s", got)
	}
}

// Grouping and ordering must be a property of what is printed, not of where the
// repo lives. The engine sorts on the paths it was handed, one of which is
// absolute; report re-sorts after relativizing.
func TestOrderingIsStableAfterRelativizing(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "zzz-last-alphabetically")
	vs := []check.Violation{
		{
			Code: check.CodeUnmapped, Severity: config.SeverityFail, Path: "app/x/",
			Source: directive.Position{File: filepath.Join(root, config.Filename)},
			Detail: "d", Hint: "h",
		},
		{
			Code: check.CodeOrphan, Severity: config.SeverityFail, Node: "n",
			Source: directive.Position{File: "docs/architecture/system.d2", Line: 3},
			Detail: "d", Hint: "h",
		},
	}
	got := render(t, vs, report.FormatHuman, report.Options{Root: root})
	if !strings.HasPrefix(got, config.Filename+"\n") {
		t.Errorf(".trestle.yml should sort first once relativized:\n%s", got)
	}
	if strings.Index(got, "docs/architecture/system.d2") < strings.Index(got, config.Filename) {
		t.Errorf("groups are out of order:\n%s", got)
	}
}

func TestExitCode(t *testing.T) {
	fail := check.Violation{Code: check.CodeOrphan, Severity: config.SeverityFail}
	warn := check.Violation{Code: check.CodeUnbound, Severity: config.SeverityWarn}

	cases := []struct {
		name   string
		vs     []check.Violation
		strict bool
		want   int
	}{
		{"clean", nil, false, 0},
		{"clean strict", nil, true, 0},
		{"warning only", []check.Violation{warn}, false, 0},
		{"warning only strict", []check.Violation{warn}, true, 1},
		{"failure", []check.Violation{fail}, false, 1},
		{"failure strict", []check.Violation{fail}, true, 1},
		{"both", []check.Violation{warn, fail}, false, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := report.ExitCode(c.vs, c.strict); got != c.want {
				t.Errorf("ExitCode = %d, want %d", got, c.want)
			}
		})
	}
}

// --strict is an exit-code decision and nothing else. Rewriting the severity
// field would leave a JSON consumer unable to tell a warning from a failure,
// which is the whole reason the flag reports itself separately.
func TestStrictDoesNotRewriteSeverity(t *testing.T) {
	vs := []check.Violation{{
		Code: check.CodeUnbound, Severity: config.SeverityWarn, Node: "queue_dispatch",
		Source: directive.Position{File: "a.d2", Line: 3}, Detail: "d", Hint: "h",
	}}

	doc := decode(t, render(t, vs, report.FormatJSON, report.Options{Strict: true}))
	if !doc.Strict {
		t.Error("strict flag not reported in JSON")
	}
	if doc.Violations[0].Severity != string(config.SeverityWarn) {
		t.Errorf("severity = %q under --strict, want warn", doc.Violations[0].Severity)
	}
	if doc.Summary.Failures != 0 || doc.Summary.Warnings != 1 {
		t.Errorf("summary = %+v, want 0 failures / 1 warning", doc.Summary)
	}
	if report.ExitCode(vs, true) != 1 {
		t.Error("--strict did not promote the warning at the exit-code stage")
	}
}

func TestJSONShape(t *testing.T) {
	vs := []check.Violation{{
		Code: check.CodeOrphan, Severity: config.SeverityFail, Node: "platform.svc_billing",
		Path:   "app/services/billing/**",
		Source: directive.Position{File: "docs/architecture/system.d2", Line: 4},
		Detail: "@bind app/services/billing/** matches 0 files",
		Hint:   "renamed?",
	}}
	out := render(t, vs, report.FormatJSON, report.Options{})

	doc := decode(t, out)
	if report.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d; version 1 ships from day one", report.SchemaVersion)
	}
	if doc.Version != report.SchemaVersion {
		t.Errorf("version = %d, want %d", doc.Version, report.SchemaVersion)
	}
	if doc.Violations[0].Node == nil || *doc.Violations[0].Node != "platform.svc_billing" {
		t.Error("node must be the fully-qualified ID")
	}

	// A violation about a node carries no path, and the field is null rather
	// than "" — an empty string reads as "the path that is named nothing".
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	first := raw["violations"].([]any)[0].(map[string]any)
	for _, key := range []string{"code", "severity", "node", "path", "source", "detail", "hint"} {
		if _, ok := first[key]; !ok {
			t.Errorf("violation is missing key %q", key)
		}
	}
}

// A clean run and a failing run must have the same JSON shape; `violations`
// is an empty array, never null.
func TestJSONCleanRunHasEmptyArray(t *testing.T) {
	out := render(t, nil, report.FormatJSON, report.Options{})
	if !strings.Contains(out, `"violations": []`) {
		t.Errorf("clean run should emit an empty array:\n%s", out)
	}
}

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"human", "json"} {
		if _, err := report.ParseFormat(s); err != nil {
			t.Errorf("ParseFormat(%q): %v", s, err)
		}
	}
	if _, err := report.ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat accepted an unknown format")
	}
}

// --- color -------------------------------------------------------------

func TestColorOffProducesNoEscapes(t *testing.T) {
	vs := []check.Violation{
		{Code: check.CodeOrphan, Severity: config.SeverityFail, Node: "a",
			Source: directive.Position{File: "a.d2", Line: 1}, Detail: "d", Hint: "h"},
		{Code: check.CodeUnbound, Severity: config.SeverityWarn, Node: "b",
			Source: directive.Position{File: "a.d2", Line: 2}, Detail: "d", Hint: "h"},
	}
	got := render(t, vs, report.FormatHuman, report.Options{Color: false})
	if strings.Contains(got, "\x1b") {
		t.Errorf("plain output contains ANSI escapes:\n%q", got)
	}
}

func TestColorOnColorizesBySeverity(t *testing.T) {
	vs := []check.Violation{
		{Code: check.CodeOrphan, Severity: config.SeverityFail, Node: "a",
			Source: directive.Position{File: "a.d2", Line: 1}, Detail: "d", Hint: "h"},
		{Code: check.CodeUnbound, Severity: config.SeverityWarn, Node: "b",
			Source: directive.Position{File: "a.d2", Line: 2}, Detail: "d", Hint: "h"},
	}
	got := render(t, vs, report.FormatHuman, report.Options{Color: true})
	if !strings.Contains(got, "\x1b[31mORPHAN\x1b[0m") {
		t.Errorf("failure not red:\n%q", got)
	}
	if !strings.Contains(got, "\x1b[33mUNBOUND\x1b[0m") {
		t.Errorf("warning not yellow:\n%q", got)
	}
	// Padding must sit outside the escape sequence or the column widths are
	// measured in bytes nobody can see.
	if !strings.Contains(got, "\x1b[31mORPHAN\x1b[0m    a") {
		t.Errorf("code column padding is inside the color codes:\n%q", got)
	}
}

// JSON is never colorized, whatever the terminal is.
func TestJSONIsNeverColorized(t *testing.T) {
	vs := []check.Violation{{Code: check.CodeOrphan, Severity: config.SeverityFail, Node: "a",
		Source: directive.Position{File: "a.d2", Line: 1}, Detail: "d", Hint: "h"}}
	if got := render(t, vs, report.FormatJSON, report.Options{Color: true}); strings.Contains(got, "\x1b") {
		t.Errorf("json output contains ANSI escapes:\n%q", got)
	}
}

// UseColor answers on evidence. A buffer is not a terminal, a pipe is not a
// terminal, and a regular file is not a terminal — all three are how output
// reaches a CI log.
func TestUseColorRequiresATerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	if report.UseColor(&bytes.Buffer{}) {
		t.Error("a bytes.Buffer is not a terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if report.UseColor(w) {
		t.Error("a pipe is not a terminal")
	}

	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if report.UseColor(f) {
		t.Error("a regular file is not a terminal")
	}
}

func TestUseColorHonorsConventions(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	if report.UseColor(os.Stdout) {
		t.Error("NO_COLOR was ignored")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if report.UseColor(os.Stdout) {
		t.Error("TERM=dumb was ignored")
	}
}

// --- helpers -----------------------------------------------------------

type doc struct {
	Version int  `json:"version"`
	Strict  bool `json:"strict"`
	Summary struct {
		Failures int `json:"failures"`
		Warnings int `json:"warnings"`
	} `json:"summary"`
	Violations []struct {
		Code     string  `json:"code"`
		Severity string  `json:"severity"`
		Node     *string `json:"node"`
		Path     *string `json:"path"`
		Source   struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"source"`
		Detail string `json:"detail"`
		Hint   string `json:"hint"`
	} `json:"violations"`
}

func decode(t *testing.T, s string) doc {
	t.Helper()
	var d doc
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("decode json: %v\n%s", err, s)
	}
	return d
}
