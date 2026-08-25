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

### Deferred (named, so they don't get re-litigated)

- **Structurizr-style model/view separation.** Real value, wrong time. Pays off only once there are enough views to contradict each other. v2.
- **Symbol-level binding via language servers.** Only if Spike 01 proves globs too coarse — and if so, that is a scope change, not an addition.
- **Generated diagrams** (derive nodes from dependency graphs rather than checking hand-written ones). Strictly more powerful, strictly larger. v2+.
- **Non-D2 backends** (Mermaid, PlantUML export). Trivial later, distracting now.

---

## Document map

| File | Contains | Audience |
| --- | --- | --- |
| `OVERVIEW.md` | This file. Scope, non-goals, decision ledger. | Everyone, first |
| `HANDOFF.md` | Sequenced build tasks, acceptance criteria, stop gates. | Build agent |
| `DESIGN.md` | Binding syntax, check semantics, violation taxonomy, CLI surface. | Build agent |
| `TECH_STACK.md` | Language, dependencies, project layout, testing. | Build agent |
| `SPIKE-01-glob-binding.md` | The day-two falsification test for O1. | Repo owner |
| `spike/glob-binding-probe.sh` | Executable probe for Spike 01. Validated against a positive control. | Repo owner |
| `CONVENTIONS.md` | The agent contract. Ships *as part of the product*, not just as internal docs. | Diagram authors + agents |
| `examples/repairs-platform/` | Worked example: `system.d2` + `.trestle.yml`. The Gate B test input. | Build agent |
