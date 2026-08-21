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

1. **Run `trestle explain <node_id>` before editing a node.** It shows the node's bindings,
   how many files each glob matches right now, and any violations against it. Run
   `trestle explain` with no argument to see every node the tool parsed — that is how you
   confirm the diagram contains what you think it contains, rather than inferring it from a
   check that reported nothing. Editing blind is how bindings rot.
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

## When a service changes shape

Most guidance about diagrams covers adding, removing, renaming, splitting and merging. There is a
sixth case that comes up just as often and is easier to get wrong:

**A service can lose a responsibility without losing its box.**

You fold one of its jobs into a caller, or move a chunk of logic elsewhere, and what is left is
smaller but still real. The right edit is usually **not** to delete the node:

- **Any code left behind still needs an owner.** If the directory is matched by a `discover:` rule
  and you delete its node, you have traded a tidy diagram for an `UNMAPPED` violation on code that
  somebody still maintains.
- **Relabel the edges instead.** An arrow that said `builds quote` and now means `reads the rate
  card` is the part that actually went stale. So are tooltips and labels describing what the box
  does.
- **Nothing will fail if you skip this.** `trestle check` cannot see edge labels or tooltips — the
  node still exists and its binding still matches, so the check stays green while the diagram is
  now wrong. See below.

Delete the node only when the code is genuinely gone. Shrinking is not disappearing.

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

## Seeing what Trestle parsed

```bash
trestle explain                    # every node, with its binding status
trestle explain svc_billing        # one node: bindings, what they match, violations
trestle explain --overlaps         # paths claimed by more than one node
trestle explain --format=json      # the same, for a program
```

`explain` never fails. It exits 0 whatever it finds, so a node with violations still prints;
`trestle check` is the command with an opinion. The one exception is a node ID that names
nothing, which exits 2 — you asked about a box that is not there.

The status column is the one-word answer to "does the tool know what is behind this box":

| Status | Means |
| --- | --- |
| `bound` | has a `@bind`. The glob and its **current match count** are printed next to it |
| `external` / `infra` | marked as somebody else's system, or as yours with no code here |
| `ignored` | `@ignore`, with the reason printed so the suppression keeps justifying itself |
| `container` | a grouping box with no directive of its own. Never reports `UNBOUND` |
| `unbound` | a leaf with no directive at all. This is what a phantom node looks like |

**`matches 0 files` is the line to look for.** It prints whether or not `ORPHAN` is switched
on — the count is evidence, and evidence outlives the violation. Anything Trestle could not
resolve to exactly one node is listed under **unresolved directives** at the bottom: a `@bind`
naming a node that is not in the file, an ID that suffix-matches two nodes, or a line that did
not parse. A binding that resolved to nothing is invisible among the nodes, so that section is
the other half of the inventory.

### For agents: the JSON

`--format=json` is the shape to read before editing a diagram. One schema for all three
questions, `"version": 1`, arrays that are `[]` and never null:

```json
{
  "version": 1,
  "kind": "inventory",
  "query": null,
  "diagrams": ["docs/architecture/system.d2"],
  "disabled": [],
  "summary": { "nodes": 12, "bound": 5, "unbound": 1, "overlaps": 0, "failures": 0, "warnings": 1 },
  "nodes": [
    {
      "id": "platform.svc_billing",
      "status": "bound",
      "files": 3,
      "bindings": [{ "glob": "app/services/billing/**", "matches": 3, "files": null }],
      "violations": []
    }
  ],
  "overlaps": [],
  "unresolved": []
}
```

- `kind` is `inventory`, `node` or `overlaps`. `nodes` and `overlaps` hold the answer to the
  question asked; `summary`, `diagrams` and `disabled` always describe the whole repo.
- A binding's `files` array is populated only for `trestle explain <node_id>`. **`null` is not
  `[]`** — `[]` means the glob claims nothing, `null` means the list was not part of this answer.
- `disabled` names codes set to `off` in `.trestle.yml`. A green `trestle check` on a repo with a
  non-empty `disabled` is a check that was told not to look.
- Node IDs are always fully qualified here, even where the directive wrote the short form.

## Traps

- **A `;` is a statement separator in D2.** `tooltip: the pipeline; the fast one` is two
  statements, and the second becomes a **phantom node** with the prose as its ID. The
  diagram renders fine and the extra node is invisible in review. Keep semicolons out of
  labels and tooltips — an em dash or a comma reads the same. Trestle catches these as
  `UNBOUND`, and `trestle explain` lists them as `unbound` nodes with the prose as the ID,
  which is how this rule got written.
- **An empty directory that a `discover:` rule matches always fails.** A unit is covered
  when at least one file under it is bound, so `mkdir app/services/inventory` with nothing
  in it fires `UNMAPPED`. Create the directory and its first file in the same change.
- **A directive cannot be commented out.** The scanner strips the leading run of `#`
  before it looks for `@`, so `## @bind svc_x app/x/**` is still a live binding, and so is
  a directive you thought you had disabled by adding a marker. To turn a binding off,
  **delete it** — or write `@ignore` with a real reason, which is the supported way to say
  "not now" and keeps the suppression justifying itself. The same rule makes it safe to
  *mention* a directive inside prose (`the binding is # @bind svc_x app/x/**`), because
  only a line whose first non-`#` character is `@` is read as one.

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
trestle check      # what CI runs
trestle explain    # what you run when check surprises you
trestle render     # SVGs, via embedded D2 — no `d2` binary needed
trestle init       # scaffold a repo. Run once, at the start
```

CI runs `trestle check` on any PR touching `docs/architecture/` or paths named in
`discover`. A failure means the diagram and the code disagree — fixing either one is
valid, but they must be reconciled in that PR, not deferred.

Four commands, and there will not be a fifth.

### If this repo was just scaffolded

`trestle init` writes the starter diagram **empty**. That is deliberate: a diagram
generated from the directory listing would pass its own check — Trestle having written
both sides of the comparison — while saying nothing about how anything fits together.

So the first `trestle check` reports one `UNMAPPED` per discovered directory. **That is an
inventory, not a verdict.** Each finding carries the exact `@bind` line to paste, and
working the list down is how the first diagram gets written: a box for each thing you
would name in an architecture review, a `shared:` entry for each thing you would not.
