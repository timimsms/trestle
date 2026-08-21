# Trestle

**Keeps architecture diagrams honest by binding diagram nodes to real paths in the repo, and
failing CI when they diverge.**

Architecture diagrams drift. Not because people are careless, but because nothing connects the
diagram to the thing it describes. A service gets renamed, a module gets deleted, a new subsystem
appears — and the diagram stays exactly as correct-looking as it was the day it was drawn.

Diagram-as-code fixes *reviewability*. It does not fix *truth*. A D2 file can be perfectly
version-controlled and completely wrong.

Trestle adds the missing edge: a declared, checkable binding between a node in the diagram and a
path in the codebase.

> **Status: MVP complete, at the stop gate.** Phases 1–4 have landed: `trestle check` works,
> in both formats, against all ten fixture repos and the worked example. `explain`, `render`
> and `init` are not built and are deliberately not started until the MVP has been pointed at
> a real repo. See [`docs/planning/mvp/GAMEPLAN.md`](docs/planning/mvp/GAMEPLAN.md).

---

## How it works

Bindings are magic comments inside the `.d2` file — no sidecar, because a sidecar bindings file
is itself a drift surface.

```d2
# @bind     svc_billing app/services/billing/**
# @bind     svc_billing app/jobs/billing/**
# @external ext_stripe
# @infra    db_primary
# @ignore   legacy_reporting "deleted Q3, kept for the migration narrative"

svc_billing: Billing Service
```

Then:

```console
$ trestle check

docs/architecture/system.d2

  ORPHAN    svc_billing
            @bind app/services/billing/** matches 0 files
            hint: renamed? `git log --diff-filter=D -- app/services/billing`

.trestle.yml

  UNMAPPED  app/services/notifications/
            no @bind glob covers this path
            hint: add `# @bind svc_notifications app/services/notifications/**` to a diagram, or add `app/services/notifications/**` to `shared:`

2 failures, 0 warnings
```

Findings group under the file responsible for them. A stale `@bind` is a fact about the diagram
that declared it; an unowned directory is a fact about the `discover:` rule that went unsatisfied,
and no single diagram is more to blame than another.

Exit `0` clean, `1` violations, `2` tool error. `1` and `2` stay distinct so CI can tell "your
diagram is wrong" from "Trestle is broken."

## Five violations, and only five

| Code | Fires when | Default |
| --- | --- | --- |
| `ORPHAN` | a `@bind` glob or `shared:` entry matches zero files | fail |
| `UNMAPPED` | a `discover:`-matched path is covered by no binding | fail |
| `DANGLING` | a directive names a node absent from the diagram | fail |
| `UNBOUND` | a node has no directive of any kind | warn |
| `SYNTAX` | malformed directive | fail |

Five is the number people will actually learn. New failure modes fold into existing codes.

## Commands

```
trestle check [--format=human|json] [--strict]   validate bindings — the product
trestle render [--watch]                         render via embedded D2
trestle explain <node_id> [--overlaps]           debug a node's bindings
trestle init                                     scaffold config + conventions
```

Four. Resist adding a fifth.

## What Trestle is not

A diagram editor, a WYSIWYG surface, a rendering engine, a hosted product, a model/view system,
or a history layer. These are deliberate exclusions, not backlog items — see
[`OVERVIEW.md`](docs/planning/handoff/OVERVIEW.md) for why each one is out.

## Documentation

| File | Contains |
| --- | --- |
| [`CONVENTIONS.md`](CONVENTIONS.md) | The agent contract. Ships with the product. |
| [`docs/planning/mvp/GAMEPLAN.md`](docs/planning/mvp/GAMEPLAN.md) | Build plan, gate verdicts, architecture |
| [`docs/planning/mvp/phases/`](docs/planning/mvp/phases/) | Per-phase tasks and acceptance criteria |
| [`docs/planning/handoff/OVERVIEW.md`](docs/planning/handoff/OVERVIEW.md) | Scope, non-goals, decision ledger L1–L12 |
| [`docs/planning/handoff/DESIGN.md`](docs/planning/handoff/DESIGN.md) | Binding syntax, check semantics, CLI surface |
| [`docs/planning/handoff/TECH_STACK.md`](docs/planning/handoff/TECH_STACK.md) | Language, dependencies, layout, testing |
| [`examples/repairs-platform/`](examples/repairs-platform/) | Worked example — a live test input |

## Development

```console
make            # fmt, vet, test, build
make test-core  # internal/check alone — it must stay I/O-free
make bench      # the 200ms/100k-file target
make spike REPO=~/code/foo DEPTH=2   # re-run the Spike 01 drift probe (read-only)
```

## Success criterion

> `trestle check` fails on a real PR, at least once in the first month, for a reason that was not
> anticipated when the bindings were written.

If it never fires, it is decoration and should be deleted. This is deliberately falsifiable.
