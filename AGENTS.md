# Agent instructions — Trestle

Trestle is a Go CLI that binds architecture-diagram nodes to real repo paths and fails CI when
they diverge. Read these in order before making changes:

1. `docs/planning/mvp/GAMEPLAN.md` — the build plan, gate verdicts, and architecture
2. `docs/planning/mvp/phases/PHASE_<N>_*.md` — the phase you are working in
3. `docs/planning/handoff/OVERVIEW.md` — scope and the locked decision ledger L1–L12
4. `docs/planning/handoff/DESIGN.md` — binding syntax and check semantics
5. `CONVENTIONS.md` — the diagram-authoring contract that ships with the product

## Locked decisions are locked

L1–L12 in OVERVIEW are not suggestions. **If one seems wrong, stop and say so — do not route
around it.** Naming a decision that the implementation pushed back on is more valuable than
quietly working around it. Two such gaps (O8, O9) were already found and resolved this way; that
is the intended workflow.

## Standing constraints

- **Five violation codes. Do not add a sixth.** Fold new failure modes into existing codes or
  surface them through `explain`.
- **Four top-level commands.** Do not add a fifth without writing down why in the ledger.
- **`internal/check` stays I/O-free.** It is a pure function of (listing, nodes, directives,
  config). Reaching for the filesystem there means the seam is in the wrong place.
- **One filesystem walk.** All globs apply to that single listing — not one walk per binding.
  The 200ms/100k-file target depends on it.
- **Do not hand-roll a D2 parser.** Gate B proved `d2compiler.Compile` works; use it. No regex
  fallback, no grammar fork.
- **Do not build the preview pane** unless O5 resolves in its favor.
- **Every violation carries a runnable hint.** It is a golden-tested contract. A failing check
  that does not tell you what to type is one people learn to route around.

## Before you say a phase is done

```console
make            # gofmt, go vet, go test, go build — all clean
make test-core  # internal/check alone
```

Then check the phase file's acceptance list item by item. A phase is done when its criteria
pass, not when the code exists. Tests ship in the same change as the logic, never as a follow-up.

## Diagram edits

If you edit any `.d2` in this repo, `CONVENTIONS.md` applies to you: edit by node ID, never
regenerate the file, add a binding in the same change as the node, and do not restyle unrelated
parts.
