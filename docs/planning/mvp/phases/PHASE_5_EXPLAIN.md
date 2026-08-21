# Phase 5 — `trestle explain` (post-MVP)

**Blocks on:** Phase 4 stop gate **and** a dogfooding result
**Status:** shipped. Authorized by the dogfooding result below; scoped against it rather than
against this file's draft.

The dogfooding demand was specific and it moved the center of the command. Eleven agent trials
produced one complaint more than any other: *there is no way to see what the tool parsed.* An
agent authoring a diagram could not confirm its tooltips had not spawned phantom nodes and had to
infer it from "0 warnings under `--strict`" — an inference from the absence of violations, which
is backwards. So **the node inventory, not `explain <node_id>`, is the primary view**, which is
the opposite of the emphasis in "Cheap and worth considering" below. The paragraph there warns
against the inventory becoming a second reporting surface competing with `check`; the line drawn
is that `explain` files violations under the node they are about and counts the rest, and never
prints the repo's violation list. `trestle check` remains the only command with an opinion.

---

```
trestle explain <node_id>
trestle explain --overlaps
```

`explain <node_id>` shows a node's bindings, the files each glob currently matches, and any
violations against it. `explain --overlaps` lists paths claimed by more than one node —
informational, **never a failure** (L12).

## Output format is deliberately unspecified

HANDOFF leaves this open on purpose and this file will not close it. Design it against the actual
debugging experience once the MVP has produced some.

One thing is worth holding onto while designing it: **this is the command an agent calls to
orient before editing a node** (CONVENTIONS.md, rule 1). Optimize for machine legibility at least
as much as human. A `--format=json` that an agent can consume is probably more load-bearing than
the human view.

### Decided, at ship

**Human: the same grammar as `check`.** Two spaces of indent, a ten-wide first column, body text
aligned under the subject. `check` and `explain` are two windows on one engine and they should
read as one tool. The first column holds a *binding status* — `bound`, `external`, `infra`,
`ignored`, `container`, `unbound` — deliberately lowercase, because a reader who has learned that
the uppercase words are the five violation codes must not be taught that there are eleven.

**No color.** `internal/report` colorizes so a failure is findable in a CI log. An inventory is a
dump whose columns already carry its structure, and a second copy of the ANSI machinery in a
second package is drift waiting to happen for a decoration nobody asked for.

**JSON: one schema, three questions.** Every view emits the same document; `kind` says which was
asked. `nodes` and `overlaps` hold the answer, `summary`/`diagrams`/`disabled` always describe the
whole repo. One shape means an agent parses one schema and `summary.overlaps` means the same
number however the command was invoked. Two conventions carried from `report`: `"version": 1` from
day one, and `null` for genuinely absent rather than `""`. One added: a binding's `files` array is
populated only in the single-node view and `null` elsewhere — `[]` means the glob claims nothing,
which is the ORPHAN case, and dumping every path behind every glob into an inventory would put a
100k-file repo's whole listing in the payload for a question nobody asked.

**Human and machine group overlaps differently, on purpose.** JSON is keyed by path, because the
question a program asks is "who claims this file". The human view groups by claiming node, because
the question a person asks is "which two boxes disagree" — and one line per path would bury that
under a directory's worth of repetition.

**One non-zero exit: an unknown node ID is exit 2.** Ambiguity is shown (it is the SYNTAX `check`
reports, and the candidates are the fix), but an ID that names nothing is a question the command
could not answer. Exiting 0 there lets an agent that misspelled the ID of the node it just renamed
read the silence as confirmation, which is the failure mode this whole tool exists to prevent.

## Why `--overlaps` exists

It is the pressure-release valve for L12. Overlapping bindings are legal because two nodes may
honestly share a directory — but the same signal is also what a copy-paste error looks like.
Rather than add a sixth violation code, the ledger chose to surface overlaps here. If `--overlaps`
turns out to be where people actually find bugs, that is an argument to revisit L12, and it should
be written down rather than acted on unilaterally.

**Status at ship: L12 is now observable, and still unproven.** The Phase 4 stop gate recorded that
L12's justification was a promissory note the MVP did not ship, and that the `overlap/` fixture
could only prove a negative — it passed against an engine with no overlap detection at all. It now
passes against one that finds the shared path and reports it as information. Whether overlap is
where people find bugs is still unmeasured; nothing has been acted on. One thing the
implementation did have to decide, recorded so it is a decision and not an accident: **a node
whose own two globs both claim a file is not an overlap.** Overlap is two *nodes* disagreeing
about ownership, and reporting a node against itself would bury the real ones.

Deliberately not included: a path claimed by a node *and* by a `shared:` entry. That is also a
contradiction — one says "owned by this box", the other says "owned by nobody" — and it is
arguably a better bug detector than node-vs-node overlap. It is out because L12 says "claimed by
more than one node" and widening the definition unilaterally is the thing this file warns against.
Worth considering as a follow-up.

## Cheap and worth considering

- `explain` on a node ID that resolves ambiguously (O8) should show every candidate. It is the
  natural place to debug the error `check` reports.
- `explain` should surface any `severity: off` in config. A silently disabled code is exactly the
  kind of thing that rots, and `check` cannot report what it has been told not to report.
- `explain` with no argument could list all nodes and their binding status. Resist if it becomes a
  second reporting surface competing with `check`.

## Constraints

- Reuses `internal/check` unchanged. If `explain` needs the engine to expose something new, add a
  return value — do not add a second engine.
- Still no fifth top-level command. `--overlaps` is a flag on `explain` for exactly this reason.
- Exit 0 always, unless the tool itself errors (exit 2). `explain` reports; it does not judge.

## Acceptance

- [x] `explain platform.svc_work_orders` on the worked example lists 2 globs with their current
      match counts — `TestWorkedExampleNodeListsBothGlobsWithCounts`
- [x] Suffix-resolved IDs work, and an ambiguous ID lists candidates rather than erroring —
      `TestSuffixResolutionAndAmbiguity`. Resolution goes through `nodes.Candidates`, so `explain`
      and `check` implement one O8; `TestFindAgreesWithTheEngineResolution` pins it
- [x] `--overlaps` on the `overlap/` fixture lists the shared path and exits 0 —
      `TestOverlapsFixtureListsTheSharedPath`
- [x] A machine-readable format exists and is documented in CONVENTIONS.md — one schema for all
      three views, golden-tested
- [x] Added beyond the draft: the node inventory (`TestInventoryListsExactlyTheParsedNodeSet`,
      checked against the parser's own node set rather than a hand-written list), `severity: off`
      surfaced in every view, and the unresolved-directive section that keeps a dead `@bind` from
      being invisible

## What it needed from the engine

One export, `check.Matcher` — the glob machinery `Check` already runs on, wrapped so `explain` can
ask "what does this claim right now". `Check` itself is unchanged. The alternative was a second
doublestar loop in `explain`, which would have been free to drift from the rules that make a match
count what it is (a glob matching a directory claims the files beneath it; a file claimed twice
counts once), and a debugging command that quietly disagrees with the command it debugs is worse
than none.

`run.Context.Input()` was also exposed, so both commands build the engine's input the same way.
