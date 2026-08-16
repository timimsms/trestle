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

## Bindings

Every node needs exactly one of:

```d2
# @bind svc_billing app/services/billing/**     # backed by code here
# @external ext_stripe                          # deliberately outside this repo
# @ignore old_thing "kept for migration narrative until Q4"
```

`@ignore` requires a reason string. An unexplained suppression is how a check quietly
dies — if the reason is hard to write, that's a signal the suppression is wrong.

## For agents editing these files

1. **Run `trestle explain <node_id>` before editing a node.** It shows current bindings
   and what they match. Editing blind is how bindings rot.
2. **Edit by node ID. Never regenerate the file.** Whole-file rewrites produce diffs no
   human will review, which defeats the purpose of the format. Targeted edits only.
3. **Adding a node? Add its binding in the same edit.** A node without a binding is
   incomplete work, not a follow-up task.
4. **Run `trestle check` before declaring done.** Exit 0 or explain why not.
5. **Do not reposition, restyle, or reformat unrelated parts of the file.** Layout is
   the engine's job. Cosmetic churn hides real changes.
6. **Do not invent nodes to make a diagram look complete.** If the code isn't there, the
   box isn't there.

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

## Rendering

```bash
trestle render --watch     # local iteration
trestle check              # what CI runs
```

CI runs `trestle check` on any PR touching `docs/architecture/` or paths named in
`discover`. A failure means the diagram and the code disagree — fixing either one is
valid, but they must be reconciled in that PR, not deferred.
