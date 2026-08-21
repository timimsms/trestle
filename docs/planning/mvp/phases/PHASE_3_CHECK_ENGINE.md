# Phase 3 — Check Engine

**Blocks on:** Phases 1 and 2, both complete
**Blocks:** Phase 4
**Package:** `internal/check`

---

> **This is the product. Everything else is packaging.**

One pure function. Zero I/O. The filesystem walk happened in Phase 2 and is passed in.

```go
func Check(in Input) []Violation

type Input struct {
    Files      []Entry          // from internal/walk — sorted, excluded already pruned
    Nodes      NodeTree         // from internal/nodes — IDs + parent relation
    Directives []Directive      // from internal/directive — with source positions
    Config     config.Config
}
```

If you reach for `os` or `io/fs` inside this package, stop. The seam is in the wrong place and
that is a design finding worth reporting, not routing around.

---

## The five codes

| Code | Fires when | Default |
| --- | --- | --- |
| `ORPHAN` | a `@bind` glob — **or a `shared:` entry** — matches zero files | fail |
| `UNMAPPED` | a path matched by a `discover:` rule is covered by no `@bind` glob | fail |
| `DANGLING` | a directive names a node ID absent from the `.d2` | fail |
| `UNBOUND` | a node has no `@bind`, `@external`, `@infra`, or `@ignore` | **warn** |
| `SYNTAX` | malformed directive | fail |

There is no sixth code. `@ignore <node> "<reason>"` suppresses **all** violations for that node.
Overlapping bindings are legal and get no code (L12) — they surface via `explain --overlaps`.

`shared:` entries being ORPHAN-checked is L11 and is not optional: it is the mechanism that stops
the shared layer accumulating dead declarations. Do not treat `shared` as write-only config.

---

## The single-pass rule

Apply every glob to the **one** listing from `internal/walk`. Not one walk per binding, not one
glob call per file per binding. DESIGN §7 makes this the stated performance strategy and the
200ms target depends on it.

### A single walk is necessary and not sufficient — read this before writing the matcher

Phase 2 measured `doublestar.Match` at roughly **7ms per pattern against a 100k listing** (~70ns
per call). The naive shape — every glob against every path — is therefore:

| Bindings | Matches | Cost |
| --- | --- | --- |
| 5 | 500k | ~35ms |
| 20 | 2M | **~140ms** |
| 50 | 5M | **~350ms** |

The walk itself already costs 29–59ms of the 200ms budget. **A double loop over
`doublestar.Match` misses the target at ordinary repo size**, and it will do so on real repos
rather than on the benchmark, which is the worst way to find out. DESIGN §7's "one walk, all
globs on one listing" fixes the I/O and says nothing about the matching cost; do not read it as
covering this.

**Narrow by literal prefix first.** `walk` returns the listing **sorted bytewise**, which is what
makes this cheap: a glob's literal prefix (`app/services/billing/**` → `app/services/billing/`)
identifies a contiguous run in that slice. Binary-search the run bounds, then run
`doublestar.Match` only inside it. A binding scoped to one directory then costs
O(log n + files-in-that-directory) instead of O(n). Globs with no literal prefix (`**/*.rb`) fall
back to a full scan — they are rare, and if they are not, that is a finding worth reporting.

Phase 2's `internal/walk` already does the analogous thing for `exclude:` and cut its cost in
half; `TestMatcherMatchesDoublestar` there proves rewrites against the library differentially
rather than by inspection. **Do the same here: any fast path must be proven equivalent to plain
`doublestar.Match` by a differential test, not by reasoning.** A matcher that is fast and subtly
wrong is worse than a slow one, because the errors look like drift.

Build `map[path][]nodeID` from that narrowed matching, then answer every question from the map.
`UNMAPPED` becomes a lookup, not a search.

### Synthesize the trailing slash for `discover:` — the silent-failure trap

`walk` returns `Entry{Path, IsDir}` with **no trailing slash on directory paths**. The shipped
example config writes `discover: app/services/*/` **with** one, and doublestar does not match a
trailing-slash pattern against a bare directory path.

**So when matching a `discover:` rule, match against `Path + "/"` for entries where `IsDir`.**

If you skip this, every `discover` rule matches zero units, `UNMAPPED` never fires, and
`trestle check` exits 0 while inspecting no code at all — a green check that proves nothing,
which is strictly worse than no check. `internal/integration/TestDiscoverGlobNeedsTrailingSlash`
pins the premise and will tell you if a doublestar upgrade changes it.

Be tolerant on input: a `discover` rule authored *without* the trailing slash should behave the
same as one with it. Normalize the rule, do not make the user guess.

### Two verified glob facts you must not re-derive

Measured against `doublestar v4.10.0`, because both are counterintuitive and both are load-bearing:

- `app/services/billing/**` **does** match `app/services/billing` itself, not only its contents.
- `app/services/*/` (trailing slash) matches `app/services/billing/` and **does not** match
  `app/services/billing`. `walk` emits directories **flagged** (`Entry.IsDir`) with a bare path,
  so `check` synthesizes the slash — see the section above.

A `discover:` rule that matches zero units is a config-level warning, not silence. That is the
guard that stops this class of bug from ever being silent again.

## O10 — what `UNMAPPED` coverage means

**A discover unit is covered when at least one non-excluded file beneath it is matched by some
`@bind` glob.** Not "does a bind glob match the unit's own path."

The path-based reading looks equivalent and is not: `@bind svc_billing app/services/billing/*.rb`
matches every file in the directory but not the directory path, so a path test would call a
correctly-bound directory `UNMAPPED`. False positives on ordinary authoring are what get a check
bypassed.

An **empty** discover unit therefore can never be covered and always fires `UNMAPPED`. That is
correct — an empty service directory is a real finding — but give it a distinguishing hint rather
than the generic "add a `@bind`" one.

## O11 — invalid directives participate in nothing else

**A directive that is `SYNTAX` or `DANGLING` is reported once and otherwise discarded.** It does
not bind, does not account for its node for `UNBOUND`, is not `ORPHAN`-checked, and confers no
`discover` coverage.

Half-using a directive whose node ID or glob could not be trusted invents intent, and inventing
intent is the wrong instinct in a tool built to catch stale intent.

Accepted consequence: one rename can produce a `DANGLING` **and** an `UNMAPPED`. Both statements
are true — the directive is stale *and* the code is now unowned — and the second is the expensive
one to miss. This is the resolution most likely to be wrong; it is also cheap to reverse. Revisit
after dogfooding and write down what you find.

---

## O8 — node ID resolution

A directive's node ID resolves to an AST node if:

1. it equals the node's `AbsID()` — exact match wins, always; or
2. it is a **dot-delimited suffix** of `AbsID()` on a segment boundary.

`svc_work_orders` → `platform.svc_work_orders` ✅
`orders` → `platform.svc_work_orders` ❌ (substring, not a segment suffix)

If a suffix resolves to **more than one** node, emit `SYNTAX` naming every candidate, and do not
pick one. A silent pick means a rename can quietly re-point a binding at the wrong node — the
precise bug class this tool exists to catch.

If it resolves to zero nodes, that is `DANGLING`.

## O9 — containers

`UNBOUND` is suppressed for a node that **has children** and where **every descendant is
accounted for** by a `@bind`, `@external`, `@infra`, or `@ignore`. A container grouping five
bound services is a grouping device, not an unowned subsystem.

- A container with an unaccounted descendant still warns — **on the descendant, not the
  container.** Never both; one modeling gap produces one warning.
- A container may carry its own `@bind`; if it does, that binding is ORPHAN-checked normally and
  the suppression rule is irrelevant.
- Leaf nodes are unaffected. `tenant` in the worked example has no children and no directive and
  correctly warns.

**Test both directions.** The negative case — container with one unaccounted child → exactly one
warning, on the child — is the one that catches an over-eager suppression.

---

## Severity and exit codes

Severity per-code from `config.Severity`, defaulting `UNBOUND: warn` and the rest to `fail`.
`off` suppresses a code entirely; it is legal but is the kind of thing `explain` should surface.

| Exit | Condition |
| --- | --- |
| 0 | no failing violations (warnings may be present) |
| 1 | one or more failing violations |
| 2 | tool error — bad config, unparseable D2, I/O failure |

**`check` returns violations; it does not compute the exit code and it does not print.** Exit
code and formatting are Phase 4. Keeping that out of here is what makes the function testable.

`--strict` promotes warnings to failures — also Phase 4. `check` reports severity; the caller
decides consequences.

---

## Every violation carries a hint

```go
type Violation struct {
    Code     Code
    Severity Severity
    Node     string   // node ID, where applicable
    Path     string   // filesystem path, where applicable
    Source   Position // file + line of the offending directive
    Detail   string   // "@bind app/services/billing/** matches 0 files"
    Hint     string   // a runnable next step
}
```

The hint is a **contract**, not a nicety, and Phase 4 golden-tests it. A failing check that does
not tell you what to type is one people learn to route around.

| Code | Hint shape |
| --- | --- |
| `ORPHAN` | ``renamed? `git log --diff-filter=D -- app/services/billing` `` |
| `UNMAPPED` | ``add `# @bind svc_notifications app/services/notifications/**` `` |
| `DANGLING` | name the closest existing node IDs by edit distance — a rename is the usual cause |
| `UNBOUND` | offer all four directives; the right answer is often `@infra`, not `@bind` |
| `SYNTAX` | quote the offending line and show the correct form |

For ambiguous-suffix `SYNTAX`, the hint lists every candidate ID so the fix is a copy-paste.

---

## Acceptance

- [ ] Every fixture from Phase 1 produces exactly its `EXPECTED` violation set and exit-code
      classification — all nine, plus `ambiguous/` if it was added
- [ ] Unit tests run with **no filesystem access**. If a test needs `testdata`, it belongs in an
      integration package, not here
- [ ] Table-driven tests: every violation code has a positive **and** a negative case
- [ ] O8 covered: exact match, suffix match, ambiguous suffix → SYNTAX, zero match → DANGLING
- [ ] O9 covered in both directions, including "warn on the descendant, not the container"
- [ ] Benchmark: **under 200ms on a 100k-file listing.** Committed as `BenchmarkCheck100k`,
      asserted in CI, not measured once by hand
- [ ] `internal/check` imports no I/O package. Add an import-graph test that fails if it does
- [ ] Severity overrides honored, including `off`

## Do not

- Do not add a sixth violation code. If a new failure mode appears, fold it into an existing
  code or surface it through `explain` — and write down that you had to.
- Do not emit a violation for overlapping bindings (L12).
- Do not compute exit codes or format output here.
- Do not make `@ignore` partial. It suppresses everything for its node; that is why the reason
  string is mandatory.
