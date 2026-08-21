package directive_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timimsms/trestle/internal/directive"
)

// The `syntax/` fixture is the cross-package contract for SYNTAX reporting:
// Phase 3 asserts the violations, this asserts the scanner sees the same
// malformed lines the fixture claims to contain. The fixture is owned by
// Phase 1; skip rather than fail if it has not landed.
func TestSyntaxFixture(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "repos", "syntax", "docs", "architecture", "system.d2")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	res, err := directive.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(res.Syntax) != 2 {
		t.Fatalf("got %d syntax errors, want 2:\n%v", len(res.Syntax), res.Syntax)
	}
	// A malformed directive must not swallow the well-formed ones around it.
	if got := res.Count(directive.KindBind); got != 3 {
		t.Errorf("got %d binds, want 3 — a malformed line discarded a valid one", got)
	}
	for _, e := range res.Syntax {
		if e.Source.Line == 0 || e.Raw == "" || e.Detail == "" {
			t.Errorf("syntax error is missing position, source line or detail: %+v", e)
		}
	}
}

// Every fixture diagram must scan without the scanner inventing directives.
// Only `syntax/` is allowed to contain malformed ones.
func TestFixtureDiagramsScan(t *testing.T) {
	repos := filepath.Join("..", "..", "testdata", "repos")
	entries, err := os.ReadDir(repos)
	if err != nil {
		t.Skipf("fixtures not present: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		diagrams, err := filepath.Glob(filepath.Join(repos, e.Name(), "docs", "architecture", "*.d2"))
		if err != nil || len(diagrams) == 0 {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			res, err := directive.ParseFiles(diagrams...)
			if err != nil {
				t.Fatalf("ParseFiles: %v", err)
			}
			if e.Name() != "syntax" && len(res.Syntax) != 0 {
				t.Errorf("fixture %s has unexpected syntax errors:\n%v", e.Name(), res.Syntax)
			}
			for _, d := range res.Directives {
				if d.Node == "" || d.Source.Line == 0 {
					t.Errorf("directive without a node or position: %+v", d)
				}
				if d.Kind == directive.KindBind && d.Glob == "" {
					t.Errorf("@bind without a glob survived parsing: %+v", d)
				}
				if d.Kind == directive.KindIgnore && d.Reason == "" {
					t.Errorf("@ignore without a reason survived parsing: %+v", d)
				}
			}
		})
	}
}
