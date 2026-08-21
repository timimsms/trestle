package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initRepo builds a temp repo with a layout `init` recognizes: three services,
// one job, two shared libraries and a `node_modules` that must not survive the
// default excludes into a proposed rule.
func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{
		"app/services/billing/billing.rb",
		"app/services/dispatch/dispatcher.rb",
		"app/jobs/reconciler/job.rb",
		"lib/http_client/client.rb",
		"lib/logging/logger.rb",
		"node_modules/left-pad/index.js",
	} {
		abs := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func exists(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestInitWritesTheScaffold(t *testing.T) {
	root := initRepo(t)

	stdout, stderr, code := runCLI(t, root, "init", "--yes")
	if code != exitClean {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitClean, stderr)
	}
	for _, f := range []string{".trestle.yml", "docs/architecture/system.d2", "CONVENTIONS.md", "AGENTS.md"} {
		if !exists(t, root, f) {
			t.Errorf("%s not written", f)
		}
	}

	// The proposal is the point of the command: a `discover:` rule nobody saw
	// is a rule nobody agreed to.
	for _, want := range []string{"app/services/*/", "2 directories", "billing", "node_modules"} {
		if want == "node_modules" {
			if strings.Contains(stdout, want) {
				t.Errorf("proposed a rule matching %s; the default excludes should have hidden it", want)
			}
			continue
		}
		if !strings.Contains(stdout, want) {
			t.Errorf("proposal does not mention %q:\n%s", want, stdout)
		}
	}
}

// The acceptance criterion, end to end: `check` immediately after `init` has to
// run. Exit 1 is the correct answer — every discovered directory is unowned
// until somebody draws it — and exit 2 would mean `init` scaffolded a repo
// Trestle cannot read.
func TestCheckRunsImmediatelyAfterInit(t *testing.T) {
	root := initRepo(t)

	if _, stderr, code := runCLI(t, root, "init", "--yes"); code != exitClean {
		t.Fatalf("init exit = %d (stderr: %s)", code, stderr)
	}

	stdout, stderr, code := runCLI(t, root, "check")
	if code == exitTool {
		t.Fatalf("check after init is a tool error: %s%s", stdout, stderr)
	}
	if code != exitFail {
		t.Fatalf("check exit = %d, want %d", code, exitFail)
	}
	// Five services and libraries, all unowned, and nothing else: no ORPHAN
	// from a zero-match `discover:` rule, no SYNTAX from the starter diagram.
	if n := strings.Count(stdout, "UNMAPPED"); n != 5 {
		t.Errorf("%d UNMAPPED, want 5:\n%s", n, stdout)
	}
	for _, code := range []string{"ORPHAN", "DANGLING", "SYNTAX", "UNBOUND"} {
		if strings.Contains(stdout, code) {
			t.Errorf("unexpected %s in a freshly scaffolded repo:\n%s", code, stdout)
		}
	}
	// Each finding names the line that fixes it. That is what makes the first
	// run a to-do list rather than a wall.
	if !strings.Contains(stdout, "@bind svc_billing app/services/billing/**") {
		t.Errorf("UNMAPPED did not carry a runnable hint:\n%s", stdout)
	}
}

// `explain` and `render` also have to work in a scaffolded repo. A starter
// diagram with no nodes is an unusual input for both of them.
func TestExplainAndRenderWorkAfterInit(t *testing.T) {
	root := initRepo(t)
	if _, stderr, code := runCLI(t, root, "init", "--yes"); code != exitClean {
		t.Fatalf("init exit = %d (stderr: %s)", code, stderr)
	}

	stdout, stderr, code := runCLI(t, root, "explain")
	if code != exitClean {
		t.Fatalf("explain exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "no nodes") {
		t.Errorf("explain does not say the diagram is empty:\n%s", stdout)
	}

	if _, stderr, code := runCLI(t, root, "render", "--quiet"); code != exitClean {
		t.Fatalf("render exit = %d (stderr: %s)", code, stderr)
	}
	if !exists(t, root, "docs/architecture/rendered/system.svg") {
		t.Error("render wrote no SVG; the scaffolded `render.out` is wrong")
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	root := initRepo(t)

	stdout, _, code := runCLI(t, root, "init", "--dry-run")
	if code != exitClean {
		t.Fatalf("exit = %d, want %d", code, exitClean)
	}
	if exists(t, root, ".trestle.yml") {
		t.Error("--dry-run wrote the config")
	}
	if !strings.Contains(stdout, "none were") {
		t.Errorf("--dry-run does not say nothing was written:\n%s", stdout)
	}
}

func TestInitPromptsAndHonorsTheAnswer(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		root := initRepo(t)
		stdout, _, code := runCLIStdin(t, root, "y\n", "init")
		if code != exitClean {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(stdout, "[y/N]") {
			t.Error("no prompt was printed")
		}
		if !exists(t, root, ".trestle.yml") {
			t.Error("answered yes and nothing was written")
		}
	})

	t.Run("no", func(t *testing.T) {
		root := initRepo(t)
		stdout, _, code := runCLIStdin(t, root, "n\n", "init")
		if code != exitClean {
			t.Fatalf("exit = %d, want %d: declining is an answer, not a failure", code, exitClean)
		}
		if exists(t, root, ".trestle.yml") {
			t.Error("answered no and it wrote anyway")
		}
		if !strings.Contains(stdout, "Nothing written") {
			t.Errorf("does not say it wrote nothing:\n%s", stdout)
		}
	})

	// A CI runner has stdin on /dev/null. Treating that as a decline would make
	// `trestle init` a command that reports success having done nothing, which
	// is the quiet no-op this repo keeps refusing to ship.
	t.Run("no answer available", func(t *testing.T) {
		root := initRepo(t)
		_, stderr, code := runCLIStdin(t, root, "", "init")
		if code != exitTool {
			t.Fatalf("exit = %d, want %d", code, exitTool)
		}
		if !strings.Contains(stderr, "--yes") {
			t.Errorf("the refusal does not say what to type instead: %s", stderr)
		}
		if exists(t, root, ".trestle.yml") {
			t.Error("wrote the config without an answer")
		}
	})
}

func TestInitTwiceLeavesTheConfigAlone(t *testing.T) {
	root := initRepo(t)
	if _, _, code := runCLI(t, root, "init", "--yes"); code != exitClean {
		t.Fatalf("first init exit = %d", code)
	}

	cfg := filepath.Join(root, ".trestle.yml")
	customized := "version: 1\ndiagrams: [docs/architecture/*.d2]\ndiscover: [app/services/*/]\n"
	if err := os.WriteFile(cfg, []byte(customized), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runCLI(t, root, "init", "--yes")
	if code != exitClean {
		t.Fatalf("second init exit = %d", code)
	}
	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != customized {
		t.Error("a customized .trestle.yml was clobbered by a second init")
	}
	if !strings.Contains(stdout, "Nothing to do") {
		t.Errorf("second run does not say the repo is already set up:\n%s", stdout)
	}
}

func TestInitRefusesToNestARoot(t *testing.T) {
	root := initRepo(t)
	if _, _, code := runCLI(t, root, "init", "--yes"); code != exitClean {
		t.Fatalf("init exit = %d", code)
	}

	sub := filepath.Join(root, "app", "services")
	_, stderr, code := runCLI(t, sub, "init", "--yes")
	if code != exitTool {
		t.Fatalf("exit = %d, want %d", code, exitTool)
	}
	if !strings.Contains(stderr, "nested repo root") {
		t.Errorf("the refusal does not explain itself: %s", stderr)
	}
}
