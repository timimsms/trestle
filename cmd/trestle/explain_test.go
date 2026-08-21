package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle/internal/nodes"
)

// `explain` reports and does not judge. The whole command is downstream of that
// sentence, and the first two tests pin it.

// Exit 0 whatever it finds, on every fixture — including the ones where `check`
// exits 1. If `explain` could fail, it would end up in CI as a second check with
// a different opinion, and the point of it is to be the thing you run when the
// first check already failed.
func TestExplainAlwaysExitsZero(t *testing.T) {
	for _, name := range append(fixtureNames(t), "examples/repairs-platform") {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			if strings.HasPrefix(name, "examples/") {
				dir = filepath.Join(origWD, "..", "..", name)
			}
			for _, args := range [][]string{
				{"explain"},
				{"explain", "--format=json"},
				{"explain", "--overlaps"},
				{"explain", "--overlaps", "--format=json"},
			} {
				out, errOut, code := runCLI(t, dir, args...)
				if code != 0 {
					t.Errorf("%v: exit = %d, want 0\nstdout: %s\nstderr: %s", args, code, out, errOut)
				}
				if errOut != "" {
					t.Errorf("%v: unexpected stderr: %s", args, errOut)
				}
				if out == "" {
					t.Errorf("%v: printed nothing; silence reads as `did not run`", args)
				}
			}
		})
	}
}

// A node with failing violations is still reported at exit 0. The fixture that
// makes this concrete is `orphan`, where `check` exits 1 on the same node.
func TestExplainOnAFailingNodeStillExitsZero(t *testing.T) {
	if _, _, code := runCLI(t, fixtureDir("orphan"), "check"); code != 1 {
		t.Fatal("the orphan fixture is supposed to fail `check`")
	}
	out, _, code := runCLI(t, fixtureDir("orphan"), "explain", "svc_reporting")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "ORPHAN") || !strings.Contains(out, "matches 0 files") {
		t.Errorf("the violation and its evidence are missing:\n%s", out)
	}
	if !strings.Contains(out, "hint: ") {
		t.Errorf("no runnable hint:\n%s", out)
	}
}

// --- the inventory ------------------------------------------------------

// The most-requested capability: with no argument, list every node the tool
// parsed. "Every" is the load-bearing word, so it is checked against the
// parser's own answer rather than against a hand-written list.
func TestInventoryListsExactlyTheParsedNodeSet(t *testing.T) {
	for _, name := range []string{"nested", "ambiguous", "overlap", "unbound"} {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			dg, err := nodes.ParseFile(filepath.Join(dir, "docs", "architecture", "system.d2"))
			if err != nil {
				t.Fatal(err)
			}
			out, _, _ := runCLI(t, dir, "explain", "--format=json")
			doc := decodeExplain(t, out)

			got := make([]string, 0, len(doc.Nodes))
			for _, n := range doc.Nodes {
				got = append(got, n.ID)
			}
			if strings.Join(got, ",") != strings.Join(dg.IDs, ",") {
				t.Errorf("inventory = %v\nparser    = %v", got, dg.IDs)
			}
			if doc.Summary.Nodes != len(dg.IDs) {
				t.Errorf("summary.nodes = %d, want %d", doc.Summary.Nodes, len(dg.IDs))
			}
			for _, n := range doc.Nodes {
				if n.Status == "" {
					t.Errorf("%s has no binding status", n.ID)
				}
			}
		})
	}
}

// Every node has a status and every status is one of the six. These are not
// violation codes and must never turn into a sixth one.
func TestStatusesAreTheSixAndNotViolationCodes(t *testing.T) {
	valid := map[string]bool{
		"bound": true, "external": true, "infra": true,
		"ignored": true, "container": true, "unbound": true,
	}
	for _, name := range fixtureNames(t) {
		out, _, _ := runCLI(t, fixtureDir(name), "explain", "--format=json")
		for _, n := range decodeExplain(t, out).Nodes {
			if !valid[n.Status] {
				t.Errorf("%s: %s: unknown status %q", name, n.ID, n.Status)
			}
			if strings.ToUpper(n.Status) == n.Status && n.Status != "" {
				t.Errorf("%s: status %q looks like a violation code", name, n.Status)
			}
		}
	}
}

// --- PHASE_5 acceptance -------------------------------------------------

// "explain platform.svc_work_orders on the worked example lists 2 globs with
// their current match counts."
func TestWorkedExampleNodeListsBothGlobsWithCounts(t *testing.T) {
	dir := filepath.Join(origWD, "..", "..", "examples", "repairs-platform")
	out, _, code := runCLI(t, dir, "explain", "platform.svc_work_orders", "--format=json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	doc := decodeExplain(t, out)
	if len(doc.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(doc.Nodes))
	}
	n := doc.Nodes[0]
	if len(n.Bindings) != 2 {
		t.Fatalf("bindings = %d, want 2: %+v", len(n.Bindings), n.Bindings)
	}
	for _, b := range n.Bindings {
		if b.Matches != 1 {
			t.Errorf("%s matches %d, want 1", b.Glob, b.Matches)
		}
		if len(b.Files) != b.Matches {
			t.Errorf("%s: %d files listed for %d matches", b.Glob, len(b.Files), b.Matches)
		}
	}
	human, _, _ := runCLI(t, dir, "explain", "platform.svc_work_orders")
	for _, want := range []string{
		"app/services/work_orders/** ", "app/models/work_order*.rb ", "matches 1 file",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human output is missing %q:\n%s", want, human)
		}
	}
}

// "Suffix-resolved IDs work, and an ambiguous ID lists candidates rather than
// erroring." (O8, from the debugging side.)
func TestSuffixResolutionAndAmbiguity(t *testing.T) {
	dir := filepath.Join(origWD, "..", "..", "examples", "repairs-platform")
	short, _, code := runCLI(t, dir, "explain", "svc_work_orders", "--format=json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if id := decodeExplain(t, short).Nodes[0].ID; id != "platform.svc_work_orders" {
		t.Errorf("suffix resolved to %q", id)
	}

	amb := fixtureDir("ambiguous")
	out, _, code := runCLI(t, amb, "explain", "svc_search", "--format=json")
	if code != 0 {
		t.Errorf("an ambiguous ID must not error: exit = %d", code)
	}
	doc := decodeExplain(t, out)
	if len(doc.Nodes) != 2 {
		t.Fatalf("candidates = %d, want both", len(doc.Nodes))
	}
	if doc.Nodes[0].ID != "billing.svc_search" || doc.Nodes[1].ID != "support.svc_search" {
		t.Errorf("candidates = %v", doc.Nodes)
	}
	human, _, _ := runCLI(t, amb, "explain", "svc_search")
	if !strings.Contains(human, "ambiguous") || !strings.Contains(human, "qualify it") {
		t.Errorf("the human view does not say how to fix it:\n%s", human)
	}
}

// "--overlaps on the overlap/ fixture lists the shared path and exits 0."
func TestOverlapsFixtureListsTheSharedPath(t *testing.T) {
	dir := fixtureDir("overlap")
	out, _, code := runCLI(t, dir, "explain", "--overlaps", "--format=json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	doc := decodeExplain(t, out)
	if len(doc.Overlaps) != 1 {
		t.Fatalf("overlaps = %+v, want the one shared path", doc.Overlaps)
	}
	o := doc.Overlaps[0]
	if o.Path != "app/services/billing/payment_processor.rb" {
		t.Errorf("path = %q", o.Path)
	}
	if len(o.Nodes) != 2 {
		t.Errorf("claimants = %+v, want two nodes", o.Nodes)
	}

	human, _, _ := runCLI(t, dir, "explain", "--overlaps")
	if !strings.Contains(human, "app/services/billing/payment_processor.rb") {
		t.Errorf("the human view omits the path:\n%s", human)
	}
	// L12: legal, and the output must not read as an accusation.
	if !strings.Contains(human, "never a failure") {
		t.Errorf("the human view does not say overlap is legal:\n%s", human)
	}
}

// A repo with no overlaps says so rather than printing nothing, for the same
// reason a clean `check` still prints a summary: silence reads as "did not run".
func TestOverlapsOnACleanRepoSaysSo(t *testing.T) {
	out, _, code := runCLI(t, fixtureDir("clean"), "explain", "--overlaps")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "no path is claimed by more than one node") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

// --- exit 2 -------------------------------------------------------------

// The one non-zero exit. A node ID that names nothing is a question the command
// could not answer, and answering it with a clean exit is how an agent that
// misspelled the ID of the node it just renamed reads the silence as
// confirmation.
func TestUnknownNodeIsAToolError(t *testing.T) {
	out, errOut, code := runCLI(t, fixtureDir("clean"), "explain", "svc_nope")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if out != "" {
		t.Errorf("a tool error wrote to stdout: %q", out)
	}
	if !strings.Contains(errOut, "svc_nope") || !strings.Contains(errOut, "trestle explain") {
		t.Errorf("stderr does not name the node and the fix:\n%s", errOut)
	}
}

func TestOverlapsRejectsANodeArgument(t *testing.T) {
	_, errOut, code := runCLI(t, fixtureDir("clean"), "explain", "--overlaps", "svc_billing")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "--overlaps") {
		t.Errorf("stderr does not explain the misuse:\n%s", errOut)
	}
}

func TestExplainRejectsAnUnknownFormat(t *testing.T) {
	_, errOut, code := runCLI(t, fixtureDir("clean"), "explain", "--format=yaml")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown format") {
		t.Errorf("stderr: %s", errOut)
	}
}

// A missing config is exit 2 here as it is for `check`: the two commands load
// the repo the same way, so they fail the same way.
func TestExplainWithoutAConfigIsAToolError(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package a\n"})
	_, errOut, code := runCLI(t, dir, "explain")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "trestle init") {
		t.Errorf("stderr: %s", errOut)
	}
}

// --- machine format -----------------------------------------------------

// The JSON is what an agent parses before editing a node, so the schema is
// pinned on every fixture rather than on a chosen one.
func TestJSONIsWellFormedEverywhere(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			out, _, _ := runCLI(t, fixtureDir(name), "explain", "--format=json")
			doc := decodeExplain(t, out)
			if doc.Kind != "inventory" {
				t.Errorf("kind = %q", doc.Kind)
			}
			if len(doc.Diagrams) == 0 {
				t.Error("no diagrams listed; the first question is always whether the right file was read")
			}
			for _, n := range doc.Nodes {
				if n.Diagram == "" {
					t.Errorf("%s does not say which diagram it came from", n.ID)
				}
				for _, b := range n.Bindings {
					// The inventory carries counts and not file lists.
					if b.Files != nil {
						t.Errorf("%s: inventory listed files for %s", n.ID, b.Glob)
					}
					if b.Source.File == "" {
						t.Errorf("%s: binding %s has no source", n.ID, b.Glob)
					}
				}
				for _, v := range n.Violations {
					if strings.TrimSpace(v.Hint) == "" {
						t.Errorf("%s: %s has no hint", n.ID, v.Code)
					}
				}
			}
		})
	}
}

// `explain` and `check` must agree about a node's violations. Two surfaces over
// one engine is the design; two opinions is a bug.
func TestExplainAndCheckAgreeOnViolations(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(name)
			checkOut, _, _ := runCLI(t, dir, "check", "--format=json")
			want := map[string]int{}
			for _, v := range decodeDoc(t, checkOut).Violations {
				if v.Node != nil {
					want[v.Code+" "+*v.Node]++
				}
			}

			explainOut, _, _ := runCLI(t, dir, "explain", "--format=json")
			got := map[string]int{}
			for _, n := range decodeExplain(t, explainOut).Nodes {
				for _, v := range n.Violations {
					got[v.Code+" "+n.ID]++
				}
			}

			for k, n := range want {
				// A violation whose node token never resolved has no node to be
				// filed under; it is listed as an unresolved directive instead,
				// which the next assertion covers.
				if got[k] == 0 && strings.HasPrefix(k, "DANGLING ") {
					continue
				}
				if strings.HasPrefix(k, "SYNTAX ") && got[k] == 0 {
					continue
				}
				if got[k] != n {
					t.Errorf("check reports %q %d times, explain %d", k, n, got[k])
				}
			}
			for k, n := range got {
				if want[k] != n {
					t.Errorf("explain reports %q %d times, check %d", k, n, want[k])
				}
			}
		})
	}
}

// Every directive that did not resolve to exactly one node is listed. That is
// the other half of the inventory: a binding that resolved to nothing is
// invisible among the nodes, and an inventory that hid it would look complete.
func TestUnresolvedDirectivesAreReported(t *testing.T) {
	cases := map[string]struct{ kind, node string }{
		"dangling":  {"@bind", "svc_invoicing"},
		"ambiguous": {"@bind", "svc_search"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			out, _, _ := runCLI(t, fixtureDir(name), "explain", "--format=json")
			doc := decodeExplain(t, out)
			if len(doc.Unresolved) == 0 {
				t.Fatalf("nothing listed as unresolved:\n%s", out)
			}
			u := doc.Unresolved[0]
			if u.Kind == nil || *u.Kind != want.kind || u.Node == nil || *u.Node != want.node {
				t.Errorf("unresolved = %+v, want %s %s", u, want.kind, want.node)
			}
			if name == "ambiguous" && len(u.Candidates) != 2 {
				t.Errorf("candidates = %v, want both", u.Candidates)
			}
		})
	}

	// A malformed line has no trustworthy node token (O11), so it is reported
	// with the raw line and no kind at all.
	out, _, _ := runCLI(t, fixtureDir("syntax"), "explain", "--format=json")
	doc := decodeExplain(t, out)
	if len(doc.Unresolved) == 0 {
		t.Fatalf("the syntax fixture listed nothing:\n%s", out)
	}
	for _, u := range doc.Unresolved {
		if u.Kind != nil {
			t.Errorf("a malformed line reported a trusted kind %q", *u.Kind)
		}
		if u.Raw == nil || *u.Raw == "" {
			t.Error("a malformed line must quote itself back")
		}
	}
}

// --- golden files -------------------------------------------------------

// The human view is a format, so it is pinned. The four fixtures cover every
// shape it has: an ordinary repo, a zero-match glob, malformed directives, and
// the full status vocabulary.
func TestExplainHumanOutputMatchesGolden(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		args []string
	}{
		{"inventory_example", "examples/repairs-platform", []string{"explain"}},
		{"inventory_orphan", "orphan", []string{"explain"}},
		{"inventory_syntax", "syntax", []string{"explain"}},
		{"inventory_ambiguous", "ambiguous", []string{"explain"}},
		{"node_work_orders", "examples/repairs-platform", []string{"explain", "svc_work_orders"}},
		{"node_ambiguous", "ambiguous", []string{"explain", "svc_search"}},
		{"overlaps", "overlap", []string{"explain", "--overlaps"}},
		{"overlaps_clean", "clean", []string{"explain", "--overlaps"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := fixtureDir(c.dir)
			if strings.HasPrefix(c.dir, "examples/") {
				dir = filepath.Join(origWD, "..", "..", c.dir)
			}
			out, errOut, code := runCLI(t, dir, c.args...)
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %s", code, errOut)
			}
			compareGolden(t, "explain_"+c.name+".human.txt", out)
		})
	}
}

// JSON goldens pin the schema itself: the version, kind and query fields, null
// versus [] for an absent file list, and the ordering.
func TestExplainJSONMatchesGolden(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		args []string
	}{
		{"inventory_overlap", "overlap", []string{"explain", "--format=json"}},
		{"node_payments", "overlap", []string{"explain", "svc_payments", "--format=json"}},
		{"overlaps", "overlap", []string{"explain", "--overlaps", "--format=json"}},
		{"inventory_ambiguous", "ambiguous", []string{"explain", "--format=json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := runCLI(t, fixtureDir(c.dir), c.args...)
			compareGolden(t, "explain_"+c.name+".json", out)
		})
	}
}

// --- helpers ------------------------------------------------------------

// explainDoc mirrors the `explain --format=json` payload. As with jsonDoc, it is
// declared here rather than exported from internal/explain so that these
// assertions read the bytes the way a third party would.
type explainDoc struct {
	Version  int      `json:"version"`
	Kind     string   `json:"kind"`
	Query    *string  `json:"query"`
	Diagrams []string `json:"diagrams"`
	Disabled []string `json:"disabled"`
	Summary  struct {
		Nodes      int `json:"nodes"`
		Bound      int `json:"bound"`
		Unbound    int `json:"unbound"`
		Bindings   int `json:"bindings"`
		Files      int `json:"files"`
		Overlaps   int `json:"overlaps"`
		Unresolved int `json:"unresolved"`
		Failures   int `json:"failures"`
		Warnings   int `json:"warnings"`
	} `json:"summary"`
	Nodes []struct {
		ID       string `json:"id"`
		Diagram  string `json:"diagram"`
		Status   string `json:"status"`
		Files    int    `json:"files"`
		Bindings []struct {
			Glob    string `json:"glob"`
			Matches int    `json:"matches"`
			Source  struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"source"`
			Files []string `json:"files"`
		} `json:"bindings"`
		Violations []struct {
			Code string `json:"code"`
			Hint string `json:"hint"`
		} `json:"violations"`
	} `json:"nodes"`
	Overlaps []struct {
		Path  string `json:"path"`
		Nodes []struct {
			Node string `json:"node"`
			Glob string `json:"glob"`
		} `json:"nodes"`
	} `json:"overlaps"`
	Unresolved []struct {
		Kind       *string  `json:"kind"`
		Node       *string  `json:"node"`
		Raw        *string  `json:"raw"`
		Candidates []string `json:"candidates"`
	} `json:"unresolved"`
}

func decodeExplain(t *testing.T, s string) explainDoc {
	t.Helper()
	var d explainDoc
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("decode json: %v\n%s", err, s)
	}
	if d.Version != 1 {
		t.Errorf("version = %d, want 1", d.Version)
	}
	return d
}
