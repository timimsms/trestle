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

> **Status: MVP complete, at the stop gate.** `trestle check` works, in both formats, against
> all ten fixture repos, the worked example, and **this repo** — `make self-check` runs in CI.
> `explain`, `render` and `init` are not built and are deliberately not started until the MVP
> has been pointed at more real repos. See
> [`docs/planning/mvp/GAMEPLAN.md`](docs/planning/mvp/GAMEPLAN.md).
>
> The first non-fixture run found a node that did not exist in the source: a `;` inside a D2
> tooltip is a statement separator, so the compiler had silently turned trailing prose into a
> child node. It rendered fine and no reviewer would have caught it. That is the
> [success criterion](#success-criterion) firing — once, on the tool's own repo, which is not
> yet a track record.

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

## What a green check does *not* mean

Trestle ties **boxes to code**. It does not verify what the diagram says about those boxes, and
knowing the difference is the difference between trusting the exit code correctly and over-trusting
it.

| A green `trestle check` guarantees | It does not guarantee |
| --- | --- |
| Every node has code behind it | That the code does what the node claims |
| Code under a `discover:` rule has an owning node | That new architecture *inside* an already-bound directory is on the diagram |
| No binding points at a path that no longer exists | That a service still *is* a service — gut it but leave one file and the glob still matches |
| — | **That any edge on the diagram is true** |

That last row is the one that surprises people. Bindings are node→path, so the arrows — who calls
whom, which is most of what a system diagram communicates — are unverified prose. You can draw an
edge that never existed, or delete one that does, and the check stays green.

This is a deliberate boundary, not a gap to be filled. Verifying edges means call-graph analysis,
which is a different tool. What Trestle prevents is a narrower and still-common class of lie: a box
with nothing behind it, and code with no box. **A rename is the most frequent way a diagram goes
stale, and on that case the check is genuinely good** — it fails with the exact binding line to
paste.

Treat `trestle check` as a floor, not a proof.

## Commands

```
trestle check [--format=human|json] [--strict]   validate bindings — the product
trestle render [--watch]                         render via embedded D2
trestle explain <node_id> [--overlaps]           debug a node's bindings   (not built)
trestle init                                     scaffold config + conventions (not built)
```

Four. Resist adding a fifth.

`render` writes an SVG per diagram to `render.out`, using the layout engine and theme from
`.trestle.yml`. **No `d2` binary is required** — D2 is embedded, so the renderer and the parser
cannot disagree about a version. `--watch` re-renders on save, debounced, and keeps going through
the syntax errors that exist between one keystroke and the next.

## What Trestle is not

A diagram editor, a WYSIWYG surface, a rendering engine, a hosted product, a model/view system,
or a history layer. These are deliberate exclusions, not backlog items — see
[`OVERVIEW.md`](docs/planning/handoff/OVERVIEW.md) for why each one is out.

## Documentation

| File | Contains |
| --- | --- |
| [`CONVENTIONS.md`](CONVENTIONS.md) | The agent contract. Ships with the product. |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Setup, the constraints and why they hold, where help is useful |
| [`docs/DOGFOODING.md`](docs/DOGFOODING.md) | How to run the trial that decides whether this ships |
| [`docs/planning/mvp/GAMEPLAN.md`](docs/planning/mvp/GAMEPLAN.md) | Build plan, gate verdicts, architecture |
| [`docs/planning/mvp/phases/`](docs/planning/mvp/phases/) | Per-phase tasks and acceptance criteria |
| [`docs/planning/handoff/OVERVIEW.md`](docs/planning/handoff/OVERVIEW.md) | Scope, non-goals, decision ledger L1–L12 |
| [`docs/planning/handoff/DESIGN.md`](docs/planning/handoff/DESIGN.md) | Binding syntax, check semantics, CLI surface |
| [`docs/planning/handoff/TECH_STACK.md`](docs/planning/handoff/TECH_STACK.md) | Language, dependencies, layout, testing |
| [`examples/repairs-platform/`](examples/repairs-platform/) | Worked example — a live test input |

## Development

```console
make             # fmt, vet, test, build
make self-check  # Trestle checks Trestle, --strict
make test-core   # internal/check alone — it must stay I/O-free
make bench       # the 200ms/100k-file target
make spike REPO=~/code/foo DEPTH=2   # re-run the Spike 01 drift probe (read-only)
```

Go 1.25+, and nothing else — D2 is embedded as a library, so there is no `d2` binary to install.

## License

[MIT](LICENSE).

## Success criterion

> `trestle check` fails on a real PR, at least once in the first month, for a reason that was not
> anticipated when the bindings were written.

If it never fires, it is decoration and should be deleted. This is deliberately falsifiable, and
it is the MVP's evaluation gate rather than a slogan — which is why
[`docs/DOGFOODING.md`](docs/DOGFOODING.md) exists and why a dogfood report where nothing fired is
still worth filing.
