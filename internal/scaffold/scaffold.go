// Package scaffold implements `trestle init`: it proposes `discover:` rules
// from the layout it finds, and writes the four files a repo needs to start
// checking its diagrams.
//
// # The starter diagram has no nodes in it, and that was the decision
//
// `init` has to write *some* diagram. `diagrams:` is required, and a config
// whose `diagrams:` glob matches nothing exits 2 — so an `init` that scaffolds
// a config and no diagram fails its own acceptance criterion the moment anyone
// runs `check`. The open question was what goes in it: an empty canvas, or one
// node per discovered unit with the binding already written.
//
// Seeding was the tempting option. It makes the first `trestle check` green and
// hands the user something to edit instead of a blank file. It was rejected:
//
//   - **A green first run would be green because Trestle wrote both sides.**
//     The diagram would be derived from the directory listing it is then
//     compared against, so it cannot disagree with it. Every other decision in
//     this codebase treats a check that passes while inspecting nothing as the
//     cardinal failure — `diagrams:` matching zero files is a loud exit 2, a
//     `discover:` rule matching zero directories is an ORPHAN, codes switched
//     `off` are printed on the summary line. Manufacturing that state and
//     handing it to CI on day one would undo all of it.
//   - **Directories are not architecture.** A box per directory is `ls` with
//     rectangles. It carries no edges, and the README is explicit that edges —
//     who calls whom — are most of what a system diagram communicates. The
//     generated artifact would be least trustworthy in exactly the dimension
//     that matters most, while looking finished.
//   - **The contract shipped in the same command forbids it.** CONVENTIONS.md,
//     which `init` writes into the repo, says "do not invent nodes to make a
//     diagram look complete" and "edit by node ID, never regenerate the file".
//     Generating a whole diagram is a whole-file generation of invented nodes.
//     Shipping a contract and breaking it in the same command is the drift
//     surface this tool exists to close.
//   - **OVERVIEW defers generated diagrams to v2 by name.** Nodes-from-
//     directories is the degenerate case of exactly that feature.
//   - **The seeding already exists, at a better moment.** UNMAPPED's hint names
//     the binding line to paste — `# @bind svc_billing app/services/billing/**`
//     — for the specific directory the reader is looking at. `check` proposes a
//     node when the user is deciding whether that box exists. `init` would
//     propose it before they have thought about it, and a proposal in a
//     committed file reads as a decision.
//
// What that costs, stated plainly: **the first `trestle check` after `init`
// exits 1, not 0.** One UNMAPPED per discovered unit. You cannot put
// `trestle init && trestle check` in a script and expect success on day one,
// and the first diagram is authored by hand.
//
// That cost is bounded and it is signposted. The count is known before anything
// is written — `init` prints it, and the user confirms the rules that produce
// it — and every line of the resulting output names its own fix. What kills
// adoption is unexpected, unactionable noise. This is neither. It is the
// inventory `docs/DOGFOODING.md` describes: the first run is not a verdict.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/timimsms/trestle"
	"github.com/timimsms/trestle/internal/config"
	"github.com/timimsms/trestle/internal/walk"

	"github.com/timimsms/trestle/internal/lang"
)

// Action is what `init` intends to do with one file.
type Action string

// The four outcomes. Only Create and Append write anything; the other two exist
// so that a second run can say what it left alone rather than saying nothing.
const (
	// Create writes a file that is not there.
	Create Action = "create"
	// Append adds a stanza to the end of a file that is.
	Append Action = "append"
	// Keep leaves an existing file exactly as it is. This is the idempotency
	// rule: `init` run twice never clobbers, and a customized `.trestle.yml`
	// outranks anything this package would have written.
	Keep Action = "keep"
	// Unchanged means the file is already byte-identical to what would be
	// written, or already carries the stanza.
	Unchanged Action = "unchanged"
)

// Artifact is one file in the plan.
type Artifact struct {
	// Path is repo-relative and slash-separated.
	Path string
	// Action is what will happen to it.
	Action Action
	// Payload is the bytes to write: the whole file for Create, the stanza to
	// append for Append, and nothing for Keep or Unchanged.
	Payload string
	// Why explains a Keep or an Unchanged in one clause. A skipped file with no
	// reason attached is how a user concludes the command silently did nothing.
	Why string
}

// Writes reports whether the artifact would put bytes on disk.
func (a Artifact) Writes() bool { return a.Action == Create || a.Action == Append }

// Plan is everything `init` proposes for one repo: the `discover:` rules it
// detected and the files it would write. Nothing in it has touched the disk.
type Plan struct {
	// Root is the absolute directory that will become the repo root.
	Root string
	// Rules holds the proposed `discover:` rules with what each matches today.
	Rules []Rule
	// Langs holds the ecosystems detected from marker files. It decides which
	// shapes were proposed above, and which test-file globs the emitted config
	// offers — a Go repo should not be told to exclude `**/*_spec.rb`.
	Langs []lang.Lang
	// Existing holds the `discover:` rules the repo's config already has. It is
	// non-nil only when there was a config to keep, and it is what turns a
	// second `init` from a no-op into an answer: run it again after adding a
	// `packages/` directory and it says which shape your config does not cover
	// yet, without touching the config to say so.
	Existing []string
	// Artifacts holds every file considered, in the order to report them —
	// including the ones that will be left alone.
	Artifacts []Artifact
}

// Writes counts the artifacts that would put bytes on disk. Zero means the repo
// is already initialized and there is nothing to confirm.
func (p *Plan) Writes() int {
	n := 0
	for _, a := range p.Artifacts {
		if a.Writes() {
			n++
		}
	}
	return n
}

// Units counts the directories the proposed rules match — which is also the
// number of UNMAPPED violations the first `trestle check` will report, since a
// fresh diagram claims none of them.
func (p *Plan) Units() int {
	n := 0
	for _, r := range p.Rules {
		n += len(r.Units)
	}
	return n
}

// NestedRootError reports that the target directory is already inside a repo
// Trestle has been initialized in.
//
// This is a refusal rather than a warning because the damage is quiet: a second
// `.trestle.yml` in a subdirectory creates a second root, and every path in
// both configs resolves against a different directory than its author expects.
// The first command to notice would be a `check` reporting ORPHAN on bindings
// that are perfectly correct.
type NestedRootError struct {
	// Existing is the absolute path of the config that was found.
	Existing string
	// Target is the directory `init` was asked to initialize.
	Target string
}

func (e *NestedRootError) Error() string {
	return fmt.Sprintf(
		"%s already exists and would make %s a second, nested repo root\n"+
			"  hint: run `trestle init` from %s, or `trestle check` if this repo is already set up",
		e.Existing, e.Target, filepath.Dir(e.Existing))
}

// Prepare inspects root and returns the plan, without writing anything.
//
// It performs the one filesystem walk `init` needs — `run.Load` is not usable
// here, because that pipeline starts by loading the config this command exists
// to create — and applies the default excludes, so `node_modules` and `vendor`
// cannot turn into proposed `discover:` rules.
func Prepare(root string) (*Plan, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", root, err)
	}

	// A config in a *parent* is a different situation from a config here: one
	// is a repo that is already set up, the other is about to become a nested
	// root nobody will diagnose correctly.
	if found, err := config.Find(abs); err == nil && filepath.Dir(found) != abs {
		return nil, &NestedRootError{Existing: found, Target: abs}
	}

	listing, err := walk.Walk(walk.Options{Root: abs, Exclude: config.DefaultExclude()})
	if err != nil {
		return nil, err
	}

	p := &Plan{Root: abs, Rules: Detect(listing), Langs: DetectLangs(listing)}

	// The config is the pivot. If the repo already has one it is authoritative,
	// and the starter diagram — which exists only to keep the config `init`
	// writes from being a config with nothing to check — has no reason to be
	// written either. Whatever `diagrams:` that config names is its author's
	// business.
	cfgWritten := false
	switch exists, err := fileExists(abs, ConfigPath); {
	case err != nil:
		return nil, err
	case exists:
		p.Artifacts = append(p.Artifacts, Artifact{
			Path: ConfigPath, Action: Keep,
			Why: "this repo is already configured; `init` never rewrites a config it did not write",
		})
		// A config that does not load is not this command's problem to report —
		// `check` says so precisely, with line numbers. Here it just means the
		// comparison cannot be made.
		if cfg, err := config.Load(filepath.Join(abs, ConfigPath)); err == nil {
			p.Existing = cfg.Discover
			if p.Existing == nil {
				p.Existing = []string{}
			}
		}
	default:
		cfgWritten = true
		p.Artifacts = append(p.Artifacts, Artifact{
			Path: ConfigPath, Action: Create, Payload: configFile(p.Rules, p.Langs),
		})
	}

	p.Artifacts = append(p.Artifacts, diagramArtifact(listing, cfgWritten))

	conventions, err := conventionsArtifact(abs)
	if err != nil {
		return nil, err
	}
	p.Artifacts = append(p.Artifacts, conventions)

	agents, err := agentsArtifact(abs)
	if err != nil {
		return nil, err
	}
	p.Artifacts = append(p.Artifacts, agents)

	return p, nil
}

func diagramArtifact(listing *walk.Listing, cfgWritten bool) Artifact {
	if !cfgWritten {
		return Artifact{
			Path: DiagramPath, Action: Keep,
			Why: "the existing `.trestle.yml` names the diagrams to check",
		}
	}
	// Any diagram already under the scaffolded glob means this repo has started
	// drawing. Adding an empty one next to it would be noise, and it would show
	// up as a second file in every `check` header.
	for _, e := range listing.Entries {
		if e.IsDir {
			continue
		}
		if ok, err := doublestar.Match("docs/architecture/*.d2", e.Path); err == nil && ok {
			return Artifact{
				Path: e.Path, Action: Keep,
				Why: "docs/architecture/ already has a diagram",
			}
		}
	}
	return Artifact{Path: DiagramPath, Action: Create, Payload: diagramFile}
}

func conventionsArtifact(root string) (Artifact, error) {
	src, err := readIfExists(root, ConventionsPath)
	switch {
	case err != nil:
		return Artifact{}, err
	case src == nil:
		return Artifact{Path: ConventionsPath, Action: Create, Payload: trestle.Conventions}, nil
	case string(src) == trestle.Conventions:
		return Artifact{Path: ConventionsPath, Action: Unchanged, Why: "already the copy this binary ships"}, nil
	default:
		return Artifact{
			Path: ConventionsPath, Action: Keep,
			Why: "yours differs from the copy this binary ships; it is your file now",
		}, nil
	}
}

func agentsArtifact(root string) (Artifact, error) {
	src, err := readIfExists(root, AgentsPath)
	switch {
	case err != nil:
		return Artifact{}, err
	case src == nil:
		return Artifact{Path: AgentsPath, Action: Create, Payload: agentsStanza}, nil
	case strings.Contains(string(src), agentsMarker):
		return Artifact{Path: AgentsPath, Action: Unchanged, Why: "the stanza is already there"}, nil
	}
	// Appended, never rewritten: a target repo's AGENTS.md is somebody else's
	// document, and the instructions already in it are load-bearing for whoever
	// wrote them.
	sep := "\n"
	if !strings.HasSuffix(string(src), "\n") {
		sep = "\n\n"
	}
	return Artifact{Path: AgentsPath, Action: Append, Payload: sep + agentsStanza}, nil
}

// Apply writes the plan. It creates with O_EXCL and appends with O_APPEND, so
// the "never clobber" rule is enforced by the open flags rather than by the
// check Prepare did a moment earlier — the two are separated by however long
// the user took to answer the prompt.
func (p *Plan) Apply() error {
	for _, a := range p.Artifacts {
		if !a.Writes() {
			continue
		}
		abs := filepath.Join(p.Root, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(a.Path), err)
		}

		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if a.Action == Append {
			flags = os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(abs, flags, 0o644)
		if err != nil {
			return fmt.Errorf("write %s: %w", a.Path, err)
		}
		if _, err := f.WriteString(a.Payload); err != nil {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", a.Path, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("write %s: %w", a.Path, err)
		}
	}
	return nil
}

func fileExists(root, rel string) (bool, error) {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	switch {
	case err == nil:
		return !info.IsDir(), nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", rel, err)
	}
}

// readIfExists returns nil bytes and no error when the file is absent.
func readIfExists(root, rel string) ([]byte, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	switch {
	case err == nil:
		return src, nil
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
}
