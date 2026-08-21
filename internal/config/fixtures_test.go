package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timimsms/trestle/internal/config"
)

// Every fixture config must load. This is the cross-package check that config
// validation and the Phase 1 fixtures agree: if it fails, either a fixture is
// invalid or validation is stricter than the design says it should be, and
// both are findings rather than something to loosen quietly.
func TestFixtureConfigsLoad(t *testing.T) {
	repos := filepath.Join("..", "..", "testdata", "repos")
	entries, err := os.ReadDir(repos)
	if err != nil {
		t.Skipf("fixtures not present: %v", err)
	}
	seen := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(repos, e.Name(), config.Filename)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(cfg.Diagrams) == 0 {
				t.Error("no diagrams configured")
			}
			if len(cfg.Severity) != len(config.Codes) {
				t.Errorf("severity map has %d entries, want %d", len(cfg.Severity), len(config.Codes))
			}
			if filepath.Base(cfg.Root) != e.Name() {
				t.Errorf("root = %q, want the fixture directory", cfg.Root)
			}
		})
	}
	if seen == 0 {
		t.Skip("no fixture configs present")
	}
}

// Discovery must find the fixture's own config when started from a nested
// directory inside it, not something further up the real repo.
func TestFindFromFixtureSubdirectory(t *testing.T) {
	start := filepath.Join("..", "..", "testdata", "repos", "clean", "app", "services")
	if _, err := os.Stat(start); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	cfg, err := config.LoadFrom(start)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if filepath.Base(cfg.Root) != "clean" {
		t.Errorf("root = %q, want the clean fixture root", cfg.Root)
	}
}
