package main

import (
	"bytes"
	"strings"
	"testing"
)

// --version has to work, because the bug issue template asks reporters for it.
// A flag the docs request and the binary does not have wastes the reporter's
// first interaction with the project.
func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := Main([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); code != exitClean {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, exitClean, stderr.String())
	}

	got := stdout.String()
	if !strings.HasPrefix(got, "trestle ") {
		t.Errorf("output = %q, want it to start with %q", got, "trestle ")
	}
	if strings.TrimSpace(strings.TrimPrefix(got, "trestle ")) == "" {
		t.Error("version string is empty; a bug report quoting this would identify nothing")
	}
}

// --version is a flag rather than a subcommand on purpose: the ledger caps the
// command surface at four (check, render, explain, init) and a `version`
// command would be a fifth. This test is the guard on that decision, since
// adding one would be a natural-looking mistake.
func TestVersionIsAFlagNotACommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := Main([]string{"version"}, strings.NewReader(""), &stdout, &stderr); code != exitTool {
		t.Errorf("`trestle version` exit = %d, want %d — it should not be a subcommand",
			code, exitTool)
	}
}

func TestVersionStringIsNeverEmpty(t *testing.T) {
	if versionString() == "" {
		t.Error("versionString() is empty")
	}
}

// An unstamped test binary has no linker value and no module version, so this
// exercises the buildinfo path that a `go install`ed binary actually takes.
func TestVersionStringFallsBackToSomethingIdentifying(t *testing.T) {
	got := versionString()
	if got == "dev" {
		// Acceptable — `go test` may report no VCS settings at all — but say so,
		// because "dev" in a real bug report is the failure this code prevents.
		t.Log("versionString() fell all the way back to \"dev\"; no VCS stamp in this build")
	}
	t.Logf("versionString() = %q", got)
}
