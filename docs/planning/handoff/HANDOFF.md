# Trestle — Handoff

**Audience:** a Claude Code agent building the MVP.
**Read first:** `OVERVIEW.md` (scope + ledger), then `DESIGN.md` (semantics), then `TECH_STACK.md`.
**Do not read as suggestions:** the locked decisions L1–L12 in OVERVIEW. If one seems wrong, stop and say so — do not route around it.

---

## Two gates before any code

### Gate A — Spike 01 verdict (human-run, blocking)

`SPIKE-01-glob-binding.md` + `spike/glob-binding-probe.sh`. Run by the repo owner against a
repo they actually work in, sweeping `--unit-depth 1..4`.

**Do not begin Task 1 until the verdict is recorded as an amendment to O1 in OVERVIEW.md.**

If the verdict is `Q3 > Q2`, the design is wrong and this handoff is void — return to design
rather than building something the spike disproved.

### Gate B — D2 AST surface (agent-run, blocking, ~30 min)

TECH_STACK assumes node IDs can be extracted via the D2 library's public AST. **This is
unverified.** Verify before writing anything else:

```go
// scratch program — does this compile and return node IDs,
// including nested container paths like `platform.svc_billing`?
import "oss.terrastruct.com/d2/d2compiler"
```

Feed it `examples/repairs-platform/system.d2` and confirm you can recover all 12 node IDs
**with container qualification intact**.

**Stop and report the result before proceeding.** Three outcomes:

| Outcome | Action |
| --- | --- |
| Public AST, container paths recoverable | Proceed to Task 1 |
| AST works but container paths are flattened | **Report.** Nested IDs are used throughout the worked example; this changes the ID matching rules |
| AST is internal-only / unstable | **Report and stop.** The regex fallback is fragile on exactly the nesting this project uses. Do not hand-roll a D2 parser — that is a tar pit and not the problem being solved |

---

## Task sequence

Each task lists acceptance criteria. A task is not done until its criteria pass.
Tasks 1–4 are the MVP. Stop after Task 4 and report before continuing.

### Task 1 — Fixture repos

Build these **before** the check engine. Writing fixtures first forces the engine's
interface to be pinned down before its logic gets written.

Under `testdata/repos/`, each a real directory tree with a `.d2` and a `.trestle.yml`:

| Fixture | Contains | Expected `check` result |
| --- | --- | --- |
| `clean/` | Every node bound, every discovered path owned | exit 0, no violations |
| `orphan/` | A `@bind` glob matching zero files | 1 ORPHAN, exit 1 |
| `orphan_shared/` | A `shared:` entry matching zero files | 1 ORPHAN, exit 1 (L11 — shared entries are ORPHAN-checked) |
| `unmapped/` | A `discover`-matched dir covered by no binding | 1 UNMAPPED, exit 1 |
| `dangling/` | A directive naming a node absent from the `.d2` | 1 DANGLING, exit 1 |
| `unbound/` | A node with no `@bind`/`@external`/`@infra`/`@ignore` | 1 UNBOUND **warning**, exit 0 |
| `overlap/` | Two nodes binding the same path | exit 0, no violation (L12) |
| `syntax/` | `@ignore` with no reason string; `@bind` with no glob | 2 SYNTAX, exit 1 |
| `nested/` | Container-qualified IDs (`platform.svc_x`) bound and checked | exit 0 |

**Acceptance:** all nine trees exist; each has a one-line `EXPECTED` file stating its
expected exit code and violation set.

### Task 2 — Directive parser + node extraction

`internal/directive` and `internal/nodes`.

- Line scan for `@bind`, `@external`, `@infra`, `@ignore`. One directive per line, no
  continuations. Position-independent — a directive need not sit near its node.
- `@ignore` without a quoted reason is `SYNTAX`, not a silent pass.
- Node IDs via the Gate B mechanism, container paths preserved.

**Acceptance:** parses `examples/repairs-platform/system.d2` yielding 6 binds, 2 external,
2 infra, 1 ignore, 12 node IDs. Table-driven tests cover each malformed-directive case in
the `syntax/` fixture.

### Task 3 — Check engine

`internal/check`. **This is the product.** Everything else is packaging.

- Pure function: `(fileListing, directives, config) -> []Violation`. **Zero I/O.** The
  filesystem walk happens outside and is passed in.
- Single tree walk; all globs applied to that one listing (DESIGN §7). Not one walk per binding.
- Five violation codes, no more. Severity overridable per-code from config.
- Exit codes: 0 clean, 1 violations, 2 tool error. Keep 1 and 2 distinct — conflating them
  trains people to ignore both.

**Acceptance:** every fixture from Task 1 produces its `EXPECTED` result. Unit tests run
with no filesystem access. Under 200ms on a 100k-file listing.

### Task 4 — `trestle check` CLI

Cobra. `--format=human|json`, `--strict` (promotes warnings to failures).

Human output per DESIGN §5 — **every violation carries a runnable hint.** A failing check
that doesn't tell you what to type is one people learn to route around. Golden-file tests
on the human output; the hints are part of the contract.

**Acceptance:** `trestle check` in each fixture dir matches its `EXPECTED`. `--format=json`
round-trips to the same violation set. Golden files committed.

**→ STOP HERE. Report before continuing.** Tasks 1–4 are the MVP; everything after is
convenience and should be re-scoped against what the MVP taught you.

---

### Task 5 — `trestle explain` (post-MVP)

`explain <node_id>` shows bindings, current matches per glob, violations.
`explain --overlaps` lists multiply-claimed paths — informational, never a failure.

Output format is **deliberately unspecified.** Design it against the debugging experience;
this is the command an agent calls to orient before editing, so optimize for machine
legibility as much as human.

### Task 6 — `trestle render`

Embedded D2 library, not a subprocess (TECH_STACK). `--watch` via fsnotify.

### Task 7 — `trestle init`

Scaffold `.trestle.yml`, seed `discover` from detected layout, write `CONVENTIONS.md` into
the target repo and add an `AGENTS.md` pointer. `CONVENTIONS.md` ships **as part of the
product** — it is the agent contract, not internal docs.

---

## Standing constraints

- **Do not add a sixth violation code.** Five is the number people will learn. New failure
  modes get folded into existing codes or surfaced through `explain`.
- **Do not add a fifth top-level command** without saying why in the ledger.
- **Do not build the preview pane** unless O5 resolves in its favor. A preview showing what
  `d2 --watch` already shows is not worth a line of code.
- **Do not implement Structurizr-style model/view separation.** Deferred, deliberately, to v2.
- **`internal/check` stays I/O-free.** If you find yourself reaching for the filesystem in
  there, the seam is in the wrong place.

## Reporting back

At each stop gate, report: what passed, what surprised you, and **any locked decision the
implementation pushed back on.** L1–L12 were made without code; some will be wrong. Naming
which ones and why is more valuable than working around them quietly.
