package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/expected"
)

// The fixtures are the contract. They were written in Phase 1, before the
// engine and long before this command, so what is being tested here is whether
// `trestle check` — the whole binary, config discovery through exit code —
// agrees with what someone wrote down before any of it existed.

// TestFixturesMatchTheirExpectedFile is the acceptance criterion: run inside
// each fixture directory, `trestle check` must produce that fixture's violation
// set and its exit code.
func TestFixturesMatchTheirExpectedFile(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			want, err := expected.Load(dir)
			if err != nil {
				t.Fatalf("load EXPECTED: %v", err)
			}

			out, errOut, code := runCLI(t, dir, "check", "--format=json")
			if errOut != "" {
				t.Fatalf("unexpected stderr: %s", errOut)
			}
			if code != want.Exit {
				t.Errorf("exit = %d, want %d\n%s", code, want.Exit, out)
			}

			doc := decodeDoc(t, out)
			missing, extra := want.DiffCodes(doc.expectedViolations())
			for _, v := range missing {
				t.Errorf("missing violation: %s %s", v.Code, v.Target)
			}
			for _, v := range extra {
				t.Errorf("unexpected violation: %s %s", v.Code, v.Target)
			}
			if doc.Summary.Warnings != want.Warnings {
				t.Errorf("warnings = %d, want %d", doc.Summary.Warnings, want.Warnings)
			}
			if doc.Strict {
				t.Error("strict must be false without the flag")
			}
		})
	}
}

// --strict changes the exit code and nothing else. The severity field is what a
// JSON consumer uses to tell a warning from a failure, so promoting warnings by
// rewriting it would make the two indistinguishable in exactly the runs where
// the distinction matters.
func TestStrictPromotesWarningsWithoutRewritingSeverity(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			want, err := expected.Load(dir)
			if err != nil {
				t.Fatalf("load EXPECTED: %v", err)
			}
			if !want.HasStrictExit {
				t.Skip("fixture declares no strict_exit")
			}

			plain, _, _ := runCLI(t, dir, "check", "--format=json")
			strictOut, _, code := runCLI(t, dir, "check", "--format=json", "--strict")

			if code != want.StrictExit {
				t.Errorf("strict exit = %d, want %d", code, want.StrictExit)
			}

			before, after := decodeDoc(t, plain), decodeDoc(t, strictOut)
			if !after.Strict {
				t.Error(`--strict must report itself as "strict": true`)
			}
			if len(before.Violations) != len(after.Violations) {
				t.Fatalf("--strict changed the violation count: %d -> %d",
					len(before.Violations), len(after.Violations))
			}
			for i := range before.Violations {
				if before.Violations[i].Severity != after.Violations[i].Severity {
					t.Errorf("--strict rewrote severity of %s %s: %q -> %q",
						after.Violations[i].Code, after.Violations[i].target(),
						before.Violations[i].Severity, after.Violations[i].Severity)
				}
			}
			if before.Summary != after.Summary {
				t.Errorf("--strict changed the summary: %+v -> %+v", before.Summary, after.Summary)
			}
		})
	}
}

// The two formats must describe the same run. A JSON consumer and a human
// reading the same CI job should never be looking at different violation sets.
func TestJSONRoundTripsToTheHumanViolationSet(t *testing.T) {
	for _, name := range append(fixtureNames(t), "") {
		dir := fixtureDir(name)
		label := name
		if name == "" {
			dir = filepath.Join(origWD, "..", "..", "examples", "repairs-platform")
			label = "examples/repairs-platform"
		}
		t.Run(label, func(t *testing.T) {
			humanOut, _, humanCode := runCLI(t, dir, "check")
			jsonOut, _, jsonCode := runCLI(t, dir, "check", "--format=json")

			if humanCode != jsonCode {
				t.Errorf("exit differs by format: human %d, json %d", humanCode, jsonCode)
			}

			fromHuman := parseHumanViolations(t, humanOut)
			fromJSON := decodeDoc(t, jsonOut).targets()
			if !equalSets(fromHuman, fromJSON) {
				t.Errorf("formats disagree\nhuman: %v\njson:  %v\n--- human output ---\n%s",
					fromHuman, fromJSON, humanOut)
			}
		})
	}
}

// Golden files for human output on every fixture. The hints are part of the
// golden file, which is the only form of "every violation carries a runnable
// hint" that survives someone adding a sixth violation shape in a hurry.
func TestHumanOutputMatchesGolden(t *testing.T) {
	cases := append(fixtureNames(t), "examples/repairs-platform")
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			if strings.HasPrefix(name, "examples/") {
				dir = filepath.Join(origWD, "..", "..", name)
			}
			out, errOut, _ := runCLI(t, dir, "check")
			if errOut != "" {
				t.Fatalf("unexpected stderr: %s", errOut)
			}
			compareGolden(t, strings.ReplaceAll(name, "/", "_")+".human.txt", out)
		})
	}
}

// JSON goldens for a representative few, pinning the schema itself: the version
// field, null vs "" for an absent node or path, the empty array on a clean run,
// and the ordering.
func TestJSONOutputMatchesGolden(t *testing.T) {
	for _, name := range []string{"clean", "unbound", "syntax", "unmapped"} {
		t.Run(name, func(t *testing.T) {
			out, _, _ := runCLI(t, fixtureDir(name), "check", "--format=json")
			compareGolden(t, name+".json", out)
		})
	}
}

// Every violation the fixtures can produce carries a hint, and it is visible in
// the output rather than merely present in the struct.
func TestEveryPrintedViolationCarriesAHint(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			out, _, _ := runCLI(t, fixtureDir(name), "check", "--format=json")
			doc := decodeDoc(t, out)
			for _, v := range doc.Violations {
				if strings.TrimSpace(v.Hint) == "" {
					t.Errorf("%s %s has no hint", v.Code, v.target())
				}
				if v.Source.File == "" {
					t.Errorf("%s %s has no source file", v.Code, v.target())
				}
				if filepath.IsAbs(v.Source.File) {
					t.Errorf("%s %s reports an absolute path %q; golden files would not be portable",
						v.Code, v.target(), v.Source.File)
				}
			}
		})
	}
}

// The summary prints even when there is nothing to report. Silence on success
// reads as "did not run", and the first thing anyone does with a check that
// looks like it did not run is stop trusting it.
func TestCleanRunStillPrintsASummary(t *testing.T) {
	out, _, code := runCLI(t, fixtureDir("clean"), "check")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if out != "0 failures, 0 warnings\n" {
		t.Errorf("clean run printed %q", out)
	}
}

// --- exit 2 ------------------------------------------------------------

// Exit 2 means Trestle could not do its job. It is deliberately not exit 1:
// "your diagram is wrong" and "the tool is broken" need different reactions,
// and a CI job that cannot tell them apart teaches people to ignore both.
func TestToolErrorsExitTwo(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		args  []string
		want  string // a substring stderr must contain
	}{
		{
			name:  "missing config",
			files: map[string]string{"app/a/thing.go": "package a\n"},
			want:  "trestle init",
		},
		{
			name: "malformed config: not YAML",
			files: map[string]string{
				".trestle.yml": "version: 1\ndiagrams: [docs/*.d2\n",
			},
			want: "invalid",
		},
		{
			name: "malformed config: unknown key",
			files: map[string]string{
				".trestle.yml": "version: 1\ndiagrams: [docs/*.d2]\ndiscovery:\n  - app/*/\n",
			},
			want: "unknown top-level key",
		},
		{
			name: "malformed config: bad severity",
			files: map[string]string{
				".trestle.yml": "version: 1\ndiagrams: [docs/*.d2]\nseverity:\n  ORPHAN: loud\n",
			},
			want: "severity.ORPHAN",
		},
		{
			name: "zero diagrams matched",
			files: map[string]string{
				".trestle.yml":   "version: 1\ndiagrams: [docs/architecture/*.d2]\n",
				"app/a/thing.go": "package a\n",
			},
			want: "nothing to check",
		},
		{
			name: "unparseable d2",
			files: map[string]string{
				".trestle.yml":   "version: 1\ndiagrams: [docs/*.d2]\n",
				"docs/system.d2": "svc_a: A {\n",
			},
			want: "docs/system.d2",
		},
		{
			name: "unknown format",
			files: map[string]string{
				".trestle.yml":   "version: 1\ndiagrams: [docs/*.d2]\n",
				"docs/system.d2": "svc_a: A\n",
			},
			args: []string{"check", "--format=yaml"},
			want: "unknown format",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeTree(t, c.files)
			args := c.args
			if args == nil {
				args = []string{"check"}
			}
			out, errOut, code := runCLI(t, dir, args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2\nstdout: %s\nstderr: %s", code, out, errOut)
			}
			if out != "" {
				t.Errorf("a tool error wrote to stdout: %q", out)
			}
			if !strings.Contains(errOut, c.want) {
				t.Errorf("stderr does not mention %q:\n%s", c.want, errOut)
			}
		})
	}
}

// A missing config is the one tool error a first-time user will hit, so it says
// what to type.
func TestMissingConfigNamesTheFix(t *testing.T) {
	_, errOut, code := runCLI(t, writeTree(t, map[string]string{"a.go": "package a\n"}), "check")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, ".trestle.yml") || !strings.Contains(errOut, "trestle init") {
		t.Errorf("stderr does not name the file and the fix:\n%s", errOut)
	}
}

// --- color -------------------------------------------------------------

// Color is off whenever stdout is not a terminal. Every writer in these tests
// is a buffer, so this asserts what a CI log receives.
func TestNoANSIEscapesWhenStdoutIsNotATerminal(t *testing.T) {
	for _, name := range fixtureNames(t) {
		out, _, _ := runCLI(t, fixtureDir(name), "check")
		if strings.Contains(out, "\x1b") {
			t.Errorf("%s: output contains ANSI escapes:\n%q", name, out)
		}
	}
}

// The same assertion made against a real process writing down a real pipe,
// because "is stdout a terminal" is a question about a file descriptor and a
// bytes.Buffer cannot fully answer it.
func TestSubprocessExitCodesAndPlainOutput(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		dir  string
		args []string
		want int
	}{
		{"clean", fixtureDir("clean"), []string{"check"}, 0},
		{"orphan", fixtureDir("orphan"), []string{"check"}, 1},
		{"unbound", fixtureDir("unbound"), []string{"check"}, 0},
		{"unbound --strict", fixtureDir("unbound"), []string{"check", "--strict"}, 1},
		{"missing config", t.TempDir(), []string{"check"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, code := runSubprocess(t, bin, c.dir, c.args...)
			if code != c.want {
				t.Errorf("exit = %d, want %d\n%s", code, c.want, out)
			}
			if strings.Contains(out, "\x1b") {
				t.Errorf("output down a pipe contains ANSI escapes:\n%q", out)
			}
		})
	}
}

// --- the worked example ------------------------------------------------

// The example is a live test input. Its `discover:` rules and `shared:` entries
// now point at a real tree, so the one thing it reports is the thing O9 says it
// should: `tenant` is a leaf with no directive, and `platform` — whose every
// descendant is accounted for — says nothing at all.
func TestWorkedExampleChecksAsDocumented(t *testing.T) {
	dir := filepath.Join(origWD, "..", "..", "examples", "repairs-platform")

	out, errOut, code := runCLI(t, dir, "check", "--format=json")
	if errOut != "" {
		t.Fatalf("unexpected stderr: %s", errOut)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0\n%s", code, out)
	}

	doc := decodeDoc(t, out)
	if doc.Summary.Failures != 0 {
		t.Errorf("failures = %d, want 0", doc.Summary.Failures)
	}
	if len(doc.Violations) != 1 {
		t.Fatalf("violations = %d, want exactly one (UNBOUND tenant): %+v",
			len(doc.Violations), doc.Violations)
	}
	v := doc.Violations[0]
	if v.Code != string(check.CodeUnbound) || v.target() != "tenant" {
		t.Errorf("got %s %s, want UNBOUND tenant", v.Code, v.target())
	}

	if _, _, strictCode := runCLI(t, dir, "check", "--strict"); strictCode != 1 {
		t.Errorf("strict exit = %d, want 1", strictCode)
	}
}

// The example ships two copies of its diagram: `system.d2` is the canonical
// Gate B / Phase 2 test input named by four other packages, and
// `docs/architecture/system.d2` is where the example's own `.trestle.yml` looks
// for it. Neither could move without editing a Phase 1-3 test, so they are kept
// identical by this assertion instead of by hope.
func TestWorkedExampleDiagramCopiesAreIdentical(t *testing.T) {
	base := filepath.Join(origWD, "..", "..", "examples", "repairs-platform")
	canonical, err := os.ReadFile(filepath.Join(base, "system.d2"))
	if err != nil {
		t.Fatal(err)
	}
	checked, err := os.ReadFile(filepath.Join(base, "docs", "architecture", "system.d2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != string(checked) {
		t.Error("examples/repairs-platform/system.d2 and docs/architecture/system.d2 have drifted; " +
			"copy the canonical file over the checked one")
	}
}

// --- helpers -----------------------------------------------------------

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(origWD, "testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // golden files are readable by design
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (regenerate with `go test ./cmd/trestle -update`): %v", err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// jsonDoc mirrors the `--format=json` payload. It is declared here rather than
// exported from internal/report on purpose: this test consumes the JSON the way
// a third party would, from the bytes, so a change to the schema shows up as a
// test failure instead of being carried along by a shared struct.
type jsonDoc struct {
	Version int  `json:"version"`
	Strict  bool `json:"strict"`
	Summary struct {
		Failures int `json:"failures"`
		Warnings int `json:"warnings"`
	} `json:"summary"`
	Violations []jsonViolation `json:"violations"`
}

type jsonViolation struct {
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
}

// target is what the human format prints in the target column: the node when
// there is one, otherwise the path.
func (v jsonViolation) target() string {
	switch {
	case v.Node != nil:
		return *v.Node
	case v.Path != nil:
		return *v.Path
	default:
		return v.Source.File
	}
}

func (d jsonDoc) targets() []string {
	out := make([]string, 0, len(d.Violations))
	for _, v := range d.Violations {
		out = append(out, v.Code+" "+v.target())
	}
	return out
}

func (d jsonDoc) expectedViolations() []expected.Violation {
	out := make([]expected.Violation, 0, len(d.Violations))
	for _, v := range d.Violations {
		out = append(out, expected.Violation{Code: v.Code, Target: v.target(), Detail: v.Detail})
	}
	return out
}

func decodeDoc(t *testing.T, s string) jsonDoc {
	t.Helper()
	var d jsonDoc
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("decode json: %v\n%s", err, s)
	}
	if d.Version != 1 {
		t.Errorf(`version = %d, want 1`, d.Version)
	}
	return d
}

// humanHeadline matches the first line of a violation block: two spaces, the
// code, padding, then the target. Detail and hint lines are indented twelve
// and never match.
var humanHeadline = regexp.MustCompile(`^ {2}([A-Z]+) +(.+?)(?:  \(warn\))?$`)

func parseHumanViolations(t *testing.T, out string) []string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(out, "\n") {
		m := humanHeadline.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !knownCode(m[1]) {
			t.Errorf("human output used an unknown violation code %q", m[1])
			continue
		}
		found = append(found, m[1]+" "+m[2])
	}
	return found
}

func knownCode(s string) bool {
	for _, c := range check.Codes {
		if string(c) == s {
			return true
		}
	}
	return false
}

func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		counts[s]--
		if counts[s] < 0 {
			return false
		}
	}
	return true
}
