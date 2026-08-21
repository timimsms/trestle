package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderRepo builds a minimal repo with a config and a diagram, and chdirs into
// it for the duration of the test.
func renderRepo(t *testing.T, cfg, diagram string) string {
	t.Helper()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, ".trestle.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "docs", "architecture")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "system.d2"), []byte(diagram), 0o644); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	// macOS resolves TempDir through /var -> /private/var, and config discovery
	// reports the resolved path. Hand back what the test should compare against.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return root
	}
	return resolved
}

const okConfig = `version: 1
diagrams:
  - docs/architecture/*.d2
render:
  out: docs/architecture/rendered/
  layout: elk
`

const okDiagram = "a: Alpha\nb: Beta\na -> b: talks to\n"

func TestRenderWritesSVG(t *testing.T) {
	root := renderRepo(t, okConfig, okDiagram)

	var stdout, stderr bytes.Buffer
	if code := Main([]string{"render"}, &stdout, &stderr); code != exitClean {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitClean, stderr.String())
	}

	out := filepath.Join(root, "docs", "architecture", "rendered", "system.svg")
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no SVG written: %v", err)
	}
	if !bytes.Contains(b, []byte("<svg")) {
		t.Error("output is not an SVG")
	}
	if got := stdout.String(); !strings.Contains(got, "1 diagram rendered") {
		t.Errorf("summary missing:\n%s", got)
	}
}

// Rendering has no exit 1. It makes no claim about whether the architecture is
// accurate — a diagram that renders beautifully can still be a lie. Everything
// that goes wrong here is a tool error.
func TestRenderExitCodes(t *testing.T) {
	t.Run("unparseable D2 is exit 2", func(t *testing.T) {
		renderRepo(t, okConfig, "a -> {{{")
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"render"}, &stdout, &stderr); code != exitTool {
			t.Errorf("exit = %d, want %d", code, exitTool)
		}
	})

	t.Run("no render.out is exit 2 and says what to add", func(t *testing.T) {
		renderRepo(t, "version: 1\ndiagrams:\n  - docs/architecture/*.d2\n", okDiagram)
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"render"}, &stdout, &stderr); code != exitTool {
			t.Errorf("exit = %d, want %d", code, exitTool)
		}
		if msg := stderr.String(); !strings.Contains(msg, "render.out") {
			t.Errorf("error should name the missing key, got: %s", msg)
		}
	})

	t.Run("unknown layout is exit 2", func(t *testing.T) {
		renderRepo(t, strings.Replace(okConfig, "layout: elk", "layout: graphviz", 1), okDiagram)
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"render"}, &stdout, &stderr); code != exitTool {
			t.Errorf("exit = %d, want %d", code, exitTool)
		}
	})

	t.Run("missing config is exit 2", func(t *testing.T) {
		wd, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(wd) })
		if err := os.Chdir(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := Main([]string{"render"}, &stdout, &stderr); code != exitTool {
			t.Errorf("exit = %d, want %d", code, exitTool)
		}
	})
}

func TestRenderQuiet(t *testing.T) {
	renderRepo(t, okConfig, okDiagram)

	var stdout, stderr bytes.Buffer
	if code := Main([]string{"render", "--quiet"}, &stdout, &stderr); code != exitClean {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr.String())
	}
	if got := stdout.String(); got != "" {
		t.Errorf("--quiet printed:\n%s", got)
	}
}

// render must not be able to report violations. Keeping the two commands'
// concerns apart is what lets `check` stay a lint-speed operation.
func TestRenderSaysNothingAboutViolations(t *testing.T) {
	// A diagram whose node has no binding at all — `check --strict` would warn.
	renderRepo(t, okConfig, okDiagram)

	var stdout, stderr bytes.Buffer
	if code := Main([]string{"render"}, &stdout, &stderr); code != exitClean {
		t.Fatalf("exit = %d, want %d", code, exitClean)
	}
	for _, word := range []string{"UNBOUND", "ORPHAN", "violation", "failures"} {
		if strings.Contains(stdout.String(), word) {
			t.Errorf("render output mentions %q; that is check's job", word)
		}
	}
}
