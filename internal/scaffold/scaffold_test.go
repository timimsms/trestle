package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timimsms/trestle"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/directive"
	"github.com/timimsms/trestle/internal/nodes"
)

// repo builds a temp directory holding the given files and returns its path.
// Content is irrelevant to detection — only the shape of the tree is — so every
// file gets the same byte.
func repo(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
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

func plan(t *testing.T, root string) *Plan {
	t.Helper()
	p, err := Prepare(root)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return p
}

func artifact(t *testing.T, p *Plan, path string) Artifact {
	t.Helper()
	for _, a := range p.Artifacts {
		if a.Path == path {
			return a
		}
	}
	t.Fatalf("no artifact for %s in %v", path, p.Artifacts)
	return Artifact{}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestPrepareScaffoldsFourFiles(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb", "packages/db/index.ts")
	p := plan(t, root)

	if got := p.Writes(); got != 4 {
		t.Fatalf("Writes() = %d, want 4 (%v)", got, p.Artifacts)
	}
	for _, path := range []string{ConfigPath, DiagramPath, ConventionsPath, AgentsPath} {
		if a := artifact(t, p, path); a.Action != Create {
			t.Errorf("%s: action = %s, want create", path, a.Action)
		}
	}
	if got := p.Units(); got != 2 {
		t.Errorf("Units() = %d, want 2", got)
	}
}

// The emitted config is the artifact most people will read and least likely to
// be re-parsed by hand, so the test that matters is that Trestle itself accepts
// it. A scaffolded config that fails validation would make `init` a command that
// breaks the repo it was run in.
func TestEmittedConfigValidates(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb", "lib/http_client/client.rb")
	p := plan(t, root)

	cfg, err := config.Parse(filepath.Join(root, ConfigPath), []byte(artifact(t, p, ConfigPath).Payload))
	if err != nil {
		t.Fatalf("scaffolded config does not validate: %v", err)
	}
	if len(cfg.Diagrams) == 0 {
		t.Error("no diagrams: a config without one is a tool error on the first check")
	}
	if len(cfg.Shared) != 0 {
		t.Errorf("shared = %v, want empty: L11 says init cannot get this right on its own", cfg.Shared)
	}
	want := []string{"app/services/*/", "lib/*/"}
	if strings.Join(cfg.Discover, ",") != strings.Join(want, ",") {
		t.Errorf("discover = %v, want %v", cfg.Discover, want)
	}
	if cfg.Render.Out == "" {
		t.Error("no render.out: `trestle render` would exit 2 on a freshly scaffolded repo")
	}
}

// The L11 warning has to be in the file, not only in the docs. `shared:` is the
// one list a user fills in later, alone, without re-reading anything.
func TestEmittedConfigCarriesTheSharedWarning(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	body := artifact(t, plan(t, root), ConfigPath).Payload

	for _, want := range []string{"enumerated, never blanket", "lib/**", "architecture review"} {
		if !strings.Contains(body, want) {
			t.Errorf("emitted config does not mention %q", want)
		}
	}
}

// A repo with no recognized shape still gets a valid config. Writing a
// `discover:` rule that matches nothing would be worse than writing none: a
// zero-match rule is an ORPHAN, so `init` would hand back a repo that fails.
func TestEmittedConfigValidatesWithNoRules(t *testing.T) {
	root := repo(t, "main.go")
	p := plan(t, root)

	if len(p.Rules) != 0 {
		t.Fatalf("rules = %v, want none", p.Rules)
	}
	body := artifact(t, p, ConfigPath).Payload
	if _, err := config.Parse(filepath.Join(root, ConfigPath), []byte(body)); err != nil {
		t.Fatalf("config with no rules does not validate: %v", err)
	}
	if !strings.Contains(body, "discover: []") {
		t.Error("want an explicit empty discover list")
	}
}

// The starter diagram must compile — `check` treats a diagram it cannot compile
// as exit 2, which is the one outcome the acceptance criterion rules out — and
// it must contain no nodes, which is the decision recorded in the package doc.
func TestStarterDiagramCompilesAndIsEmpty(t *testing.T) {
	d, err := nodes.Parse(DiagramPath, []byte(diagramFile))
	if err != nil {
		t.Fatalf("starter diagram does not compile: %v", err)
	}
	if d.Len() != 0 {
		t.Errorf("starter diagram has %d nodes, want none: init does not invent boxes", d.Len())
	}
}

// The scanner strips the leading run of `#` before looking for `@`, so there is
// no way to comment out a directive: `## @bind svc_x app/x/**` is live. The
// starter diagram talks about directives at length, and every one of those
// mentions has to stay prose — a live `@bind` in a file with no nodes in it
// would be DANGLING on the first run, from a file Trestle wrote itself.
func TestStarterDiagramHasNoLiveDirectives(t *testing.T) {
	res := directive.Parse(DiagramPath, []byte(diagramFile))
	if len(res.Directives) != 0 {
		t.Errorf("starter diagram parses %d directives, want none: %+v", len(res.Directives), res.Directives)
	}
	if len(res.Syntax) != 0 {
		t.Errorf("starter diagram parses %d syntax errors, want none: %+v", len(res.Syntax), res.Syntax)
	}
	// The examples still have to be *there*; a file that avoids the trap by
	// saying nothing teaches nothing.
	if !strings.Contains(diagramFile, "@bind svc_billing") {
		t.Error("starter diagram no longer shows a binding example")
	}
}

func TestApplyWritesEverythingItPlanned(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	p := plan(t, root)
	if err := p.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := read(t, root, ConventionsPath); got != trestle.Conventions {
		t.Error("CONVENTIONS.md is not the embedded copy")
	}
	if got := read(t, root, DiagramPath); got != diagramFile {
		t.Error("starter diagram is not what was planned")
	}
	if got := read(t, root, AgentsPath); !strings.Contains(got, agentsMarker) {
		t.Error("AGENTS.md has no stanza")
	}
	if got := read(t, root, AgentsPath); !strings.Contains(got, "trestle explain") {
		t.Error("the AGENTS.md stanza does not point at `trestle explain`")
	}
}

// Idempotency, which is the difference between a command people re-run and one
// they are afraid of.
func TestRunningTwiceClobbersNothing(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	if err := plan(t, root).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	customized := read(t, root, ConfigPath) + "\n# a rule somebody thought about\n"
	if err := os.WriteFile(filepath.Join(root, ConfigPath), []byte(customized), 0o644); err != nil {
		t.Fatal(err)
	}

	second := plan(t, root)
	if got := second.Writes(); got != 0 {
		t.Errorf("second run would write %d files, want 0", got)
	}
	if a := artifact(t, second, ConfigPath); a.Action != Keep {
		t.Errorf("config action = %s, want keep", a.Action)
	}
	if err := second.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := read(t, root, ConfigPath); got != customized {
		t.Error("a customized .trestle.yml was modified")
	}
}

// Prepare and Apply are separated by however long the user takes to answer the
// prompt, so "the file was not there a moment ago" is not a guarantee. O_EXCL
// makes the open itself the guard.
func TestApplyRefusesToOverwriteAFileThatAppeared(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	p := plan(t, root)

	if err := os.WriteFile(filepath.Join(root, ConfigPath), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := p.Apply()
	if err == nil {
		t.Fatal("Apply overwrote a file that appeared between plan and write")
	}
	if !strings.Contains(err.Error(), ConfigPath) {
		t.Errorf("error does not name the file: %v", err)
	}
}

func TestAgentsStanzaIsAppendedNotOverwritten(t *testing.T) {
	const existing = "# House rules\n\nRun the tests.\n"
	root := repo(t, "app/services/billing/billing.rb")
	if err := os.WriteFile(filepath.Join(root, AgentsPath), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	p := plan(t, root)
	if a := artifact(t, p, AgentsPath); a.Action != Append {
		t.Fatalf("action = %s, want append", a.Action)
	}
	if err := p.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := read(t, root, AgentsPath)
	if !strings.HasPrefix(got, existing) {
		t.Error("the repo's own AGENTS.md was not preserved verbatim at the top")
	}
	if !strings.Contains(got, agentsMarker) {
		t.Error("no stanza appended")
	}

	// A third state: the file exists, ends without a newline, and already has
	// the stanza. The second append must not happen.
	if p2 := plan(t, root); artifact(t, p2, AgentsPath).Action != Unchanged {
		t.Error("a second run would append the stanza twice")
	}
}

func TestAgentsStanzaSeparatesFromAFileWithNoTrailingNewline(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	if err := os.WriteFile(filepath.Join(root, AgentsPath), []byte("no trailing newline"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := plan(t, root).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := read(t, root, AgentsPath); !strings.Contains(got, "newline\n\n"+agentsMarker) {
		t.Errorf("stanza not separated from the existing text: %q", got)
	}
}

func TestConventionsKeptWhenTheRepoHasEditedIt(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	if err := os.WriteFile(filepath.Join(root, ConventionsPath), []byte("# ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := artifact(t, plan(t, root), ConventionsPath)
	if a.Action != Keep {
		t.Errorf("action = %s, want keep", a.Action)
	}
	if a.Why == "" {
		t.Error("a kept file with no reason reads as a command that silently did nothing")
	}
}

// A repo that already draws diagrams does not need an empty one added next to
// them; it would show up as a second file in every check header forever.
func TestStarterDiagramSkippedWhenOneExists(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb", "docs/architecture/context.d2")
	p := plan(t, root)

	if a := artifact(t, p, "docs/architecture/context.d2"); a.Action != Keep {
		t.Errorf("action = %s, want keep", a.Action)
	}
	if p.Writes() != 3 {
		t.Errorf("Writes() = %d, want 3", p.Writes())
	}
}

// A second `.trestle.yml` in a subdirectory creates a second root, and every
// relative path in both configs then resolves against a directory its author
// did not mean. The first symptom is ORPHAN on bindings that are correct.
func TestPrepareRefusesANestedRoot(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb", "sub/app/services/x/x.rb")
	if err := os.WriteFile(filepath.Join(root, ConfigPath), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Prepare(filepath.Join(root, "sub"))
	var nested *NestedRootError
	if !errors.As(err, &nested) {
		t.Fatalf("Prepare in a subdirectory = %v, want a NestedRootError", err)
	}
	if !strings.Contains(err.Error(), "hint:") {
		t.Error("the refusal does not say what to do instead")
	}
}

// Prepare reports the existing `discover:` list so a second run can say which
// shape the config does not cover yet. That is what makes re-running it useful
// on a repo that grew a new directory, rather than a no-op.
func TestPrepareReportsTheExistingDiscoverList(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb", "packages/db/index.ts")
	if err := plan(t, root).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	second := plan(t, root)
	if len(second.Existing) != 2 {
		t.Fatalf("Existing = %v, want the two scaffolded rules", second.Existing)
	}
	var buf strings.Builder
	if err := second.WriteProposal(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "not in your `discover:`") {
		t.Error("a rule the config already has was reported as missing")
	}
}

// The bootstrap procedure is the answer to "the diagram is empty, now what".
// init deliberately writes no nodes, so the stanza has to say how to get from
// an empty canvas to a real diagram — otherwise the empty-diagram decision
// just relocates the problem instead of solving it.
func TestAgentsStanzaExplainsHowToStart(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	if err := plan(t, root).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := read(t, root, AgentsPath)

	for _, want := range []string{
		"first diagram",       // the section exists
		"--format=json",       // names the machine-readable inventory
		"Ask about the edges", // the step that needs a human
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the AGENTS.md stanza is missing %q", want)
		}
	}
}

// Edges are the one thing `check` cannot verify, so a wrong one is never
// contradicted by anything downstream. The stanza must tell an agent to ask
// rather than infer, and must say why — a rule without its reason is one an
// agent will optimize away when inferring looks faster.
func TestAgentsStanzaWarnsAgainstInferringEdges(t *testing.T) {
	root := repo(t, "app/services/billing/billing.rb")
	if err := plan(t, root).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := read(t, root, AgentsPath)
	if !strings.Contains(got, "Do not infer") {
		t.Error("the stanza does not tell agents to ask about edges rather than infer them")
	}
	if !strings.Contains(got, "cannot verify") {
		t.Error("the stanza states the rule without the reason it exists")
	}
}
