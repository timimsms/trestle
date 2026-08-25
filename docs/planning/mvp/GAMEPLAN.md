# Trestle — MVP Gameplan

**Derived from:** `docs/planning/handoff/` (OVERVIEW, DESIGN, TECH_STACK, HANDOFF, SPIKE-01).
**Status:** both blocking gates cleared. Build is authorized through Phase 4.
**Scope of "MVP":** Phases 1–4. Everything after is convenience and gets re-scoped against
what the MVP teaches.

This document is the plan. The per-phase task breakdowns live in `phases/`. Where this file
and the handoff disagree, the handoff wins unless the disagreement is called out below as a
resolved amendment.

---

## 1. What is being built

A single static Go binary, `trestle`, that reads architecture diagrams written in D2, reads
binding directives embedded in those diagrams as magic comments, walks the repo once, and
reports where the diagram and the filesystem disagree — with a CI-meaningful exit code.

The whole product is one pure function:

```
(fileListing, directives, nodeIDs, config) -> []Violation
```

Everything else — the CLI, the walker, the D2 parse, the renderer — is packaging around it.
`internal/check` must never touch the filesystem. If a phase's implementation reaches for
`os.ReadFile` inside `internal/check`, the seam is in the wrong place and the phase is not done.

**The success criterion has not changed** and is not a build target: `trestle check` must fail
on a real PR, at least once, for a reason nobody anticipated when the bindings were written.
If it never fires it is decoration and should be deleted.

---

## 2. Gate verdicts

Both gates in HANDOFF.md are cleared. Full numbers are recorded as ledger amendments in
`../handoff/OVERVIEW.md`; summarized here because they change the plan.

### Gate A — Spike 01 (O1): **PROCEED, with a caveat**

Probed a private 4,007-file monorepo (3,172 commits, all inside the window) at unit
depths 1–4 over 180 days.

| Depth | Q2 orphan | Q3 silent | Q4 new | Signal |
| --- | --- | --- | --- | --- |
| 1 | 1 | **0** | 16 | 17 |
| 2 | 1 | **0** | 59 | 60 |
| 3 | 0 | **0** | 105 | 105 |

The result that would have voided the design — `Q3 > Q2`, silent gutting outweighing detectable
drift — did not fire at any depth. **Q3 was zero everywhere.** Globs are not too coarse.

The caveat, carried forward rather than buried: the signal is Q4-dominated and Q4 is inflated by
a young repo growing (5–6 units at window start, 20–64 today), not by an established architecture
drifting. Q2 is thin. This licenses the build; it does not prove the tool will fire often. Treat
the OVERVIEW success criterion as genuinely at risk.

**Consequence for the plan:** `discover:` seeding in `trestle init` (Phase 7) targets **depth 2**.
Depth 3 fragments a real repo into 118 units, 35 of them holding ≤2 files — authoring burden, not
architecture.

### Gate B — D2 AST surface: **PASS (outcome 1)**

`oss.terrastruct.com/d2 v0.7.2`. `d2compiler.Compile` is public; walking `g.Root.ChildrenArray`
and reading `Object.AbsID()` recovers all 12 node IDs from the worked example **with container
qualification intact**. Shapes, labels and all 10 edges come along for free. No regex fallback,
no D2 grammar fork. Pin the version — TECH_STACK says it moves, and it does.

---

## 3. Two spec gaps the gate probe surfaced

Gate B was supposed to be a 30-minute yes/no. It came back yes, and with two problems that would
have detonated in Phase 3 if they had been discovered there. Both are resolved here; both are
recorded as O8/O9 in the ledger.

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

## 4. Architecture

```
conventions.go          package `trestle`: go:embed of CONVENTIONS.md, nothing else.
cmd/trestle/            main + cobra wiring. Thin. No logic.
internal/
  config/               .trestle.yml load, validate, defaults, root discovery
  directive/            magic-comment line scanner
  nodes/                D2 AST -> node IDs (d2compiler)
  walk/                 the single filesystem walk. All I/O lives here.
  check/                THE PRODUCT. Pure. Zero I/O. Heavily tested.
  render/               D2 library wrapper (Phase 6)
  scaffold/             `trestle init` — layout detection, the emitted files (Phase 7)
  report/               human + json formatting, golden-tested
  expected/             fixture EXPECTED parser, shared by Phases 3 and 4
  integration/          cross-seam guards — where two packages must agree
testdata/repos/         ten fixture trees
examples/repairs-platform/   the worked example — a live test input, not a doc
spike/                  glob-binding-probe.sh (Gate A, keep for re-runs)
```

Dependency direction is one-way: `cmd` → everything; `check` → nothing but `config`,
`directive`, `nodes` types. `check` importing `walk` is a design failure, and CI should
eventually enforce that with an import-graph test.

**Restating the I/O rule precisely**, because the original phrasing contradicted itself: "all
filesystem I/O lives in `walk`" was written alongside explicit permission for `directive`,
`nodes`, and `config` to open their own named input file. The rule that is actually meant:

> **The repo walk lives in `walk`. `check` does no I/O at all.**

`directive`, `nodes`, and `config` each expose a pure `Parse(path, src []byte)` primary with a
thin `ParseFile` convenience. That is the shape they were built in, and it is the shape Phase 4
should wire.

**Pinned dependencies** (TECH_STACK, confirmed by Gate B):

| Concern | Module |
| --- | --- |
| D2 | `oss.terrastruct.com/d2 v0.7.2` — pinned exactly |
| CLI | `spf13/cobra` |
| Config | `goccy/go-yaml` |
| Globs | `bmatcuk/doublestar/v4` — non-negotiable, stdlib has no `**` |
| Walk | stdlib `io/fs.WalkDir` |
| Watch | `fsnotify/fsnotify` — Phase 6 only |
| Directives | stdlib. Line scan + `strings.Fields`. No parser generator. |

---

## 5. Phase map

| Phase | File | Delivers | Blocks on | MVP |
| --- | --- | --- | --- | --- |
| 0 | `PHASE_0_GATES.md` | Gate A + B verdicts, O8/O9 resolutions, repo scaffold | — | ✅ done |
| 1 | `PHASE_1_FIXTURES.md` | Ten fixture repos + `EXPECTED` contracts | 0 | ✅ done |
| 2 | `PHASE_2_PARSERS.md` | `directive`, `nodes`, `config`, `walk` | 0 | ✅ done |
| 3 | `PHASE_3_CHECK_ENGINE.md` | `internal/check` — the product | 1, 2 | ✅ done |
| 4 | `PHASE_4_CLI.md` | `trestle check`, human + json, golden files | 3 | ✅ done |
| — | **← STOP GATE. The MVP is here. Dogfood before Phase 5.** | | | |
| 5 | `PHASE_5_EXPLAIN.md` | `trestle explain`, `--overlaps` | 4 | — |
| 6 | `PHASE_6_RENDER.md` | `trestle render`, `--watch` | 4 | — |
| 7 | `PHASE_7_INIT.md` | `trestle init`, CONVENTIONS emission | 4, 5 | — |

**Phases 1 and 2 are independent and run in parallel.** This is deliberate and it is the reason
HANDOFF orders fixtures first: writing the expected outputs before the engine forces the engine's
interface to be pinned down by its contract rather than by its implementation. Phase 3 must not
start until both land, because Phase 3's definition of done is *"every fixture produces its
`EXPECTED`"* — an engine written against a moving fixture set proves nothing.

Within Phase 2 the four packages are independent of each other and can be split further.

---

## 6. The violation taxonomy is closed

Five codes. `ORPHAN`, `UNMAPPED`, `DANGLING`, `UNBOUND`, `SYNTAX`. Severity is overridable
per-code from config; `UNBOUND` defaults to `warn`.

Exit codes: `0` clean (warnings allowed), `1` failing violations, `2` tool error. **Keep 1 and 2
distinct.** Conflating them trains people to ignore both.

Standing constraints, carried verbatim from HANDOFF and non-negotiable without a ledger entry:

- **Do not add a sixth violation code.** New failure modes fold into existing codes or surface
  through `explain`.
- **Do not add a fifth top-level command** without writing down why.
- **Do not build the preview pane** unless O5 resolves in its favor.
- **Do not implement Structurizr-style model/view separation.** v2, deliberately.
- **`internal/check` stays I/O-free.**
- **Do not hand-roll a D2 parser.** Gate B removed the only excuse for it.

**Every violation carries a runnable hint.** This is a contract, not a nicety, and it is
golden-tested. A failing check that does not tell you what to type is one people learn to route
around.

---

## 7. Definition of done, per phase

A phase is done when its acceptance criteria in the phase file pass — not when the code exists.
Across all phases:

- `go build ./...` and `go vet ./...` clean.
- `go test ./...` passes. New logic ships with tests in the same change, not as a follow-up.
- `gofmt -l .` empty.
- No new dependency without a line in the phase file saying why stdlib was insufficient.
- `internal/check` tests run with no filesystem access. If a check test needs a `testdata` dir,
  the test is in the wrong package.

---

## 8. Risks, ranked

| Risk | Why it matters | Mitigation |
| --- | --- | --- |
| **Success criterion never fires** | The tool is decoration and OVERVIEW says delete it | Gate A caveat is explicit: Q4 was inflated. Dogfood on a real repo the moment Phase 4 lands, before building Phases 5–7 |
| **O8 suffix matching is too clever** | Silent mis-binding after a rename is the exact bug class this tool exists to catch | Ambiguity is `SYNTAX`, never a silent pick. Fixture `nested/` must cover both the resolving and the ambiguous case |
| **`UNMAPPED` is noisy** | A noisy check gets `--no-verify`'d within a week; noise kills adoption | `discover:` is explicit config, not heuristic (O2 resolved). Depth 2 per Gate A |
| **Trailing-slash glob mismatch silences `UNMAPPED` entirely** | Verified: `app/services/*/` matches `app/services/billing/` but **not** `app/services/billing`. The shipped example config uses the trailing-slash form. Bare directory paths would make every `discover` rule match nothing and the check pass while seeing nothing — a silent failure in the half of the product that catches *new* code | `walk` emits directories **flagged** (`Entry.IsDir`), with no trailing slash; `check` synthesizes the slash on both sides and accepts `*`, `*/`, `**` as equivalent authoring forms. A `discover` rule matching zero units fires `ORPHAN`, so this can never fail silently again. Pinned by `integration.TestDiscoverGlobNeedsTrailingSlash` |
| **d2 v0.7.2 AST breaks on upgrade** | Node extraction is load-bearing | Pinned exactly. `nodes` package has a version-canary test against the worked example |
| **200ms/100k-file target missed** | A check slower than a lint rule gets moved to nightly, and nightly stops blocking the PR that broke it | Single walk, all globs applied to one listing. Benchmark is a Phase 3 acceptance criterion, not a Phase 4 afterthought |
| **Enumerated `shared:` doesn't scale (O7)** | L11 becomes unusable | Gate A suggests ~13 entries for a 4k-file repo — inside L11's range, but only just. Re-probe a second repo before v1 |

---

## 9. Stop-gate report — MVP complete

`trestle check` builds, runs, and matches all ten fixtures plus the worked example in both
formats. `go build`, `go vet`, `gofmt`, `go test`, `go test -race`, and `golangci-lint` are clean.
End-to-end on a 100k-file repo is **37–71ms** against a 200ms budget; `internal/check` alone is
**2.5ms**, and it is I/O-free with an import-graph test that fails if that ever changes.

### What surprised us

- **Gate B was meant to be a 30-minute yes/no.** It passed and produced two spec gaps (O8, O9)
  that would have detonated in Phase 3. Without them the shipped worked example fails its own
  check with 6 `DANGLING` and 2 `UNBOUND`.
- **A subtree is not the contiguous run after its directory.** `app/services/billing-old` sorts
  *between* `app/services/billing` and its children, because `-` < `/` in ASCII. The obvious
  implementation reports a bound service as `UNMAPPED` and an unrelated sibling as owned.
- **"One walk, all globs on one listing" is necessary and not sufficient.** It fixes the I/O and
  says nothing about matching cost; naive matching would have blown the budget at 20 bindings.
  Prefix-narrowing against the sorted listing gave an 82× reduction.
- **Three documents described behavior the tool does not have**, in ways that would have misled
  the next reader: two claimed `walk` emits trailing slashes, and DESIGN §5 grouped `UNMAPPED`
  under the diagram. All corrected against the implementation, not the other way round.

### Locked decisions the implementation pushed back on

- **L12 is currently unobservable.** Its justification is "surfaced via `explain --overlaps`",
  which the MVP does not ship. The `overlap/` fixture can only prove a negative — it passes
  against an engine with no overlap detection at all. Not wrong; not yet true.
- **L11/O7 remains unexercised.** Every fixture `shared:` list has 1–2 entries. Whether
  enumeration scales is still unmeasured and still needs a second real repo.
- **DESIGN §5 anticipated a field the engine does not carry** — the as-written suffix is
  discarded at resolution, so both formats print the fully-qualified ID.
- **Config-sourced violations have no line number.** `config` already tracks `seqLine("shared", i)`
  for validation errors and discards it, so `.trestle.yml` findings are not clickable. Small
  change, real ergonomic payoff, not yet made.

### Closed at the gate

A repo could set `severity: {ORPHAN: off, UNMAPPED: off}` and get `0 failures, 0 warnings` and
exit 0 from a check that inspected nothing — the same silent-green family as a config matching
zero diagrams, which Phase 4 was explicitly told to close. `check.DisabledCodes` is now reported
on the summary line (`0 failures, 0 warnings (ORPHAN off)`) and as a `disabled` array in JSON,
pinned by `internal/integration/disabled_test.go`. It is not a sixth code: disabling a code is
legal, doing it invisibly is not.

### The next move is dogfooding, not Phase 5

Every violation shape produced so far came from a fixture written on purpose. Trestle has no
`.trestle.yml` of its own and has never run against a repo with real churn. Gate A's caveat —
Q4-inflated signal, thin Q2 — means the OVERVIEW success criterion is genuinely at risk, and it
is the MVP's evaluation gate. Building `explain`, `render`, and `init` on a check that never fires
is three phases of work on a tool OVERVIEW says to delete.

---

## 10. Reporting back

At the Phase 4 stop gate, report three things — the third matters most:

1. What passed.
2. What surprised you.
3. **Which locked decisions the implementation pushed back on.** L1–L12 were made without code.
   Gate B already broke two unstated assumptions inside thirty minutes. Naming which decisions
   were wrong, and why, is worth more than quietly routing around them.
