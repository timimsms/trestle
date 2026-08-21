package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimal = `
a: Alpha
b: Beta
a -> b: talks to
`

func TestSVGProducesRenderableOutput(t *testing.T) {
	svg, err := SVG(context.Background(), minimal, "t.d2", Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(svg)
	if !strings.Contains(got, "<svg") {
		t.Errorf("output is not an SVG:\n%s", got[:min(200, len(got))])
	}
	for _, want := range []string{"Alpha", "Beta", "talks to"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered SVG is missing %q", want)
		}
	}
}

// Both engines have to work, because .trestle.yml can name either and the
// worked example names elk.
func TestBothLayoutEngines(t *testing.T) {
	for _, layout := range []string{LayoutDagre, LayoutELK} {
		t.Run(layout, func(t *testing.T) {
			svg, err := SVG(context.Background(), minimal, "t.d2", Options{Layout: layout})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(svg), "<svg") {
				t.Error("not an SVG")
			}
		})
	}
}

func TestUnknownLayoutIsAnError(t *testing.T) {
	_, err := SVG(context.Background(), minimal, "t.d2", Options{Layout: "graphviz"})
	if err == nil {
		t.Fatal("want an error for an unknown layout engine")
	}
	if !strings.Contains(err.Error(), "graphviz") {
		t.Errorf("error should name the bad engine, got: %v", err)
	}
}

// With no configured layout, a diagram's own `layout-engine` var decides.
//
// This is also the regression test for a real bug: D2 only calls LayoutResolver
// when CompileOptions.Layout is non-nil, so an earlier version of this package
// never called it at all — `render.layout: elk` was accepted, silently ignored,
// and everything came out dagre. The unknown-engine test above is what caught
// it, because an ignored option cannot produce an error.
func TestDiagramLayoutVarAppliesWhenConfigIsSilent(t *testing.T) {
	src := `
vars: {
  d2-config: {
    layout-engine: elk
  }
}
a -> b
`
	svg, err := SVG(context.Background(), src, "t.d2", Options{})
	if err != nil {
		t.Fatalf("diagram-level layout var was not honored: %v", err)
	}
	if !strings.Contains(string(svg), "<svg") {
		t.Error("not an SVG")
	}
}

// A configured layout reaches D2 rather than being quietly dropped. Asserted
// through the resolver: a bad engine name can only surface as an error if the
// value was actually consulted.
func TestConfiguredLayoutIsActuallyUsed(t *testing.T) {
	src := `
vars: {
  d2-config: {
    layout-engine: elk
  }
}
a -> b
`
	_, err := SVG(context.Background(), src, "t.d2", Options{Layout: "nonsense"})
	if err == nil {
		t.Fatal("configured layout was ignored; `render.layout` would silently do nothing")
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("want the bad engine named, got: %v", err)
	}
}

// A malformed diagram is a tool error, never a violation. "Trestle is broken"
// and "your diagram is wrong" have to stay distinguishable.
func TestBrokenDiagramReturnsError(t *testing.T) {
	_, err := SVG(context.Background(), "a -> {{{", "bad.d2", Options{})
	if err == nil {
		t.Fatal("want an error for unparseable D2")
	}
	var re *Error
	if !asRenderError(err, &re) {
		t.Fatalf("want a *render.Error, got %T", err)
	}
	if re.Path != "bad.d2" {
		t.Errorf("error should carry the path, got %q", re.Path)
	}
}

func TestFileWritesSVGAndCreatesOutputDir(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "docs", "system.d2")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := File(context.Background(), src, Options{Root: root, Out: "docs/rendered"})
	if err != nil {
		t.Fatal(err)
	}

	if res.Out != "docs/rendered/system.svg" {
		t.Errorf("Out = %q, want docs/rendered/system.svg", res.Out)
	}
	if res.Source != "docs/system.d2" {
		t.Errorf("Source = %q, want a repo-relative path", res.Source)
	}
	if res.Bytes == 0 {
		t.Error("Bytes = 0")
	}

	written, err := os.ReadFile(filepath.Join(root, res.Out))
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if len(written) != res.Bytes {
		t.Errorf("Bytes = %d but file is %d", res.Bytes, len(written))
	}
}

// L8's payoff, asserted rather than assumed: rendering must work with no `d2`
// binary anywhere. Embedding D2 as a library is the entire reason this project
// is written in Go, and until this phase nothing exercised it — the compiler
// was only ever used to extract node IDs.
func TestRendersWithNoD2BinaryOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := SVG(context.Background(), minimal, "t.d2", Options{Layout: LayoutELK}); err != nil {
		t.Fatalf("render needs a d2 binary on PATH, which defeats L8: %v", err)
	}
}

func asRenderError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
