# Phase 5 — `trestle explain` (post-MVP)

**Blocks on:** Phase 4 stop gate **and** a dogfooding result
**Status:** not authorized. Re-scope against what the MVP taught before starting.

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

## Why `--overlaps` exists

It is the pressure-release valve for L12. Overlapping bindings are legal because two nodes may
honestly share a directory — but the same signal is also what a copy-paste error looks like.
Rather than add a sixth violation code, the ledger chose to surface overlaps here. If `--overlaps`
turns out to be where people actually find bugs, that is an argument to revisit L12, and it should
be written down rather than acted on unilaterally.

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

## Acceptance (draft — revise at scoping)

- [ ] `explain platform.svc_work_orders` on the worked example lists 2 globs with their current
      match counts
- [ ] Suffix-resolved IDs work, and an ambiguous ID lists candidates rather than erroring
- [ ] `--overlaps` on the `overlap/` fixture lists the shared path and exits 0
- [ ] A machine-readable format exists and is documented in CONVENTIONS.md
