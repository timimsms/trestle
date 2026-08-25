# Trestle — Overview

**Working name:** Trestle (a structure that holds up a span). CLI binary: `trestle`.
**One line:** Keeps architecture diagrams honest by binding diagram nodes to real paths in the repo and failing CI when they diverge.

---

## The problem being solved

Architecture diagrams drift. Not because people are careless, but because nothing connects the diagram to the thing it describes. A service gets renamed, a module gets deleted, a new subsystem appears — and the diagram stays exactly as correct-looking as it was the day it was drawn.

Diagram-as-code (D2, Mermaid, Structurizr) fixes *reviewability* — you can see the change in a PR. It does not fix *truth*. A D2 file can be perfectly version-controlled and completely wrong.

Trestle adds the missing edge: a declared, checkable binding between a node in the diagram and a path in the codebase.

## What Trestle is not

These are deliberate exclusions, not backlog items.

| Not | Why |
| --- | --- |
| A diagram editor | `d2 --watch` + an agent already covers authoring. Competing here is a losing race. |
| A WYSIWYG / drag-drop surface | Layout is owned by the layout engine. This trade was accepted upfront; reintroducing manual positioning re-breaks agent editability. |
| A rendering engine | D2 renders. Trestle embeds it and gets out of the way. |
| A hosted or multi-user product | Local-first CLI on a repo. No accounts, no server, no realtime. |
| A model/view system (Structurizr-style) | Genuinely valuable, explicitly deferred. See *Deferred* below. |
| A history/versioning layer | Git. |

## Success criterion

> `trestle check` fails on a real PR, at least once in the first month, for a reason that was not anticipated when the bindings were written.

If it never fires, it is decoration and should be deleted. This is the primary MVP evaluation gate, and it is deliberately falsifiable.

---

## Decision ledger

### Locked

| # | Decision | Rationale |
| --- | --- | --- |
| L1 | **Local-first CLI operating on a repo.** No hosted component. | Ratified default from scoping. Team surface would invert the architecture; revisit only as a separate product. |
| L2 | **D2 is the diagram format.** Not Mermaid, not Structurizr DSL. | Best available balance of human-authorability and agent-editability with real layout control. |
| L3 | **Bindings live as magic comments inside the `.d2` file**, not in a sidecar file. | A sidecar bindings file is itself a drift surface. Co-location means one file to edit and one thing for an agent to keep consistent. |
| L4 | **Bindings are path globs, not code symbols.** | Language-agnostic, no LSP dependency, near-zero cost. Correctness of this choice is the subject of Spike 01. |
| L5 | **Node IDs must correspond to real code identifiers**, with display labels carried separately. | Established in scoping as the single highest-leverage practice for agent comprehension. Enforced by convention + lint, not by type system. |
| L6 | **`trestle check` returns a CI-meaningful exit code.** | The check is worthless if it only ever runs interactively. |
| L7 | **Agents edit by node ID, never by whole-file regeneration.** | Preserves reviewable diffs. Enforced via the shipped agent contract, not technically preventable. |
| L8 | **Go, with D2 embedded as a library.** | Single static binary, no `d2` install prerequisite, no runtime. See TECH_STACK.md for the escape hatch. |
| L9 | **`@infra` is a distinct directive from `@external`.** | Surfaced by the worked example. `@external` means third-party; a database or queue you own is neither third-party nor code-backed. Collapsing them would have forced a lie into every diagram with a Postgres box. |
| L10 | **Shared paths are declared in `.trestle.yml` under `shared:`, not as a diagram directive.** | "This code is shared" is a fact about repo layout, true across every diagram. As a directive it would have to be repeated in each `.d2` file, or arbitrarily assigned to one. |
| L11 | **`shared:` entries must be enumerated, never blanket.** | `lib/**` would swallow a future `lib/dispatch_engine/` — real architectural weight, silently exempted. Enumeration means new shared subsystems still fire `UNMAPPED`. Entries are ORPHAN-checked so the list can't rot. |
| L12 | **Overlapping bindings are legal and get no violation code.** | Two nodes may honestly share a directory. Surfaced via `trestle explain --overlaps`. Keeps the taxonomy at five, which is the number people will actually learn. |

### Open

| # | Question | Needed by | Notes |
| --- | --- | --- | --- |
| ~~O1~~ | ~~Is glob-level binding granular enough to catch real drift?~~ | **RESOLVED** | **Proceed, with a caveat.** See amendment below. |
| O2 | What is the discovery rule for `UNMAPPED`? | Day 4 | Per-repo convention (`app/services/*/`) vs. heuristic. Leaning explicit config — heuristics will generate noise and noise kills adoption. |
| ~~O3~~ | ~~Should unbound nodes warn or fail by default?~~ | **RESOLVED** | **Warn.** In the worked example `UNBOUND` fired on a queue node — not an error, a modeling gap I hadn't considered. That is prompt-shaped, not failure-shaped. Fail would have trained a suppression reflex on the first diagram written. |
| ~~O6~~ | ~~Cross-cutting code with no single owning node.~~ | **RESOLVED** | `shared:` in config, enumerated. See L10–L12. |
| O7 | Does enumerated `shared:` stay practical at scale? | Day 4 | L11 assumes a repo has ~5–20 shared subsystems. Unmeasured. If a real repo needs 50+ entries, enumeration is unusable and L11 needs revisiting — though 50 shared subsystems is itself a finding about the codebase, not just about Trestle. Count `lib/*/` and `app/middleware/*/` before day 4. |
| O4 | Monorepo behavior — one binding namespace or many? | Post-MVP | Deferred unless Spike 01 surfaces it as immediate. |
| O5 | Does the preview pane justify its build cost in v1? | Day 3 | `d2 --watch` already renders. Trestle's preview only earns its place if it overlays check status onto the diagram. If that overlay slips, cut the pane entirely. |

### Amendment — O1 verdict (Spike 01)

**Date:** 2026-08-16 · **Run by:** build agent, at the repo owner's direction · **Probe:** `spike/glob-binding-probe.sh`

Two repos were probed. Trestle itself returned zero at every depth — it is a docs-only repo with
one commit, so it measures nothing and is reported here only to say it was tried. The real run was
against a private monorepo (3,172 commits, 4,007 files, all inside the window) — the one repo to hand under
enough structural change to produce signal.

| Depth | Units today (at start) | Q2 orphan | Q3 silent | Q4 new | Signal |
| --- | --- | --- | --- | --- | --- |
| 1 | 20 (5) | 1 | **0** | 16 | 17 |
| 2 | 64 (6) | 1 | **0** | 59 | 60 |
| 3 | 118 (13) | 0 | **0** | 105 | 105 |

**Verdict: PROCEED.** The falsification criterion that would have voided the design — `Q3 > Q2`,
silent gutting outweighing detectable drift — did not fire at any depth. Q3 was **zero
everywhere**: no unit in this repo lost ≥70% of its files while surviving. The false-negative
mode that kills the tool is not present here.

**Caveat, recorded so it is not lost.** The signal is Q4-dominated, and Q4 is inflated: the repo
had 5–6 units at the window start versus 20–64 today, so most of that count is a young repo
growing rather than an established architecture drifting. Q2 — the clean-deletion signal — is
genuinely thin (1, 1, 0). Read this as *"globs are not too coarse"* (a strong result, Q3=0),
not as *"drift is rampant and the check will constantly fire"* (unproven). The OVERVIEW success
criterion stays genuinely at risk and remains the MVP's evaluation gate.

**Depth 2 is the right unit depth** for a monorepo of this shape — `ui/src`, `server/src`,
`packages/db`, `packages/adapters`, `cli/src` are the boxes you would actually draw. Depth 3
fragments into 118 units with 35 of them holding ≤2 files; that is authoring burden, not
architecture. Seed `discover:` at depth 2 in `trestle init`.

**Bears on O7.** Depth 2 yields 64 units in a 4k-file repo. If even a fifth of those are shared
plumbing, the enumerated `shared:` list runs to ~13 entries — inside L11's assumed 5–20 range,
so L11 survives, but only just. Re-check O7 against a second repo before v1.

### Amendment — Gate B verdict (D2 AST surface)

**Date:** 2026-08-16 · **d2 version:** `oss.terrastruct.com/d2 v0.7.2`

**PASS — outcome 1: public AST, container paths recoverable.** `d2compiler.Compile` is public
and stable-looking; walking `g.Root.ChildrenArray` recursively and reading `Object.AbsID()`
recovers all **12** node IDs from `examples/repairs-platform/system.d2` with container
qualification intact (`platform.svc_work_orders`, not `svc_work_orders`). Shapes, labels and all
10 edges come along for free. No regex fallback needed; no D2 grammar fork.

The probe surfaced **two spec gaps that Task 2 must resolve before the check engine is written**:

- **New O8 — qualified vs. unqualified node IDs.** The AST yields `platform.svc_work_orders`.
  Every directive in the worked example says `@bind svc_work_orders`. Under strict string
  matching, *all six binds in the shipped example are `DANGLING`* and `platform` is `UNBOUND`.
  Either the example is wrong or matching must resolve an unqualified ID against a unique
  suffix. Resolved in GAMEPLAN as: **suffix match, ambiguity is `SYNTAX`.**
- **New O9 — are containers nodes?** `platform` is a node in the AST with no directive, so it
  fires `UNBOUND`, as does `tenant`. A container that groups five bound services is a grouping
  device, not an unowned subsystem. Resolved in GAMEPLAN as: **a container whose descendants are
  all accounted for is itself accounted for.** No new violation code; L-taxonomy stays at five.

### Amendment — first dogfood result

**Date:** 2026-08-20 · **Target:** Trestle itself, first non-fixture config in existence

Trestle was pointed at its own repo the moment `check` worked. The first run produced four
findings from a config written to describe the repo honestly rather than to pass:

| Finding | Verdict |
| --- | --- |
| `UNMAPPED internal/render/` | **True positive.** An empty package directory left over from the Phase 0 scaffold. Git does not track empty directories, so it was invisible to every other tool in the repo. Deleted. |
| `UNBOUND author`, `UNBOUND repo` | **Correct, prompt-shaped.** Two diagram nodes that are genuinely not code. A modeling gap, not an error — exactly what O3 predicted when it resolved `UNBOUND` to *warn*. Fixed with `@external`. |
| `UNBOUND engine.run.the only place that orchestrates I/O` | **The interesting one — see below.** |

That third node does not exist in the source. It exists because a tooltip read
`tooltip: the pipeline; the only place that orchestrates I/O`, and **`;` is a statement
separator in D2**. The compiler split the line and turned the trailing prose into a child node.
The diagram rendered without complaint; the phantom node was invisible to review; the only reason
anyone found out is that Trestle asked what code backed it.

**This is the success criterion firing, on the first non-fixture run.** OVERVIEW asks for a
failure "for a reason that was not anticipated when the bindings were written," and nobody
writing those bindings anticipated D2 statement-separator semantics inside a tooltip. The tool
found a real defect in a real diagram that a human reviewer would not have caught, and it did so
by asking a question — *what code is this?* — that no linter or renderer asks.

Two caveats, so this is not over-read:

1. **Sample size one, and the repo is the tool's own.** One catch is not a track record. The
   Gate A caveat stands and the criterion says *"on a real PR, in the first month"* — Trestle
   still needs to run somewhere it was not designed alongside.
2. **The catch was UNBOUND, a warning.** Under default severity this finding does not fail a
   build. `make self-check` runs `--strict` for exactly this reason.

`.trestle.yml` and `docs/architecture/system.d2` now live in this repo and `make self-check` runs
in CI as a required job. If Trestle will not hold itself to the standard it recommends, it has no
business recommending it.


---

## Resolutions

The ledger above fixes *what* Trestle is. These fix *how it behaves* in the cases the ledger
left open. They are numbered O8 onward, continuing the open-question sequence, and none of
them added a violation code or a command.


These are the semantics the ledger did not pin down, and the implementation had to. Each was
found by building or by running the tool against a real repository, and each is recorded with
the evidence that forced it — a resolution whose reason is lost gets re-litigated.

### O8 — qualified vs. unqualified node IDs

The AST yields `platform.svc_work_orders`. Every directive in the shipped worked example says
`@bind svc_work_orders`. Under strict string equality, **all six binds in the example are
`DANGLING` and the example fails its own check.**

**Resolution: suffix matching, with ambiguity as an error.**

A directive's node ID matches an AST node if it is equal to that node's `AbsID()`, **or** if it is
a dot-delimited *suffix* of it on a segment boundary. `svc_work_orders` matches
`platform.svc_work_orders`. `orders` does **not** match `platform.svc_work_orders` — segment
boundary, not substring.

If an unqualified ID matches more than one node, that is `SYNTAX`, not a silent pick. The hint
names the candidates and tells the author to qualify. This keeps the common case ergonomic — you
should not have to restate the container in every directive — without letting a rename quietly
re-point a binding at the wrong node.

Fully-qualified IDs always win over suffix candidates when both are present.

### O9 — are containers nodes?

`platform` is a node in the AST. It has no directive, so it fires `UNBOUND` — as does `tenant`.
In the worked example that means the reference diagram ships with two warnings out of the box,
which is exactly the noise that trains a suppression reflex.

**Resolution: a container whose descendants are all accounted for is itself accounted for.**

`UNBOUND` is suppressed for any node with children where every descendant has a `@bind`,
`@external`, `@infra`, or `@ignore`. A container is a grouping device; grouping five bound
services is not an unowned subsystem. A container with an unaccounted-for descendant still
warns — on the descendant, not the container.

A container **may** carry its own `@bind`, and if it does the binding is checked normally.
Leaf nodes are unaffected: `tenant` has no children and no directive, so it correctly warns.

**No new violation code.** The taxonomy stays at five, per the standing constraint.

### O10 — what "covered" means for `UNMAPPED`

`discover: app/services/*/` matches a **directory**. `@bind svc_billing app/services/billing/**`
is authored to match **files**. DESIGN never says how one is tested against the other, and every
fixture depends on the answer.

**Resolution: a discover unit is covered when at least one non-excluded file beneath it is matched
by some `@bind` glob.** Not "does a bind glob match the unit's own path."

The path-based reading looks equivalent and is not. `@bind svc_billing app/services/billing/*.rb`
matches every file in the directory but does not match `app/services/billing/` itself — under a
path-based rule that directory is `UNMAPPED` while being plainly, correctly bound. That is a
false positive on ordinary authoring, and false positives are what get a check `--no-verify`'d.

Corollary: an **empty** discover unit can never be covered and will always fire `UNMAPPED`. This
is correct — an empty service directory is a real finding — but it should carry a distinguishing
hint rather than the generic "add a `@bind`" one.

**Amended after the field trials — placeholder files are not code.** The corollary above was
reasoned about a state git cannot store: a directory with no files is not committable, so the only
shape it takes in a real repo is a directory holding `.keep` or `.gitkeep`. Two field trials found
both halves of that mattering:

- A node bound to a directory holding only `.keep` reported `matches 1 file` and **passed** — a box
  claiming a service that does not exist, with `explain` confirming "no violations". That is a
  silent green of the same family as `severity: off` and a zero-match `diagrams:`.
- A Go repo had **7 of 15 packages declared and not yet written**, each holding a `.gitkeep`, and
  no honest resolution available: `@bind` makes a box backed by a placeholder, `shared:` calls a
  `.gitkeep` real code, and `exclude:` guarantees the check stays quiet on the day the code finally
  lands.

One rule answers both. A placeholder is not code, so:

| | before | after |
| --- | --- | --- |
| `@bind` matching only placeholders | passes | **`ORPHAN`** — it claims code that is not there |
| discover unit holding only placeholders | `UNMAPPED`, unresolvable | **silent** — no code, nothing to map |
| real code lands beside the placeholder | — | **`UNMAPPED`**, exactly as intended |

That is the signal a repo with declared-but-unbuilt packages actually wants, and it needed no new
directive, config key or violation code — which is why it is an amendment here rather than a
proposal to the ledger. A truly empty unit (untracked, working-tree only) keeps the old behavior
and its distinguishing hint.

It cannot be used to hide anything: silencing a real `UNMAPPED` this way would mean deleting the
code, which is a larger act than the check was ever going to prevent.

**Amended after Phase 3 — `shared:` confers coverage too.** The wording above said coverage comes
from `@bind`, which read as a contradiction of DESIGN §4's table. Checked against the table:
`shared` is explicitly marked ✅ *Suppresses UNMAPPED*. So O10's rule was incomplete, not wrong.
Corrected statement:

> A discover unit is covered when at least one non-excluded file beneath it is matched by some
> `@bind` glob **or some `shared:` entry**.

Without this, `shared: app/middleware/**` exempts code from ownership and then fires `UNMAPPED`
at the same code — the exemption and the complaint cancelling each other out. Coverage stays
file-level per O10; only the set of contributing globs widens.

**Also amended: a glob matching a directory claims the files beneath it.** `config` accepts a
bare `lib/pricing_engine` as a `shared:` entry, which under a files-only reading matches zero
files and fails `ORPHAN` on its first run. This is the same rule `walk` already applies to
`exclude:`, so it is consistency rather than a new concept.

### O11 — do invalid directives participate in the other checks?

Three questions that are one question in three costumes. Does a malformed directive still account
for its node for `UNBOUND`? Is a `DANGLING` directive's glob still `ORPHAN`-checked? Does an
ambiguous-suffix directive confer `discover` coverage?

**Resolution: an unresolvable directive participates in no other check.** A directive that is
`SYNTAX` or `DANGLING` is reported once and otherwise discarded — it does not bind, does not
account for its node, is not `ORPHAN`-checked, and confers no coverage.

One rule, three costumes, and it fails loudly rather than quietly. The alternative — half-using a
directive whose node ID or glob we could not trust — invents intent, and inventing intent in a
tool whose entire job is catching stale intent is the wrong instinct.

**Accepted consequence:** one rename can produce a `DANGLING` *and* an `UNMAPPED`. That reads as
piling on, but both statements are true — the directive is stale *and* the code is now unowned —
and the second is the one that costs money to miss. Revisit after dogfooding if it proves noisy;
this is the resolution most likely to be wrong, and it is cheap to reverse.

### Smaller resolutions taken during Phase 2

Recorded so they are decisions rather than accidents. Each is reversible; none adds a code.

- **Blanket `shared:` is defined as:** at most one literal leading segment followed by nothing but
  `*`/`**` segments. Rejects `lib/**`, `lib/*`, `**`, `**/*`; accepts `lib/http_client/**`,
  `app/*/middleware/**`, and bare `lib/pricing_engine`. L11 says "enumerated, never blanket"
  without saying where the line is; this is where it is. Belongs in DESIGN §4.
- **`diagrams:` is required** — a config without it makes `trestle check` a silent no-op that
  exits 0, which is precisely the "decoration" failure OVERVIEW says to delete the tool over.
  Loud exit 2 beats a silent pass. This is a fifth config validation the phase file did not list.
- **`@ignore ""` is `SYNTAX`.** An empty string is technically quoted and is still an unexplained
  suppression.
- **A directive must own its line.** `svc_billing: Billing # @bind ...` is ignored; `## @bind ...`
  *is* parsed, because silently dropping a real binding is worse than flagging an odd comment.
  **Consequence: there is no way to comment out a directive** — adding a `#` does not disable it,
  you must delete it. That is a real authoring gap and CONVENTIONS.md should say so, or a
  disable syntax should be designed. Flagged, not resolved.
- **`config` restates the five codes as strings** to validate `severity:` keys, because it cannot
  import `check`. That duplicates the closed taxonomy. Phase 3 must add a test pinning `check`'s
  codes to `config.Codes`; if they drift, `severity: {UNBOUND: warn}` silently stops applying.

### Smaller resolutions taken during Phase 3

Recorded so they are decisions rather than accidents. Each is reversible; none adds a code.

- **A zero-match `discover:` rule is reported as `ORPHAN`.** The phase file requires that it not be
  silent — it is the trailing-slash trap's calling card, and a discover rule matching nothing means
  `UNMAPPED` stops firing while the check still exits 0. It folds into `ORPHAN` (a declaration that
  matches nothing) rather than becoming a sixth code. It therefore *fails* by default, which is
  stronger than "config-level warning" and is deliberate: the failure it guards against is a green
  check that inspected no code.
- **`shared:` entries confer `discover` coverage.** O10 says a unit is covered when a `@bind` glob
  matches a file beneath it; DESIGN §4's table says `shared` suppresses `UNMAPPED`. Both are
  honored: coverage is file-level per O10, and the globs that contribute are `@bind` **and**
  `shared`. Otherwise `shared: app/middleware/**` would exempt code from ownership and then fire
  `UNMAPPED` at it anyway, which makes the declaration useless.
- **A glob that matches a directory claims every file beneath it.** `shared: lib/pricing_engine`
  (bare, no wildcard) is a form `config.blanketPrefix` explicitly accepts, and under a
  files-only reading it matches zero files and fails as `ORPHAN` on its first run. The rule is the
  same one `internal/walk` already applies to `exclude:`, where a bare `node_modules` prunes the
  subtree rather than one directory entry.
- **`Check` takes all diagrams at once, not one per call.** Node IDs are scoped per `.d2` file —
  merging two diagrams that both declare `svc_billing` would make every unqualified directive in
  both an ambiguous-suffix `SYNTAX` — but coverage is a fact about the repo, so a unit bound in
  `data-flow.d2` must not be `UNMAPPED` in `system.d2`'s run. `Input.Diagrams []Diagram` is the
  only shape that gets both right, and it keeps the per-diagram loop out of Phase 4 where the
  coverage bug would have been reinvented.
- **`@ignore` does not suppress `SYNTAX`.** It suppresses every violation for its node, but a
  malformed line's node token is exactly what O11 says cannot be trusted; honoring it would let one
  typo'd suppression hide the next syntax error. Nor does it suppress its own `DANGLING`.
- **`check.Entry` mirrors `walk.Entry` rather than importing it.** Importing `walk` for a two-field
  struct drags `io/fs` across the seam the I/O rule exists to defend.
  `integration.TestCheckEntryMirrorsWalkEntry` pins the shapes together so the copy cannot drift.

### Smaller resolutions taken during Phase 7 (`init`)

Recorded so they are decisions rather than accidents. Each is reversible; none adds a code or a
command.

- **The starter diagram is written empty.** The choice was between an empty canvas and one node
  per discovered unit with its binding pre-written. Seeded would make the first `trestle check`
  green; it was rejected because that green would be **manufactured** — Trestle comparing a
  diagram it derived from the directory listing against the same listing. Every other decision
  here treats a check that passes while inspecting nothing as the cardinal failure (`diagrams:`
  matching zero files is a loud exit 2; a `discover:` rule matching zero directories is an
  ORPHAN; codes set to `off` are printed on the summary line). A generated diagram also carries
  no edges, which the README says are most of what a diagram communicates, and CONVENTIONS.md —
  which `init` writes into the repo in the same breath — says "do not invent nodes to make a
  diagram look complete". OVERVIEW defers generated diagrams to v2 by name.
  **Accepted cost:** the first `trestle check` after `init` exits 1, one `UNMAPPED` per
  discovered unit, and `trestle init && trestle check` cannot be a green pipeline on day one.
  That cost is bounded, predicted out loud before anything is written, and every line of it
  carries the `@bind` that fixes it — UNMAPPED's hint already emits the exact binding, at the
  moment the reader is looking at that directory. Reverse this only if a real trial shows people
  abandoning the first run rather than working it down.
- **`init` proposes and does not impose, with an unanswerable prompt as a tool error.** Stdin
  closed — a CI runner — exits 2 telling you to pass `--yes`. Treating "no answer" as a decline
  would make `init` a command that reports success having written nothing.
- **Re-running `init` is not an error; clobbering is.** Each artifact is handled independently:
  missing ones are written, existing ones are kept and reported with the reason they were kept,
  and `.trestle.yml` is never rewritten once it exists. A second run prints which detected shapes
  the existing config does not cover, so it is worth running again after the repo grows.
  Exit stays 0. The one refusal is a `.trestle.yml` in a *parent* directory: that would create a
  nested second root, and every relative path in both configs would resolve against a directory
  its author did not mean.
- **CONVENTIONS.md is embedded from the repo root, which required a root Go package.** `go:embed`
  cannot reach outside its own package directory, so the alternative was a second copy of the
  contract under `internal/`. A four-line package at the root is the cheaper price than a
  duplicated contract file in a repo whose entire subject is duplicated-fact drift.
- **The starter diagram cannot show a commented-out directive**, because there is no such thing:
  the scanner strips the leading run of `#` before looking for `@`, so `## @bind ...` is live.
  Every example in the scaffolded file therefore keeps the directive off the start of its line,
  and `internal/scaffold` has a test that parses the file and asserts zero directives. This is
  the authoring gap Phase 2 flagged and left unresolved; CONVENTIONS.md now states it under
  Traps, with `@ignore` named as the supported way to say "not now".

---

---

### Deferred (named, so they don't get re-litigated)

- **Structurizr-style model/view separation.** Real value, wrong time. Pays off only once there are enough views to contradict each other. v2.
- **Symbol-level binding via language servers.** Only if Spike 01 proves globs too coarse — and if so, that is a scope change, not an addition.
- **Generated diagrams** (derive nodes from dependency graphs rather than checking hand-written ones). Strictly more powerful, strictly larger. v2+.
- **Non-D2 backends** (Mermaid, PlantUML export). Trivial later, distracting now.

---

## Document map

| File | Contains |
| --- | --- |
| [`DECISIONS.md`](DECISIONS.md) | This file. Scope, non-goals, the locked ledger, and the resolutions taken while building. |
| [`DESIGN.md`](DESIGN.md) | Binding syntax, the five violation codes, config, the CLI surface. |
| [`DOGFOODING.md`](DOGFOODING.md) | How to run a trial that can falsify the tool. |
| [`../CONVENTIONS.md`](../CONVENTIONS.md) | The diagram-authoring contract. Ships with the product. |
| [`../CONTRIBUTING.md`](../CONTRIBUTING.md) | Setup, the architecture, and the constraints that are not negotiable by accident. |
| [`../spike/README.md`](../spike/README.md) | Spike 01 — the falsification test for O1, and the probe that runs it. |
| [`../examples/repairs-platform/`](../examples/repairs-platform/) | Worked example: a service tree with a bound diagram. |

**Open questions and deferred work live in the issue tracker**, not here — see the
[`roadmap`](https://github.com/timimsms/trestle/labels/roadmap) and
[`limitation`](https://github.com/timimsms/trestle/labels/limitation) labels. The ledger records
what was decided and why; the tracker records what has not been.
