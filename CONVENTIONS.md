# Architecture Diagram Conventions

> This file ships **with the product** — `trestle init` writes it into the target repo
> and adds a pointer to `AGENTS.md`. It is the agent contract, not internal documentation.

Diagrams under `docs/architecture/` are source, not artifacts. The rendered SVGs are
generated. Never edit an SVG; edit the `.d2`.

---

## Node IDs

**IDs must match real code identifiers. Labels carry the human-readable name.**

```d2
svc_billing: Billing Service     # ✅ ID maps to app/services/billing
Billing Service                  # ❌ no stable ID to bind or reference
```

This is the single highest-leverage rule in this file. An ID that corresponds to
something real lets any tool — or any agent — cross-reference the diagram against the
codebase. An arbitrary ID makes the diagram a picture and nothing more.

- `snake_case` IDs.
- Prefix by kind: `svc_`, `db_`, `queue_`, `ext_`, `job_`.
- The ID should be greppable in the repo. If it isn't, it's probably wrong.

### Nodes inside containers

A node declared inside a container has a qualified ID — `svc_billing` inside `platform`
is really `platform.svc_billing`. **You can write either form in a directive.** The short
form matches as long as it is unambiguous:

```d2
# @bind svc_billing           app/services/billing/**   # ✅ resolves to platform.svc_billing
# @bind platform.svc_billing  app/services/billing/**   # ✅ also fine, and always unambiguous
```

If the short name matches nodes in two different containers, that is a `SYNTAX` error
naming both candidates — Trestle will not guess which one you meant. Qualify the ID to
fix it. Matching is on whole segments, so `billing` does **not** match `svc_billing`.

## Bindings

Every node needs exactly one of:

```d2
# @bind svc_billing app/services/billing/**     # backed by code here
# @infra db_primary                             # your infrastructure, no code to point at
# @external ext_stripe                          # third-party, outside this repo
# @ignore old_thing "kept for migration narrative until Q4"
```

**`@infra` and `@external` are not interchangeable.** `@external` means somebody else's
system — Stripe, Twilio, a partner API. `@infra` means *yours*, with no code in this repo
to bind to: a database, a queue, a cache. Marking your own Postgres `@external` says it
belongs to a third party, which is a lie in the direction that hides things.

`@ignore` requires a reason string. An unexplained suppression is how a check quietly
dies — if the reason is hard to write, that's a signal the suppression is wrong.

One directive per line. Whitespace between the fields is free, so aligning a block of
them into columns is fine and does not change how they parse.

## For agents editing these files

1. **Read the directives for a node before editing it.** They are plain comment lines, so
   `grep '@' docs/architecture/system.d2` shows you every binding in the file. Editing
   blind is how bindings rot.
2. **Edit by node ID. Never regenerate the file.** Whole-file rewrites produce diffs no
   human will review, which defeats the purpose of the format. Targeted edits only.
3. **Adding a node? Add its binding in the same edit.** A node without a binding is
   incomplete work, not a follow-up task.
4. **Run `trestle check` before declaring done.** Exit 0 or explain why not.
5. **Do not reposition, restyle, or reformat unrelated parts of the file.** Layout is
   the engine's job. Cosmetic churn hides real changes.
6. **Do not invent nodes to make a diagram look complete.** If the code isn't there, the
   box isn't there.
7. **Do not silence a finding you cannot fix.** Deleting a `discover:` rule, widening a
   glob until it swallows the complaint, or adding `@ignore` with a hollow reason all
   produce a green check that means nothing. Say the check fails and why — that is a
   better outcome than a passing check nobody can trust.

## What the check does not verify

`trestle check` ties boxes to code. **It does not check what the diagram says about them**, and a
green run is not evidence the diagram is true.

- **Edges are unverified.** Bindings are node→path, so the arrows are prose. An edge that never
  existed, or a deleted edge that still does, both pass.
- **Anything inside an already-bound directory is invisible.** Add a third-party integration to a
  service that already has a `@bind` and nothing fires — the directory still has an owner.
- **A gutted service still passes.** Move a service's logic elsewhere but leave one file behind and
  the glob keeps matching.

So: **the check is a floor, not a proof.** It reliably catches a box with nothing behind it and
code with no box — which is what a rename or a deletion produces, and those are the most common way
a diagram goes stale. It cannot tell you the diagram is *right*.

If you are an agent: a green check does not license "the diagram still matches." If you moved
responsibilities between services, say so and update the edges, even though nothing will fail if
you do not.

## Traps

- **A `;` is a statement separator in D2.** `tooltip: the pipeline; the fast one` is two
  statements, and the second becomes a **phantom node** with the prose as its ID. The
  diagram renders fine and the extra node is invisible in review. Keep semicolons out of
  labels and tooltips — an em dash or a comma reads the same. Trestle catches these as
  `UNBOUND`, which is how this rule got written.
- **An empty directory that a `discover:` rule matches always fails.** A unit is covered
  when at least one file under it is bound, so `mkdir app/services/inventory` with nothing
  in it fires `UNMAPPED`. Create the directory and its first file in the same change.

## For humans

- Sketch wherever is fastest — whiteboard, Excalidraw, paper. Then have an agent
  transcribe to D2 and throw the sketch away. Do not maintain both.
- Resist fighting the layout engine. If a diagram is unreadable, it usually has too many
  nodes, not bad layout. Split it.
- One diagram, one question. A diagram that answers three questions answers none well.

## Shared code

Most shared code should not appear on a system diagram at all. An HTTP client or a
logging wrapper that every service uses adds an edge from every node, turning the
diagram into a hairball while telling you nothing about the architecture.

Declare it in `.trestle.yml` under `shared:` and leave it off the canvas.

**The test for whether shared code deserves a node:** would you mention it by name in an
architecture review? `lib/pricing_engine` encodes a domain boundary — that's a node.
`lib/http_client` is plumbing — that's `shared:`.

When in doubt, leave it off. A node you later need is easy to add; a diagram nobody
reads because it has forty boxes is not recoverable.

**Enumerate `shared:` entries individually.** `lib/**` is not acceptable — it exempts
every future subsystem that lands there, including ones that genuinely needed a box.
List `lib/http_client/**`, `lib/logging/**`, and so on.

### `shared:` is not `exclude:`

Both keep code off the diagram; only one stays accountable.

| | Means | If the path disappears |
| --- | --- | --- |
| `exclude:` | Not architecturally real — tests, vendored code, generated output | Nothing. Never looked at |
| `shared:` | Real code, deliberately owned by no node | Fails the build, like any stale binding |

Reach for `exclude:` only when the answer to "is this part of the architecture?" is no.
Real code that simply has no owning box belongs in `shared:`, where a stale entry gets
caught. Using `exclude:` to quiet a finding is how a blindspot becomes permanent.

## Running it

```bash
trestle check     # what CI runs
```

CI runs `trestle check` on any PR touching `docs/architecture/` or paths named in
`discover`. A failure means the diagram and the code disagree — fixing either one is
valid, but they must be reconciled in that PR, not deferred.

`trestle explain` and `trestle render --watch` are described in the design docs but are
**not built yet**. If you are reading this in a repo that adopted Trestle early, `check`
is the whole tool.
