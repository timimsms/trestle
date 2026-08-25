package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/config"
)

// write puts a config file at dir/.trestle.yml and returns its path.
func write(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, config.Filename)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const minimal = "version: 1\ndiagrams: [docs/architecture/*.d2]\n"

// Acceptance: the worked example round-trips.
func TestWorkedExample(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "repairs-platform", config.Filename)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load worked example: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	wantSlices := map[string][]string{
		"diagrams": {"docs/architecture/*.d2"},
		"discover": {"app/services/*/", "app/jobs/*/"},
		"shared":   {"lib/http_client/**", "lib/logging/**", "app/middleware/**"},
		"exclude":  {"**/*_spec.rb", "**/vendor/**"},
	}
	got := map[string][]string{
		"diagrams": cfg.Diagrams,
		"discover": cfg.Discover,
		"shared":   cfg.Shared,
		"exclude":  cfg.Exclude,
	}
	for name, want := range wantSlices {
		if !equal(got[name], want) {
			t.Errorf("%s = %v, want %v", name, got[name], want)
		}
	}

	// Trailing slashes in `discover:` are load-bearing — they name directories,
	// while @bind globs name files. Config must not normalize them away.
	for _, d := range cfg.Discover {
		if !strings.HasSuffix(d, "/") {
			t.Errorf("discover entry %q lost its trailing slash", d)
		}
	}

	if cfg.SeverityFor(config.CodeUnbound) != config.SeverityWarn {
		t.Errorf("UNBOUND = %q, want warn", cfg.SeverityFor(config.CodeUnbound))
	}
	for _, code := range []string{config.CodeOrphan, config.CodeUnmapped, config.CodeDangling, config.CodeSyntax} {
		if cfg.SeverityFor(code) != config.SeverityFail {
			t.Errorf("%s = %q, want fail", code, cfg.SeverityFor(code))
		}
	}

	if cfg.Render.Out != "docs/architecture/rendered/" || cfg.Render.Layout != "elk" {
		t.Errorf("render = %+v", cfg.Render)
	}
	if cfg.Render.Theme != 0 {
		t.Errorf("render.theme = %d, want 0", cfg.Render.Theme)
	}

	wantRoot, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != wantRoot {
		t.Errorf("root = %q, want %q", cfg.Root, wantRoot)
	}
	if !filepath.IsAbs(cfg.Path) || filepath.Base(cfg.Path) != config.Filename {
		t.Errorf("path = %q", cfg.Path)
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := config.Parse("/repo/.trestle.yml", []byte(minimal))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Absent severity: UNBOUND warns, everything else fails.
	want := map[string]config.Severity{
		config.CodeOrphan:   config.SeverityFail,
		config.CodeUnmapped: config.SeverityFail,
		config.CodeDangling: config.SeverityFail,
		config.CodeUnbound:  config.SeverityWarn,
		config.CodeSyntax:   config.SeverityFail,
	}
	if len(cfg.Severity) != len(want) {
		t.Errorf("severity map has %d entries, want %d", len(cfg.Severity), len(want))
	}
	for code, sev := range want {
		if cfg.SeverityFor(code) != sev {
			t.Errorf("%s = %q, want %q", code, cfg.SeverityFor(code), sev)
		}
	}
	// Absent exclude: .git, node_modules, vendor.
	if !equal(cfg.Exclude, config.DefaultExclude()) {
		t.Errorf("exclude = %v, want %v", cfg.Exclude, config.DefaultExclude())
	}
	// Absent discover is legal and means UNMAPPED never fires.
	if cfg.Discover != nil {
		t.Errorf("discover = %v, want nil", cfg.Discover)
	}
	if cfg.Shared != nil {
		t.Errorf("shared = %v, want nil", cfg.Shared)
	}
	// Render is not defaulted here; Phase 6 owns render behavior.
	if cfg.Render != (config.Render{}) {
		t.Errorf("render = %+v, want zero", cfg.Render)
	}
}

func TestExplicitEmptyExcludeIsHonored(t *testing.T) {
	cfg, err := config.Parse("/repo/.trestle.yml", []byte(minimal+"exclude: []\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Exclude) != 0 {
		t.Errorf("exclude = %v, want empty (an explicit [] is not an absent key)", cfg.Exclude)
	}
}

func TestSeverityOverrides(t *testing.T) {
	src := minimal + "severity:\n  UNBOUND: fail\n  ORPHAN: warn\n  UNMAPPED: off\n"
	cfg, err := config.Parse("/repo/.trestle.yml", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for code, want := range map[string]config.Severity{
		config.CodeUnbound:  config.SeverityFail,
		config.CodeOrphan:   config.SeverityWarn,
		config.CodeUnmapped: config.SeverityOff,
		config.CodeDangling: config.SeverityFail, // untouched default
	} {
		if got := cfg.SeverityFor(code); got != want {
			t.Errorf("%s = %q, want %q", code, got, want)
		}
	}
}

// `off` is a YAML 1.1 boolean. goccy is YAML 1.2, so it stays a string — but
// this is exactly the kind of thing that breaks on a library bump.
func TestSeverityOffIsNotABoolean(t *testing.T) {
	cfg, err := config.Parse("/repo/.trestle.yml", []byte(minimal+"severity: {SYNTAX: off}\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.SeverityFor(config.CodeSyntax) != config.SeverityOff {
		t.Errorf("SYNTAX = %q, want off", cfg.SeverityFor(config.CodeSyntax))
	}
}

// The four invalid-config classes from docs/DESIGN.md §2, plus the near-misses
// that share their code paths. Every one must name the offending key and, where
// the key exists in the file, its line.
func TestInvalidConfigs(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKey  string
		wantLine int // 0 = do not assert
		wantMsg  string
		wantHint string
	}{
		{
			name:     "unknown top-level key",
			src:      minimal + "shred:\n  - lib/foo/**\n",
			wantKey:  "shred",
			wantLine: 3,
			wantMsg:  "unknown top-level key",
			wantHint: "diagrams",
		},
		{
			name:     "unknown severity code",
			src:      minimal + "severity:\n  UNBOUNDED: warn\n",
			wantKey:  "severity.UNBOUNDED",
			wantLine: 4,
			wantMsg:  "unknown violation code",
			wantHint: "ORPHAN, UNMAPPED, DANGLING, UNBOUND, SYNTAX",
		},
		{
			name:     "bad severity value",
			src:      minimal + "severity:\n  UNBOUND: whisper\n",
			wantKey:  "severity.UNBOUND",
			wantLine: 4,
			wantMsg:  "invalid severity whisper",
			wantHint: "fail, warn, off",
		},
		{
			name:     "non-string severity value",
			src:      minimal + "severity:\n  UNBOUND: 3\n",
			wantKey:  "severity.UNBOUND",
			wantLine: 4,
			wantMsg:  "invalid severity 3",
			wantHint: "fail, warn, off",
		},
		{
			name:     "blanket shared entry",
			src:      minimal + "shared:\n  - lib/http_client/**\n  - lib/**\n",
			wantKey:  "shared[1]",
			wantLine: 5,
			wantMsg:  `blanket entry "lib/**"`,
			wantHint: "enumerate",
		},
		{
			name:     "version other than 1",
			src:      "version: 2\ndiagrams: [a.d2]\n",
			wantKey:  "version",
			wantLine: 1,
			wantMsg:  "unsupported version 2",
			wantHint: "version: 1",
		},
		{
			name:    "missing version",
			src:     "diagrams: [a.d2]\n",
			wantKey: "version",
			wantMsg: "missing",
		},
		{
			name:     "unknown render key",
			src:      minimal + "render:\n  outt: docs/rendered/\n",
			wantKey:  "render.outt",
			wantLine: 4,
			wantMsg:  "unknown key",
			wantHint: "out, layout, theme",
		},
		{
			name:    "no diagrams",
			src:     "version: 1\n",
			wantKey: "diagrams",
			wantMsg: "nothing to check",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Parse("/repo/.trestle.yml", []byte(tc.src))
			if err == nil {
				t.Fatalf("expected an error, got config %v", cfg)
			}
			if cfg != nil {
				t.Errorf("expected a nil config alongside the error")
			}
			list, ok := err.(config.ErrorList)
			if !ok {
				t.Fatalf("error is %T, want config.ErrorList: %v", err, err)
			}
			var match *config.Error
			for _, e := range list {
				if e.Key == tc.wantKey {
					match = e
				}
			}
			if match == nil {
				t.Fatalf("no error for key %q; got: %v", tc.wantKey, err)
			}
			if !strings.Contains(match.Msg, tc.wantMsg) {
				t.Errorf("msg = %q, want it to contain %q", match.Msg, tc.wantMsg)
			}
			if tc.wantHint != "" && !strings.Contains(match.Hint, tc.wantHint) {
				t.Errorf("hint = %q, want it to contain %q", match.Hint, tc.wantHint)
			}
			if tc.wantLine != 0 && match.Line != tc.wantLine {
				t.Errorf("line = %d, want %d", match.Line, tc.wantLine)
			}
			// The rendered message must locate the problem in the file.
			if !strings.Contains(err.Error(), "/repo/.trestle.yml") {
				t.Errorf("Error() = %q, want it to name the file", err.Error())
			}
		})
	}
}

func TestBlanketSharedDetection(t *testing.T) {
	blanket := []string{
		"lib/**",
		"lib/*",
		"lib/**/",
		"**",
		"*",
		"*/**",
		"**/*",
		"app/**",
		"  lib/**  ",
	}
	enumerated := []string{
		"lib/http_client/**",
		"lib/logging/**",
		"app/middleware/**",
		"app/*/middleware/**", // still names a specific leaf
		"lib/{http_client,logging}/**",
		"lib/pricing_engine",
		"lib/dispatch*/**",
	}

	for _, p := range blanket {
		t.Run("blanket "+p, func(t *testing.T) {
			src := minimal + "shared:\n  - " + strings.TrimSpace(p) + "\n"
			if _, err := config.Parse("/repo/.trestle.yml", []byte(src)); err == nil {
				t.Errorf("shared entry %q accepted; L11 says blanket entries are an error", p)
			}
		})
	}
	for _, p := range enumerated {
		t.Run("enumerated "+p, func(t *testing.T) {
			src := minimal + "shared:\n  - " + p + "\n"
			if _, err := config.Parse("/repo/.trestle.yml", []byte(src)); err != nil {
				t.Errorf("shared entry %q rejected: %v", p, err)
			}
		})
	}
}

// `exclude:` is a blindspot by design (DESIGN §4) and is deliberately allowed
// to be blanket. Only `shared:` is an accountable exemption.
func TestExcludeMayBeBlanket(t *testing.T) {
	src := minimal + "exclude:\n  - \"**\"\n  - lib/**\n"
	if _, err := config.Parse("/repo/.trestle.yml", []byte(src)); err != nil {
		t.Errorf("blanket exclude rejected: %v", err)
	}
}

func TestMultipleErrorsAreAllReported(t *testing.T) {
	src := "version: 3\ndiagrams: [a.d2]\nshred: 1\nshared:\n  - lib/**\nseverity:\n  NOPE: warn\n"
	_, err := config.Parse("/repo/.trestle.yml", []byte(src))
	list, ok := err.(config.ErrorList)
	if !ok {
		t.Fatalf("error is %T, want ErrorList: %v", err, err)
	}
	if len(list) != 4 {
		t.Fatalf("got %d errors, want 4:\n%v", len(list), err)
	}
	// Reported in file order so the user can fix top-down.
	for i := 1; i < len(list); i++ {
		if list[i-1].Line > list[i].Line {
			t.Errorf("errors out of file order: %v", err)
		}
	}
}

// A duplicated key silently dropping half a `shared:` list is exactly the kind
// of quiet exemption L11 exists to prevent.
func TestDuplicateKeyIsRejected(t *testing.T) {
	src := "version: 1\ndiagrams: [a.d2]\nshared:\n  - lib/http_client/**\nshared:\n  - lib/logging/**\n"
	_, err := config.Parse("/repo/.trestle.yml", []byte(src))
	if err == nil {
		t.Fatal("expected an error for a duplicated key")
	}
	if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("error should say the key is duplicated: %v", err)
	}
	if !strings.Contains(err.Error(), ".trestle.yml:5") {
		t.Errorf("error should carry the line: %v", err)
	}
}

func TestMalformedYAML(t *testing.T) {
	_, err := config.Parse("/repo/.trestle.yml", []byte("version: 1\ndiagrams: [a.d2\n"))
	if err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "/repo/.trestle.yml") {
		t.Errorf("error does not name the file: %v", err)
	}
	var cerr *config.Error
	if !asConfigError(err, &cerr) {
		t.Fatalf("error is %T, want *config.Error: %v", err, err)
	}
}

func TestWrongTypeIsNotAPanic(t *testing.T) {
	// Every one of these is a type mismatch that a naive decoder would
	// panic on. Config errors are user-facing; a Go panic is not a message.
	for _, src := range []string{
		"version: one\ndiagrams: [a.d2]\n",
		"version: 1\ndiagrams: docs/architecture/*.d2\n",
		"version: 1\ndiagrams: [a.d2]\nshared: lib/foo\n",
		"version: 1\ndiagrams: [a.d2]\nseverity: warn\n",
		"version: 1\ndiagrams: [a.d2]\nrender: []\n",
		"",
		"---\nversion: 1\ndiagrams: [a.d2]\n---\nversion: 1\n",
	} {
		t.Run(strings.ReplaceAll(src, "\n", "|"), func(t *testing.T) {
			cfg, err := config.Parse("/repo/.trestle.yml", []byte(src))
			if err == nil {
				t.Fatalf("expected an error, got %v", cfg)
			}
			if err.Error() == "" {
				t.Error("empty error message")
			}
		})
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	write(t, root, minimal)
	deep := filepath.Join(root, "app", "services", "billing")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}

	got, err := config.Find(deep)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := filepath.EvalSymlinks(filepath.Dir(got))
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != wantRoot {
		t.Errorf("found %q, want it under %q", got, wantRoot)
	}

	cfg, err := config.LoadFrom(deep)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	// The directory containing the config is the root, not the CWD we started
	// the search from.
	if cfg.Root == deep {
		t.Errorf("root = %q, want the directory holding the config", cfg.Root)
	}
}

// The nearest config wins: a nested one shadows its parent.
func TestFindStopsAtNearest(t *testing.T) {
	root := t.TempDir()
	write(t, root, minimal)
	nested := filepath.Join(root, "sub")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, nested, minimal)

	got, err := config.Find(nested)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if filepath.Base(filepath.Dir(got)) != "sub" {
		t.Errorf("found %q, want the config in sub/", got)
	}
}

func TestFindMissing(t *testing.T) {
	// t.TempDir() is somewhere under the system temp dir, which has no
	// .trestle.yml above it.
	_, err := config.Find(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when no config exists")
	}
	if !strings.Contains(err.Error(), "trestle init") {
		t.Errorf("error should hint at `trestle init`: %v", err)
	}
}

func TestFindIgnoresADirectoryNamedLikeTheConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", config.Filename), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Find(filepath.Join(root, "sub")); err == nil {
		t.Error("a directory named .trestle.yml must not be treated as the config")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), config.Filename))
	if err == nil {
		t.Fatal("expected an error")
	}
	var cerr *config.Error
	if !asConfigError(err, &cerr) {
		t.Fatalf("error is %T, want *config.Error", err)
	}
}

func TestSeverityValid(t *testing.T) {
	for _, s := range config.Severities {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if config.Severity("whisper").Valid() {
		t.Error("whisper should not be valid")
	}
}

func TestCodesAreTheFive(t *testing.T) {
	if len(config.Codes) != 5 {
		t.Fatalf("got %d codes, want 5 — the taxonomy is closed", len(config.Codes))
	}
	want := map[string]bool{"ORPHAN": true, "UNMAPPED": true, "DANGLING": true, "UNBOUND": true, "SYNTAX": true}
	for _, c := range config.Codes {
		if !want[c] {
			t.Errorf("unexpected code %q", c)
		}
	}
	if len(config.DefaultSeverity()) != len(config.Codes) {
		t.Error("DefaultSeverity must cover every code")
	}
}

func asConfigError(err error, target **config.Error) bool {
	e, ok := err.(*config.Error)
	if ok {
		*target = e
	}
	return ok
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
