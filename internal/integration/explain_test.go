package integration

import (
	"go/build"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/explain"
)

// `explain` is a second window on the engine, and the two ways that goes wrong
// are guarded here: it can stop being a pure function of the engine's input, and
// its vocabulary can quietly become a sixth violation code.

// explainForbidden are the imports that would mean `explain` had grown its own
// idea of what is on disk. `io` and `bufio` are absent from this list on
// purpose — it writes to an io.Writer, which is not the same thing as reading
// the repo.
var explainForbidden = map[string]string{
	"os":            "explain is a function of check.Input; the walk already happened in internal/walk",
	"os/exec":       "explain runs nothing",
	"io/fs":         "the listing is passed in as []check.Entry",
	"io/ioutil":     "explain reads nothing",
	"path/filepath": "listing paths are slash-separated and repo-relative; use `path`",
	"net":           "explain is local and offline",
	"net/http":      "explain is local and offline",
	"github.com/timimsms/trestle/internal/walk": "explain reports on the one listing it was handed, not on a walk of its own",
	"github.com/timimsms/trestle/internal/run":  "run wires the pipeline and calls explain, not the other way round",
}

// TestExplainIsIOFree keeps `explain` answering questions about the same listing
// `check` ran on. The moment it can open a file it can disagree with the check
// it exists to debug, and a debugging command that quietly contradicts the
// command it debugs is worse than no debugging command.
func TestExplainIsIOFree(t *testing.T) {
	pkg, err := build.ImportDir("../explain", 0)
	if err != nil {
		t.Fatalf("read internal/explain: %v", err)
	}
	groups := map[string][]string{
		"package":        pkg.Imports,
		"test files":     pkg.TestImports,
		"external tests": pkg.XTestImports,
	}
	for group, imports := range groups {
		for _, imp := range imports {
			if why, bad := explainForbidden[imp]; bad {
				t.Errorf("internal/explain %s import %q: %s", group, imp, why)
			}
		}
	}
}

// The statuses are a vocabulary, not a taxonomy. They must stay lowercase and
// stay six, because a new uppercase word in `explain`'s first column is exactly
// how a sixth violation code would arrive without anyone deciding to add one.
//
// One status does share its name with a code, deliberately: an `unbound` node is
// precisely the node UNBOUND reports on, and inventing a second word for one
// condition would be worse than the echo. It is pinned as the *only* such
// overlap so that the next one has to be argued for.
func TestExplainStatusesStayAVocabulary(t *testing.T) {
	if len(explain.Statuses) != 6 {
		t.Errorf("statuses = %d, want the six: %v", len(explain.Statuses), explain.Statuses)
	}
	if len(check.Codes) != 5 {
		t.Errorf("the taxonomy is closed at five, got %d", len(check.Codes))
	}

	codes := map[string]bool{}
	for _, c := range check.Codes {
		codes[strings.ToLower(string(c))] = true
	}
	var shared []string
	for _, s := range explain.Statuses {
		if string(s) != strings.ToLower(string(s)) {
			t.Errorf("status %q is not lowercase; the uppercase words in this output are the five codes", s)
		}
		if codes[strings.ToLower(string(s))] {
			shared = append(shared, string(s))
		}
	}
	if strings.Join(shared, ",") != "unbound" {
		t.Errorf("statuses sharing a name with a violation code = %v, want only [unbound]", shared)
	}
}
