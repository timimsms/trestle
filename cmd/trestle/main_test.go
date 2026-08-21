package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// subprocessEnv turns the test binary into `trestle` itself. It exists so a
// handful of assertions can be made about the real process — its exit status,
// and the fact that output down a pipe carries no ANSI escapes — rather than
// about a function that returns an int.
const subprocessEnv = "TRESTLE_TEST_SUBPROCESS"

// update rewrites the committed golden files. Run:
//
//	go test ./cmd/trestle -update
var update = flag.Bool("update", false, "rewrite golden files")

// origWD is the package directory, captured before any test changes directory.
// Every test here runs `check` from somewhere else, so paths to fixtures and
// golden files have to be resolved against this rather than against the
// current directory.
var origWD string

func TestMain(m *testing.M) {
	if os.Getenv(subprocessEnv) == "1" {
		os.Exit(Main(os.Args[1:], os.Stdout, os.Stderr))
	}
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	origWD = wd

	code := m.Run()
	if benchRoot != "" {
		_ = os.RemoveAll(benchRoot)
	}
	os.Exit(code)
}

// fixtureDir is the absolute path of one of the ten Phase 1 fixture repos.
func fixtureDir(name string) string {
	return filepath.Join(origWD, "..", "..", "testdata", "repos", name)
}

// fixtureNames lists the fixture repos. The acceptance set is ten; fewer means
// a fixture went missing, which would quietly shrink the contract.
func fixtureNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(origWD, "..", "..", "testdata", "repos"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) < 10 {
		t.Fatalf("found %d fixtures, want the ten of the acceptance set", len(out))
	}
	return out
}

// runCLI invokes the command exactly as main() does, in dir, and returns what
// the process would have written and exited with.
//
// The writers are buffers, which is also the point: a bytes.Buffer is not a
// terminal, so every assertion in this file is made against the plain output a
// CI log would receive.
func runCLI(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	code = Main(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

// runSubprocess re-executes the test binary as `trestle` and returns its
// combined output and real exit status. exec.Cmd wires stdout to a pipe, which
// is what makes this the honest test of "not a terminal".
func runSubprocess(t *testing.T, bin, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), subprocessEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(out), ee.ExitCode()
	}
	t.Fatalf("run %s: %v", bin, err)
	return "", -1
}
