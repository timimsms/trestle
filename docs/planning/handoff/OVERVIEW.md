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
| O1 | Is glob-level binding granular enough to catch real drift? | **Day 2** | Spike 01. This is the load-bearing assumption. If it fails, the differentiator wobbles and scope must change before any build work. |
| O2 | What is the discovery rule for `UNMAPPED`? | Day 4 | Per-repo convention (`app/services/*/`) vs. heuristic. Leaning explicit config — heuristics will generate noise and noise kills adoption. |
| ~~O3~~ | ~~Should unbound nodes warn or fail by default?~~ | **RESOLVED** | **Warn.** In the worked example `UNBOUND` fired on a queue node — not an error, a modeling gap I hadn't considered. That is prompt-shaped, not failure-shaped. Fail would have trained a suppression reflex on the first diagram written. |
| ~~O6~~ | ~~Cross-cutting code with no single owning node.~~ | **RESOLVED** | `shared:` in config, enumerated. See L10–L12. |
| O7 | Does enumerated `shared:` stay practical at scale? | Day 4 | L11 assumes a repo has ~5–20 shared subsystems. Unmeasured. If a real repo needs 50+ entries, enumeration is unusable and L11 needs revisiting — though 50 shared subsystems is itself a finding about the codebase, not just about Trestle. Count `lib/*/` and `app/middleware/*/` before day 4. |
| O4 | Monorepo behavior — one binding namespace or many? | Post-MVP | Deferred unless Spike 01 surfaces it as immediate. |
| O5 | Does the preview pane justify its build cost in v1? | Day 3 | `d2 --watch` already renders. Trestle's preview only earns its place if it overlays check status onto the diagram. If that overlay slips, cut the pane entirely. |

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
