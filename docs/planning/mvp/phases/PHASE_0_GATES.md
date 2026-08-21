# Phase 0 — Gates & Scaffold

**Status:** ✅ complete
**Blocks:** everything
**Owner:** repo owner (Gate A) + build agent (Gate B)

Phase 0 exists to answer two questions that, if answered wrong, mean the rest of the plan should
not be built. Both are now answered. This file is the record.

---

## 0.1 Gate A — Spike 01 (O1) ✅

**Verdict: PROCEED, with a recorded caveat.**

Run `spike/glob-binding-probe.sh` against a repo with real structural history, sweeping
`--unit-depth 1..4`. Probed `paperclip` at 180 days.

| Depth | Units today (start) | Q2 orphan | Q3 silent | Q4 new | Signal |
| --- | --- | --- | --- | --- | --- |
| 1 | 20 (5) | 1 | **0** | 16 | 17 |
| 2 | 64 (6) | 1 | **0** | 59 | 60 |
| 3 | 118 (13) | 0 | **0** | 105 | 105 |

`Q3 > Q2` — the falsification criterion that voids the design — did not fire at any depth.
Q3 was zero everywhere: no unit lost ≥70% of its files while surviving.

Trestle itself was also probed and returned zero at every depth. It has one commit and no code,
so it measures nothing; recorded only to note it was tried.

**Caveat:** signal is Q4-dominated, and Q4 is inflated by a young repo growing rather than a
settled architecture drifting. Q2 is thin (1, 1, 0). Read as *"globs are not too coarse"*, not
as *"drift is rampant."*

**Depth 2 is the right unit depth.** Depth 3 fragments into 118 units, 35 with ≤2 files.

Full numbers: `../../handoff/OVERVIEW.md`, amendment to O1.

## 0.2 Gate B — D2 AST surface ✅

**Verdict: PASS, outcome 1 — public AST, container paths recoverable.**

```go
g, _, err := d2compiler.Compile(path, strings.NewReader(src), nil)
// walk g.Root.ChildrenArray recursively; read Object.AbsID()
```

`d2 v0.7.2` returns all 12 node IDs from `examples/repairs-platform/system.d2` with container
qualification intact (`platform.svc_work_orders`). Shapes, labels, and all 10 edges available.

**Pin `oss.terrastruct.com/d2` to v0.7.2 exactly.** TECH_STACK warns it moves.

## 0.3 Amendments the gate produced

Gate B surfaced two gaps that would have detonated in Phase 3. Both resolved in GAMEPLAN §3 and
recorded as O8/O9 in the ledger. Restated here because Phases 2 and 3 implement them:

**O8 — suffix matching.** A directive ID matches an AST node if equal to `AbsID()` **or** a
dot-delimited suffix of it *on a segment boundary*. `svc_work_orders` matches
`platform.svc_work_orders`; `orders` does not. Ambiguous suffix → `SYNTAX`, never a silent pick.
Fully-qualified beats suffix.

**O9 — containers.** `UNBOUND` is suppressed for a node with children when every descendant is
accounted for by some directive. A container with an unaccounted descendant still warns — on the
descendant. Containers may carry their own `@bind`, checked normally.

Without these, the shipped worked example fails its own check with 6 `DANGLING` + 2 `UNBOUND`.

## 0.4 Scaffold ✅

- `go.mod` → `github.com/timimsms/trestle`
- Spec pack unpacked to canonical locations: `examples/repairs-platform/`, `spike/`
- `CONVENTIONS.md` promoted to repo root — it ships with the product, it is not internal docs
- Package skeleton: `cmd/trestle`, `internal/{config,directive,nodes,walk,check,render,report}`
- `.gitignore`, `Makefile`, `.golangci.yml`, `.editorconfig`, CI workflow, `AGENTS.md`
- `.DS_Store` removed from tracking; tarball and duplicated docs removed

## Acceptance

- [x] Gate A verdict recorded as an O1 amendment with numbers, date, and repo
- [x] Gate B verdict recorded with the d2 version and the working API call
- [x] O8 and O9 resolved in writing before any engine code exists
- [x] `go build ./...` succeeds on the empty scaffold
