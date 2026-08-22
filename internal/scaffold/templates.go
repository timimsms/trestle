package scaffold

import (
	"fmt"
	"strings"

	"github.com/timimsms/trestle/internal/lang"
)

// The three files `init` authors. CONVENTIONS.md is not here: it is embedded
// verbatim from the repo root (see the root `trestle` package) so there is
// exactly one copy of the contract in existence.
//
// Everything below is written as documentation, because that is what it turns
// out to be. A dogfooding trial put an agent that had never heard of Trestle in
// front of a repo that had adopted it; the agent added a node and its binding
// correctly, and when asked why, cited the comment above `discover:`. The
// comments in the emitted config are the primary way this convention travels.
// They are not filler and they are not for the person who ran `init`.
const (
	// ConfigPath is where the config must live: the directory containing it is
	// the repo root, and every path in the system resolves against it.
	ConfigPath = ".trestle.yml"
	// DiagramPath is the starter diagram. It is inside the directory the
	// scaffolded `diagrams:` glob covers, which is what keeps `trestle check`
	// from being a tool error on the first run.
	DiagramPath = "docs/architecture/system.d2"
	// ConventionsPath is the agent contract, written verbatim from the copy
	// embedded in the binary.
	ConventionsPath = "CONVENTIONS.md"
	// AgentsPath is the file coding agents read first. `init` appends to it and
	// never rewrites it: a target repo's AGENTS.md is somebody else's document.
	AgentsPath = "AGENTS.md"
)

// RenderOut is the scaffolded `render.out`. It is set rather than left empty so
// that `trestle render` works on the first run instead of exiting 2 with a
// message about a block the user has never heard of.
const RenderOut = "docs/architecture/rendered/"

// agentsMarker identifies a stanza this tool wrote. Detection is by marker
// rather than by heading text so that editing the prose — which the repo owns
// once it is written — does not cause a second stanza to be appended.
const agentsMarker = "<!-- trestle -->"

// configFile renders `.trestle.yml` for a set of proposed rules.
// testGlobComment offers the exclude patterns for the ecosystems this repo
// uses, commented out.
//
// Offered rather than applied, because excluding tests is a real decision with
// a real cost — and at least one of these is a trap. On a Go repo,
// uncommenting `**/*_test.go` can *increase* failures: an external test package
// (`package foo_test`) may be the only file in its directory, so excluding it
// turns a healthy unit into an empty one. Real repo, 7 failures to 9.
func testGlobComment(langs []lang.Lang) string {
	var b strings.Builder
	b.WriteString("  # Test files usually belong here. Uncomment the pattern your repo uses,\n")
	b.WriteString("  # but read what it removes first — excluding the only file in a directory\n")
	b.WriteString("  # turns a unit Trestle was watching into one it reports as empty.\n")
	for _, l := range langs {
		for _, g := range l.TestGlobs {
			b.WriteString("  # - \"" + g + "\"\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func configFile(rules []Rule, langs []lang.Lang) string {
	var b strings.Builder

	b.WriteString(`# Trestle configuration.
#
# Trestle keeps this repo's architecture diagrams honest: every box on a diagram
# declares the code behind it, and ` + "`trestle check`" + ` fails when a binding points
# at code that is gone, or when code appears that no box owns.
#
# The comments below are the documentation. Read one before you change the rule
# under it, and keep it when you do.
version: 1

# The diagrams to check. Every ` + "`.d2`" + ` matched here is parsed for the binding
# directives in its comments: @bind, @external, @infra and @ignore.
#
# A ` + "`diagrams:`" + ` glob that matches no file is a tool error rather than a quiet
# pass, because a check with nothing to check would otherwise exit 0 having
# inspected nothing.
diagrams:
  - docs/architecture/*.d2

# Every directory matched here needs an owner: a ` + "`@bind`" + ` glob on some diagram
# that claims at least one file inside it, or an entry in ` + "`shared:`" + ` below. A
# directory with neither is reported as UNMAPPED. This is the half of Trestle
# that catches *new* code — a subsystem that appeared and never made it onto the
# diagram — and it is the half that decides whether running the tool is worth it.
#
# Seeded at depth 2: one level inside a container directory that exists in this
# repo. Spike 01 measured the depth on a 4,000-file repo. Depth 2 produced units
# that correspond to boxes somebody would actually draw; depth 3 fragmented the
# same repo into 118 units with 35 of them holding two files or fewer, which is
# authoring burden rather than architecture.
#
# The trailing slash is load-bearing. These rules match directories, and without
# it the rule matches nothing at all — at which point UNMAPPED stops firing and
# the check goes green while seeing nothing.
`)

	if len(rules) == 0 {
		b.WriteString(`#
# Nothing was detected here, so this list is empty and UNMAPPED will never fire.
# Add the directory that holds your subsystems, e.g.
#
#   discover:
#     - app/services/*/
discover: []
`)
	} else {
		b.WriteString("discover:\n")
		for _, r := range rules {
			fmt.Fprintf(&b, "  - %s\n", r.Glob)
		}
	}

	b.WriteString(`
# Real code that is deliberately owned by no node: the plumbing every service
# uses. An entry here suppresses UNMAPPED for what it matches, and is itself
# checked — a ` + "`shared:`" + ` entry pointing at a path that no longer exists fails
# the build like any other stale binding. That is the difference between this
# and ` + "`exclude:`" + `, which is a blindspot by design.
#
# Left empty on purpose. ` + "`init`" + ` cannot fill this in and be right: entries must
# be enumerated, never blanket, because ` + "`lib/**`" + ` would exempt a future
# ` + "`lib/dispatch_engine/`" + ` — real architectural weight, silently waved through.
# Trestle rejects a blanket entry for that reason.
#
# The honest way to fill it in is to run ` + "`trestle check`" + `, read the UNMAPPED
# list, and triage each entry with one question: would you mention it by name in
# an architecture review?
#
#   yes -> it is a node. Put it on a diagram with a @bind directive.
#   no  -> it is plumbing. Enumerate it here:
#
`)
	fmt.Fprintf(&b, "#            shared:\n#              - %s\n#              - %s\n",
		sharedExample(rules, 0), sharedExample(rules, 1))
	b.WriteString(`shared: []

# Not architecturally real: tests, vendored code, generated output. Anything
# matched here is invisible to every check, including to ` + "`diagrams:`" + `. Reach for
# this list only when the answer to "is this part of the architecture?" is no —
# using it to quiet a finding is how a blindspot becomes permanent.
exclude:
  - "**/.git"
  - "**/node_modules"
  - "**/vendor"
` + testGlobComment(langs) + `

# UNBOUND is a node with no directive of any kind. It warns rather than fails
# because it is usually a modeling gap — a box that is genuinely not code — and
# failing on it out of the box trains people to suppress it. Run
# ` + "`trestle check --strict`" + ` in CI to treat warnings as failures.
#
# The other four codes — ORPHAN, UNMAPPED, DANGLING, SYNTAX — fail by default.
# There are five codes and there will not be a sixth.
severity:
  UNBOUND: warn

# Where ` + "`trestle render`" + ` writes SVGs. They are generated artifacts: add this
# directory to .gitignore and edit the .d2, never the SVG.
render:
  out: ` + RenderOut + `
`)
	return b.String()
}

// sharedExample picks the i-th plausible `shared:` entry to show in the comment
// above the empty list. Concrete beats abstract: if the repo has a `lib/`, the
// example names two directories that are actually in it, so the reader can see
// the shape against something they recognize.
func sharedExample(rules []Rule, i int) string {
	fallback := []string{"lib/http_client/**", "lib/logging/**"}
	for _, r := range rules {
		if !strings.HasPrefix(r.Glob, "lib/") {
			continue
		}
		if i < len(r.Units) {
			return r.Units[i].Path + "/**"
		}
	}
	return fallback[i]
}

// diagramFile is the starter diagram: comments and nothing else.
//
// The decision to write no nodes is recorded in the package doc. What is worth
// recording here is a constraint the file has to work around: **a directive
// cannot be commented out.** The scanner strips the leading run of `#` before
// looking for `@`, so `## @bind svc_x app/x/**` parses as a live binding, and a
// commented-out example in this file would immediately be DANGLING against a
// node that does not exist.
//
// So every example below keeps the directive off the start of its line. That is
// not a stylistic choice, it is the only form that works, and the same trap is
// waiting for anyone who tries to temporarily disable a binding — which is why
// CONVENTIONS.md now says so.
const diagramFile = `# System architecture.
#
# ` + "`trestle init`" + ` wrote this file with no nodes in it, deliberately.
#
# It is a starting point to edit, not a description of your architecture.
# Trestle will not invent boxes for you: a diagram generated from your directory
# listing would pass its own check on the first run — Trestle having written
# both sides of the comparison — while telling you nothing. And the edges, which
# are most of what a system diagram communicates, cannot be guessed at all.
#
# Start by running ` + "`trestle check`" + `. Every directory matched by a ` + "`discover:`" + `
# rule in .trestle.yml that no node claims is reported as UNMAPPED, and each of
# those reports carries the exact binding line to paste. That list is the to-do
# list for this file. It is an inventory, not a verdict.
#
# A node is one line, and its binding is a comment that owns its own line. The
# two belong in the same edit:
#
#   the binding:  # @bind svc_billing app/services/billing/**
#   the node:     svc_billing: Billing Service
#
# Not everything is code. A queue or a database you run is ` + "`# @infra db_primary`" + `;
# somebody else's API is ` + "`# @external ext_stripe`" + `.
#
# Node IDs must be real code identifiers: snake_case, greppable in this repo,
# prefixed by kind (svc_, db_, queue_, ext_, job_). The label after the colon
# carries the human-readable name. CONVENTIONS.md is the contract and explains
# why that rule earns its place.
#
# Note while editing: a leading # does not disable a directive. The scanner
# strips them, so a "commented-out" binding is still live. Delete it instead.
#
# Delete this block once the diagram has something in it.
`

// agentsStanza is appended to the target repo's AGENTS.md.
//
// It is short on purpose. An AGENTS.md that already exists belongs to the repo,
// and a tool that appends two screens of its own documentation to it will be
// deleted from that file within a week. Everything it does say points at
// something runnable.
const agentsStanza = agentsMarker + `
## Architecture diagrams

` + "`docs/architecture/*.d2`" + ` is checked by [Trestle](https://github.com/timimsms/trestle):
every box on a diagram declares the code behind it, and ` + "`trestle check`" + ` fails when a
binding points at code that is gone, or when code appears that no box owns.

1. **Read ` + "`CONVENTIONS.md`" + ` before editing a ` + "`.d2`" + `.** It is the contract for these
   files, not background reading.
2. **Run ` + "`trestle explain`" + ` first.** It lists every node the tool parsed and what each
   binding matches right now; ` + "`trestle explain <node_id>`" + ` drills into one, and
   ` + "`--format=json`" + ` is the shape to read if you are a program. Editing a diagram you
   have not looked at through the tool is how bindings rot.
3. **Adding a node? Add its binding in the same edit.** A node with no binding is
   incomplete work, not a follow-up task.
4. **Edit by node ID. Never regenerate the file.** A whole-file rewrite produces a diff
   nobody will review, which defeats the point of a text format.
5. **Run ` + "`trestle check`" + ` before declaring done.** Exit 0, or say why not. Do not reach
   green by deleting a ` + "`discover:`" + ` rule, widening a glob until the complaint
   disappears, or adding ` + "`@ignore`" + ` with a hollow reason — a green check nobody can
   trust is worse than a failing one.

### Writing the first diagram

If ` + "`docs/architecture/system.d2`" + ` is still empty, this is the job. Trestle deliberately
does not generate it: a diagram derived from the directory listing cannot disagree with
that listing, so it would pass its own check while telling nobody anything.

1. **Get the inventory.** ` + "`trestle check --format=json`" + ` — every ` + "`UNMAPPED`" + ` is a
   directory that needs an owner, and each one already carries the exact ` + "`@bind`" + `
   line that would claim it.
2. **Group units into boxes.** One box per thing a person would name out loud in an
   architecture review. Two directories that are one subsystem get one node with two
   ` + "`@bind`" + ` lines — bindings are repeatable and OR together. Plumbing that nobody
   would name goes in ` + "`shared:`" + `, not on the canvas.
3. **Ask about the edges. Do not infer them.** This is the step that matters most and
   the one you are most likely to get wrong. Import graphs tell you what calls what,
   not what the architecture *means* — and ` + "`trestle check`" + ` cannot verify a single
   edge, so nothing downstream will catch a wrong one. **A missing edge is a gap; a
   confident wrong edge is a lie the tool will never contradict.** Ask the human which
   boxes talk to which, and what each arrow carries.
4. **Bind every node as you add it**, and give infrastructure ` + "`@infra`" + ` and
   third-party systems ` + "`@external`" + ` rather than leaving them bare.
5. **Run ` + "`trestle check`" + ` until it is clean**, then ` + "`trestle render`" + ` and look at
   the result. If it is unreadable it usually has too many boxes, not bad layout.

Stop at one diagram that answers one question. A diagram answering three answers
none of them well.
`
