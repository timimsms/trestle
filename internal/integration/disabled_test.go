package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/report"
)

// A code set to `off` in .trestle.yml disappears from the result entirely. That
// is legal (DESIGN §4: severity is overridable per code) but it must never be
// invisible: a repo that switches ORPHAN and UNMAPPED off would otherwise get a
// bare "0 failures, 0 warnings" and exit 0 from a check that inspected nothing,
// which is a green result that means nothing — the precise failure this tool
// exists to prevent, and the same family as a config matching zero diagrams.
//
// These tests pin the qualification into both output formats. If they fail
// because someone removed the note, the tool has started lying about its own
// coverage.
func TestDisabledCodesAreReportedNotHidden(t *testing.T) {
	cfg := &config.Config{
		Severity: map[string]config.Severity{
			string(check.CodeOrphan):   config.SeverityOff,
			string(check.CodeUnmapped): config.SeverityOff,
			string(check.CodeDangling): config.SeverityFail,
			string(check.CodeSyntax):   config.SeverityFail,
			string(check.CodeUnbound):  config.SeverityWarn,
		},
	}

	disabled := check.DisabledCodes(cfg)
	if len(disabled) != 2 {
		t.Fatalf("DisabledCodes = %v, want ORPHAN and UNMAPPED", disabled)
	}

	t.Run("human summary carries the note", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.Write(&buf, nil, report.FormatHuman, report.Options{Disabled: disabled})
		if err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		if !strings.Contains(got, "0 failures, 0 warnings") {
			t.Fatalf("summary line missing:\n%s", got)
		}
		for _, code := range []string{string(check.CodeOrphan), string(check.CodeUnmapped)} {
			if !strings.Contains(got, code) {
				t.Errorf("summary does not name disabled code %s — a clean run "+
					"is indistinguishable from a run that checked nothing:\n%s", code, got)
			}
		}
	})

	t.Run("json exposes the field", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.Write(&buf, nil, report.FormatJSON, report.Options{Disabled: disabled})
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Disabled []string `json:"disabled"`
		}
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Disabled) != 2 {
			t.Errorf("json disabled = %v, want [ORPHAN UNMAPPED]", doc.Disabled)
		}
	})

	t.Run("field is present and empty when nothing is disabled", func(t *testing.T) {
		var buf bytes.Buffer
		err := report.Write(&buf, nil, report.FormatJSON, report.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(buf.Bytes(), []byte(`"disabled"`)) {
			t.Error(`"disabled" must always be present so a consumer can tell ` +
				`"nothing disabled" from an older payload that could not say`)
		}
		var doc struct {
			Disabled []string `json:"disabled"`
		}
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Disabled) != 0 {
			t.Errorf("disabled = %v, want empty", doc.Disabled)
		}
	})

	t.Run("human summary is unqualified when nothing is disabled", func(t *testing.T) {
		var buf bytes.Buffer
		if err := report.Write(&buf, nil, report.FormatHuman, report.Options{}); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(buf.String()); got != "0 failures, 0 warnings" {
			t.Errorf("clean summary = %q, want unqualified", got)
		}
	})
}

// End-to-end: the orphan fixture with ORPHAN switched off must still tell the
// reader that ORPHAN was switched off.
func TestDisabledCodeSurvivesTheRealPipeline(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join("..", "..", "testdata", "repos", "orphan")

	if err := copyTree(src, root); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, config.Filename)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, append(raw, []byte("\nseverity:\n  ORPHAN: off\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	disabled := check.DisabledCodes(cfg)
	if len(disabled) != 1 || disabled[0] != check.CodeOrphan {
		t.Fatalf("DisabledCodes = %v, want [ORPHAN]", disabled)
	}

	var buf bytes.Buffer
	err = report.Write(&buf, nil, report.FormatHuman, report.Options{Root: cfg.Root, Disabled: disabled})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), string(check.CodeOrphan)) {
		t.Errorf("a repo that disabled ORPHAN gets an unqualified clean report:\n%s", buf.String())
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
