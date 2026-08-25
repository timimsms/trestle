package integration

import (
	"go/build"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/check"
	"github.com/timimsms/trestle/internal/expected"
	"github.com/timimsms/trestle/internal/walk"
)

// ioPackages are the imports that would mean the seam is in the wrong place.
//
// The rule is not "check must be simple"; it is that the repo walk lives in
// internal/walk and check is a function of its result. The moment check can
// open a file it can also disagree with the listing it was handed, and every
// violation it produces stops being reproducible from its inputs.
var ioPackages = map[string]string{
	"os":            "the walk happened in internal/walk; check is a function of its result",
	"os/exec":       "check runs nothing",
	"io":            "check reads nothing",
	"io/fs":         "the listing is passed in as []check.Entry",
	"io/ioutil":     "check reads nothing",
	"path/filepath": "listing paths are slash-separated and repo-relative; use `path`",
	"net":           "check is local and offline",
	"net/http":      "check is local and offline",
	"bufio":         "check parses nothing from a stream",
	"github.com/timimsms/trestle/internal/walk": "importing walk drags io/fs across the seam; mirror the two fields instead",
}

// allowedInternal is the dependency direction CONTRIBUTING.md, Architecture fixes: check depends
// on the three parse packages' types, plus `lang`, and on nothing else.
//
// What the rule actually protects is two things — the engine does no I/O, and
// it does not depend on the walk. Every entry here is checked against that
// rather than against the list's length.
//
// `lang` was added deliberately. It is a leaf: pure constants describing each
// ecosystem's conventions, importing nothing, so it can introduce neither I/O
// nor a walk dependency. It is here because the node-ID prefixes it holds are
// Rails vocabulary that used to live inside this package with nothing knowing
// that — on a Go repo the hint suggested `svc_db` for a package named `db`,
// which appears nowhere in the repo and collides with the `db_` prefix
// CONVENTIONS reserves for datastores.
//
// Adding to this list is a decision, not a formality. The bar is: can the
// import bring I/O or the filesystem walk into the engine, now or later?
var allowedInternal = map[string]bool{
	"github.com/timimsms/trestle/internal/config":    true,
	"github.com/timimsms/trestle/internal/directive": true,
	"github.com/timimsms/trestle/internal/nodes":     true,
	"github.com/timimsms/trestle/internal/lang":      true,
}

// TestLangIsALeaf backs the reasoning above: `lang` is allowed into the engine
// because it imports nothing, so it cannot smuggle I/O in behind it. If that
// ever stops being true, this fails before the engine quietly gains a
// dependency it was built to refuse.
func TestLangIsALeaf(t *testing.T) {
	pkg, err := build.ImportDir("../lang", 0)
	if err != nil {
		t.Fatalf("read internal/lang: %v", err)
	}
	for _, imp := range pkg.Imports {
		t.Errorf("internal/lang imports %q; it is allowed into internal/check only because it is a leaf", imp)
	}
}

// TestCheckIsIOFree is the standing constraint made mechanical. "internal/check
// stays I/O-free" has been written down four times across the handoff; a test is
// the only form of it that survives a refactor by someone who has not read them.
func TestCheckIsIOFree(t *testing.T) {
	pkg, err := build.ImportDir("../check", 0)
	if err != nil {
		t.Fatalf("read internal/check: %v", err)
	}

	// Test files are held to the same rule: a check unit test that reaches for
	// testdata is testing a seam, and seams are tested here.
	groups := map[string][]string{
		"package":        pkg.Imports,
		"test files":     pkg.TestImports,
		"external tests": pkg.XTestImports,
	}
	for group, imports := range groups {
		for _, imp := range imports {
			if why, bad := ioPackages[imp]; bad {
				t.Errorf("internal/check %s import %q: %s", group, imp, why)
			}
			if strings.HasPrefix(imp, "github.com/timimsms/trestle/") && !allowedInternal[imp] {
				t.Errorf("internal/check %s import %q: not in the allowlist — see allowedInternal", group, imp)
			}
		}
	}
}

// TestCheckEntryMirrorsWalkEntry pins the one deliberate duplication in the
// codebase. check.Entry exists because importing walk.Entry would pull io/fs
// across the seam the test above defends; that trade is only sound while the
// two shapes are identical, and a field added to one and not the other is a
// silent data loss at the exact point the listing enters the engine.
func TestCheckEntryMirrorsWalkEntry(t *testing.T) {
	got, want := reflect.TypeOf(check.Entry{}), reflect.TypeOf(walk.Entry{})
	if got.NumField() != want.NumField() {
		t.Fatalf("check.Entry has %d fields, walk.Entry has %d; the mirror has drifted",
			got.NumField(), want.NumField())
	}
	for i := 0; i < want.NumField(); i++ {
		w, g := want.Field(i), got.Field(i)
		if w.Name != g.Name || w.Type != g.Type {
			t.Errorf("field %d: check.Entry has %s %s, walk.Entry has %s %s",
				i, g.Name, g.Type, w.Name, w.Type)
		}
	}
}

// The taxonomy is written down in three packages: check emits the codes, config
// validates them as `severity:` keys, and expected parses them out of fixture
// files. check_test pins check to config; this pins both to the fixtures, so a
// sixth code cannot arrive by being added to two of the three.
func TestViolationCodesAgreeAcrossPackages(t *testing.T) {
	mine := make([]string, 0, len(check.Codes))
	for _, c := range check.Codes {
		mine = append(mine, string(c))
	}
	sort.Strings(mine)

	theirs := append([]string(nil), expected.Codes...)
	sort.Strings(theirs)

	if strings.Join(mine, ",") != strings.Join(theirs, ",") {
		t.Errorf("violation codes disagree\n check:    %v\n expected: %v", mine, theirs)
	}
	if len(mine) != 5 {
		t.Errorf("the taxonomy is closed at five, got %d: %v", len(mine), mine)
	}
}
