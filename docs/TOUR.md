# A tour of Trestle

This walks a repository from nothing to a working check, and then breaks it on purpose. It is the
long version of the README: same claims, but you get to watch each one happen.

Everything below was run against a throwaway monorepo — a TypeScript ledger service, nine source
files — and every block is output from that run, elided only where a line is marked as such. The
binary came from source:

```console
$ go build -o trestle ./cmd/trestle
```

Nothing else is needed. D2 is embedded in the binary, so there is no `d2` to install and no chance
of the parser and the renderer disagreeing about a version.

---

## 1. A diagram that is wrong and looks right

Here is the diagram that has been in `docs/architecture/` for six months.

```d2
direction: right

api: Public API
ledger: Ledger Core
payouts: Payouts
reporting: Reporting

db_primary: Postgres {
  shape: cylinder
}

ext_stripe: Stripe

api -> ledger: posts entries
ledger -> db_primary: persists
payouts -> ledger: reads balances
payouts -> ext_stripe: initiates transfer
reporting -> db_primary: nightly rollups
```

It is valid D2. It renders without a warning. It is in version control, it was reviewed in a PR,
and every tool in the repository is happy with it. Now the repository:

```console
$ ls packages/
http-client
ledger
logging
notifications
payouts
```

There is no `reporting`. It was deleted last quarter and the box outlived it. And `notifications`
— which every payout goes through — has no box at all, because it was added by someone who did not
know the diagram existed.

Neither of those is a typo, a lint error, or a rendering problem. There is nothing in the file to
check against anything, which is why nothing caught it. Trestle's entire proposition is to add the
missing statement: *this box is that code*, written down, in the diagram, where a checker can read
it.

---

## 2. `trestle init`

`init` proposes and does not impose. `--dry-run` prints the proposal and writes nothing.

```console
$ trestle init --dry-run
trestle init — /tmp/ledger

`discover:` rules seeded from the layout found here. Every directory one of
these matches needs an owner — a `@bind` on some diagram, or an entry in
`shared:` — so these rules decide how much the first `trestle check` has to
say. Delete the ones that are not architecture.

  packages/*/  5 directories  http-client, ledger, logging, notifications, payouts
  apps/*/      1 directory    api

Files:

  create    .trestle.yml
  create    docs/architecture/system.d2
  create    CONVENTIONS.md
  create    AGENTS.md

The starter diagram is written with no nodes in it. Trestle does not invent
boxes: a diagram generated from your directory listing would pass its own
check while telling you nothing.
So the first `trestle check` will report 6 UNMAPPED — one per
directory above — and each one carries the exact `@bind` line to paste
into the diagram. That is the to-do list, not a verdict on the repo.

--dry-run: 4 files would be written, none were.
```

The `discover:` rules are the part of that screen to read carefully, and the proposal is the moment
to narrow them. Deleting a rule here because you can see it matched `.cache` is triage; deleting it
in a fortnight because the check has been noisy is the failure mode the whole design is trying to
avoid. They are seeded at depth 2 — `packages/ledger`, not `packages/ledger/src` — because a probe
against a 4,000-file repo found depth 2 lands on units that correspond to boxes somebody would
actually draw, while depth 3 fragmented the same repo into 118 units, 35 of them holding two files
or fewer.

Run it for real and it prompts. Nothing existing is ever overwritten, so running `init` twice is
safe.

```console
$ trestle init
  [the same proposal, elided]

Write 4 files? [y/N] y

  wrote        .trestle.yml
  wrote        docs/architecture/system.d2
  wrote        CONVENTIONS.md
  wrote        AGENTS.md

Next:

  trestle check      6 directories to account for. Each UNMAPPED names the
                     `@bind` line that fixes it; paste it into the diagram, or
                     put the path in `shared:` if it is plumbing nobody draws.
  trestle explain    every node Trestle parsed, and what each binding matches
  trestle render     SVGs into docs/architecture/rendered/

Read CONVENTIONS.md before editing a diagram. It is the contract.
```

**The starter diagram it wrote has no nodes in it.** This surprises people, and the reason is worth
stating plainly: a diagram generated from your own directory listing cannot disagree with that
listing. Trestle would have written both sides of the comparison, so the first check would pass and
mean nothing — green from day one and green through every kind of drift that matters, because the
boxes were never a claim anybody made.

The edges cannot be generated at all. An import graph tells you what calls what; it does not tell
you what the architecture *means*, which is what the arrows are for.

So `init` gives you an empty file and a to-do list. The diagram is authored.

---

## 3. The first `check` is a to-do list

```console
$ trestle check
.trestle.yml

  UNMAPPED  apps/api/
            no @bind glob covers this path
            hint: add `# @bind api apps/api/**` to a diagram, or add `apps/api/**` to `shared:`

  UNMAPPED  packages/http-client/
            no @bind glob covers this path
            hint: add `# @bind http_client packages/http-client/**` to a diagram, or add `packages/http-client/**` to `shared:`

  UNMAPPED  packages/ledger/
            no @bind glob covers this path
            hint: add `# @bind ledger packages/ledger/**` to a diagram, or add `packages/ledger/**` to `shared:`

  UNMAPPED  packages/logging/
            no @bind glob covers this path
            hint: add `# @bind logging packages/logging/**` to a diagram, or add `packages/logging/**` to `shared:`

  UNMAPPED  packages/notifications/
            no @bind glob covers this path
            hint: add `# @bind notifications packages/notifications/**` to a diagram, or add `packages/notifications/**` to `shared:`

  UNMAPPED  packages/payouts/
            no @bind glob covers this path
            hint: add `# @bind payouts packages/payouts/**` to a diagram, or add `packages/payouts/**` to `shared:`

6 failures, 0 warnings · discover: 9 of 14 files
```

Exit code 1. This is not a failed setup — it is an inventory of everything the repository has that
nobody has drawn yet, and working it down is how the first diagram gets written.

Three things to notice.

**The findings are grouped under `.trestle.yml`, not under the diagram.** An `UNMAPPED` is a fact
about a `discover:` rule that went unsatisfied. No single diagram is more to blame than another, so
the file that declared the rule owns the finding.

**Every hint is paste-ready.** The hint on `apps/api/` contains the literal directive —
`# @bind api apps/api/**` — node ID and all, derived from the directory's own name. That is a
golden-tested contract across all five violation codes: a failing check that does not tell you what
to type is one people learn to route around.

**The summary line says how much was looked at.** `discover: 9 of 14 files` — 9 of the 14 files in
this repo are inside a `discover:` unit and therefore need an owner. That clause exists because two
early field trials reached a green check while watching 0 and 27 files respectively, and nothing in
the output said so. A green result must never be able to mean *nothing was inspected*.

If you are triaging a first run on a real repository, `--format=json` is easier to tally, and
[`DOGFOODING.md`](DOGFOODING.md) has the protocol — including the instruction to write the counts
down before you fix anything.

---

## 4. Writing the diagram

Start with two boxes. A binding is a comment; the node is a line of D2; they belong in the same
edit.

```d2
# Ledger — service topology.

# @bind  api     apps/api/**
# @bind  ledger  packages/ledger/**

direction: right

api: Public API
ledger: Ledger Core

api -> ledger: posts entries
```

```console
$ trestle check
.trestle.yml

  UNMAPPED  packages/http-client/
  UNMAPPED  packages/logging/
  UNMAPPED  packages/notifications/
  UNMAPPED  packages/payouts/

4 failures, 0 warnings · discover: 9 of 14 files
```

<sub>Detail and hint lines elided here and in the next block. They are identical in form to the
ones in section 3.</sub>

Six down to four. Bindings are repeatable and OR together, so a subsystem spread across two
directories is one node with two `@bind` lines — not two boxes.

### Boxes that are not code

Add the rest of the services, plus Postgres and Stripe:

```console
$ trestle check
.trestle.yml

  UNMAPPED  packages/http-client/
  UNMAPPED  packages/logging/

docs/architecture/system.d2

  UNBOUND   db_primary  (warn)
            no @bind, @external, @infra or @ignore
            hint: add one of `# @bind db_primary <glob>`, `# @infra db_primary`, `# @external db_primary`, or `# @ignore db_primary "<reason>"` — for a database or queue the answer is usually `@infra`

  UNBOUND   ext_stripe  (warn)
            no @bind, @external, @infra or @ignore
            hint: add one of `# @bind ext_stripe <glob>`, `# @infra ext_stripe`, `# @external ext_stripe`, or `# @ignore ext_stripe "<reason>"` — for a database or queue the answer is usually `@infra`

2 failures, 2 warnings · discover: 9 of 14 files
```

`UNBOUND` warns rather than fails, and this is exactly why: neither of these is an error. They are
boxes with no code in this repository, and the tool is asking which kind. (The tail of that hint is
the same sentence for every node, including `ext_stripe`. It lists all four directives; the closing
advice is generic, not a reading of your node ID.)

```d2
# @infra    db_primary
# @external ext_stripe
```

**These are not interchangeable.** `@external` means somebody else's system — Stripe, Twilio, a
partner API. `@infra` means yours, with no code here to point at: a database, a queue, a cache.
Marking your own Postgres `@external` says it belongs to a third party, which is a lie in the
direction that hides things. (There is a fourth directive, `@ignore`, which suppresses everything
for a node and requires a written reason. See [`DESIGN.md`](DESIGN.md).)

### `shared:` — real code that no box owns

Two units are left: `http-client` and `logging`. Both are real code. Neither is architecture.

The test is: would you name it out loud in an architecture review? `packages/ledger` yes.
`packages/http-client` no — putting it on the canvas draws an edge from every other box and turns
the diagram into a hairball. So it goes in `.trestle.yml`:

```yaml
shared:
  - packages/http-client/**
  - packages/logging/**
```

```console
$ trestle check
0 failures, 0 warnings · discover: 9 of 14 files
```

Exit 0. Two things about `shared:` that matter later:

**It is enumerated, never blanket.** `packages/**` would be quicker and would also exempt every
future subsystem that lands there, including ones that genuinely needed a box. Trestle rejects a
blanket entry with a config error and exit 2 rather than accepting it.

**It stays accountable.** A `shared:` entry pointing at a path that no longer exists is an
`ORPHAN` and fails the build like any other stale binding. That is the whole difference from
`exclude:`, which is a blindspot by design — tests, vendored code, generated output, never looked
at again. Reach for `exclude:` only when the answer to "is this part of the architecture?" is no.
Using it to quiet a finding is how a blindspot becomes permanent.

---

## 5. `explain` — what the tool actually sees

`check` has an opinion. `explain` does not: it exits 0 whatever it finds, and its job is to answer
"does Trestle see what I think it sees".

```console
$ trestle explain
docs/architecture/system.d2

  bound     api            apps/api/** — 2 files
  bound     ledger         packages/ledger/** — 2 files
  bound     payouts        packages/payouts/** — 2 files
  bound     notifications  packages/notifications/** — 1 file
  infra     db_primary
  external  ext_stripe

6 nodes: 4 bound, 1 external, 1 infra
```

The match counts are the point of the command. A count is printed next to every binding whether or
not `ORPHAN` is switched on, and a zero there is the line to look for: the count is evidence, and
evidence outlives the violation that would have reported it.

One node at a time gives you the files:

```console
$ trestle explain payouts
docs/architecture/system.d2

  bound     payouts
            label "Payouts", shape rectangle, declared line 14
            @bind packages/payouts/** (docs/architecture/system.d2:5) matches 2 files
              packages/payouts/src/schedule.ts
              packages/payouts/src/transfer.ts
            no violations
```

And `--overlaps` lists paths claimed by more than one node. Overlap is legal — a directory holding
both an invoice generator and a payment processor may honestly be two boxes — so it gets no
violation code and appears only here.

```console
$ trestle explain --overlaps
no path is claimed by more than one node
```

### Why this command exists

Add a tooltip to the ledger node, exactly as you would write it in English:

```d2
ledger: Ledger Core {
  tooltip: double-entry; the only place money moves
}
```

```console
$ trestle explain
docs/architecture/system.d2

  bound     api                                apps/api/** — 2 files
  bound     ledger                             packages/ledger/** — 2 files
  unbound   ledger.the only place money moves  UNBOUND
  bound     payouts                            packages/payouts/** — 2 files
  bound     notifications                      packages/notifications/** — 1 file
  infra     db_primary
  external  ext_stripe

7 nodes: 4 bound, 1 external, 1 infra, 1 unbound
0 failures, 1 warning — `trestle check`
```

Seven nodes. There are six boxes in that file.

**`;` is a statement separator in D2.** The compiler split the tooltip and turned the trailing
prose into a child node called `ledger.the only place money moves`. It renders without complaint,
the extra node is invisible in a diff, and no reviewer would catch it:

```console
$ trestle render
docs/architecture/system.d2 -> docs/architecture/rendered/system.svg
1 diagram rendered
```

This is the one catch so far that nobody anticipated, and it was found on Trestle's own diagram.
Nothing about it is a Trestle feature — it fell out of asking a question no linter or renderer
asks: *what code is behind this box?* Keep semicolons out of labels and tooltips; a comma or an em
dash reads the same.

It arrives as `UNBOUND`, which is a warning, so it does not fail a build by default:

```console
$ trestle check --strict
docs/architecture/system.d2

  UNBOUND   ledger.the only place money moves  (warn)
            no @bind, @external, @infra or @ignore
            hint: add one of `# @bind ledger.the only place money moves <glob>`, `# @infra ledger.the only place money moves`, `# @external ledger.the only place money moves`, or `# @ignore ledger.the only place money moves "<reason>"` — for a database or queue the answer is usually `@infra`

0 failures, 1 warning · discover: 9 of 15 files
--strict: warnings count as failures
```

Exit 1 under `--strict`, exit 0 without it. That trade-off is deliberate — `UNBOUND` fires often
enough on genuine modeling gaps, boxes that are honestly not code, that failing by default would
train a suppression reflex on the first diagram anybody writes. It is also why CI should run
`--strict`.

Replace the `;` with a comma and the seventh node goes away. The rest of the tour is back to six.

---

## 6. `render`

```console
$ trestle render
docs/architecture/system.d2 -> docs/architecture/rendered/system.svg
1 diagram rendered
```

One SVG per diagram, into the directory named by `render.out` in `.trestle.yml`, using the layout
engine and theme configured there. They are generated artifacts: add the directory to
`.gitignore` and edit the `.d2`, never the SVG.

`--watch` re-renders on save and keeps going through the syntax errors that exist between one
keystroke and the next, which is most of them:

```console
$ trestle render --watch
docs/architecture/system.d2 -> docs/architecture/rendered/system.svg
1 diagram rendered
watching 1 file(s); ctrl-c to stop
docs/architecture/system.d2 -> docs/architecture/rendered/system.svg
render /tmp/ledger/docs/architecture/system.d2: /tmp/ledger/docs/architecture/system.d2:30:1: connection missing destination
/tmp/ledger/docs/architecture/system.d2:30:11: maps must be terminated with }
docs/architecture/system.d2 -> docs/architecture/rendered/system.svg
```

Broken, then fixed, without the watcher dying. Note what rendering tells you about accuracy:
nothing. A diagram that renders is a diagram that parses.

---

## 7. Make it fail

This is the case Trestle is actually for. Somebody renames a directory.

```console
$ git mv packages/payouts packages/disbursements
$ git commit -m "rename payouts to disbursements"
```

Nothing else changes. The code all works, the tests pass, the diagram still renders, and the PR
looks clean.

```console
$ trestle check
.trestle.yml

  UNMAPPED  packages/disbursements/
            no @bind glob covers this path
            hint: add `# @bind disbursements packages/disbursements/**` to a diagram, or add `packages/disbursements/**` to `shared:`

docs/architecture/system.d2

  ORPHAN    payouts
            @bind packages/payouts/** matches 0 files
            hint: renamed? `git log --diff-filter=D -- packages/payouts`

2 failures, 0 warnings · discover: 9 of 15 files
```

Two findings, one rename. They are the two halves of the same fact and they come from different
files, which is why both are reported: the diagram claims code that is gone, and code exists that
the diagram never learned about. Either one alone would be ambiguous. Together they say *this
moved*.

The hint on the `ORPHAN` is runnable, and it answers the question you are about to ask:

```console
$ git log --diff-filter=D -- packages/payouts
commit ece1a41a61d548dba10a3b375b96c88fafa11a9a
Author: Tour <tour@example.com>
Date:   Mon Aug 24 23:59:38 2026 -0700

    rename payouts to disbursements
```

There is the commit, the author, and the date the box stopped being true.

Now fix it — and get it half wrong first, because this is the mistake people make. Rename the node
but forget the directive:

```console
$ trestle check
.trestle.yml

  UNMAPPED  packages/disbursements/
            no @bind glob covers this path
            hint: add `# @bind disbursements packages/disbursements/**` to a diagram, or add `packages/disbursements/**` to `shared:`

docs/architecture/system.d2

  DANGLING  payouts
            @bind names a node that is not in docs/architecture/system.d2
            hint: no node named `payouts` in docs/architecture/system.d2 — delete the directive at docs/architecture/system.d2:5, or add the node

  UNBOUND   disbursements  (warn)
            no @bind, @external, @infra or @ignore
            hint: add one of `# @bind disbursements <glob>`, `# @infra disbursements`, `# @external disbursements`, or `# @ignore disbursements "<reason>"` — for a database or queue the answer is usually `@infra`

2 failures, 1 warning · discover: 9 of 15 files
```

`ORPHAN` became `DANGLING` — the binding no longer names a node that exists — and the renamed node
picked up an `UNBOUND` warning of its own. The hint names the file and line of the directive to
delete. Update both sides and it goes quiet:

```d2
# @bind  disbursements  packages/disbursements/**

disbursements: Disbursements
```

```console
$ trestle check
0 failures, 0 warnings · discover: 9 of 15 files
```

A rename is the most common way a diagram goes stale, and on that case the check is genuinely
good. What it took to catch was one line of comment written six months earlier.

---

## 8. What it will not catch

The README commits to this and the tour would be dishonest without it. Trestle binds **boxes to
code**. It does not verify anything the diagram *says* about those boxes.

### New architecture inside an already-bound directory

Add a file to `packages/ledger`, one that calls a third-party FX API on every cross-currency entry
— a new external dependency, a new arrow on any diagram worth reading:

```console
$ trestle check
0 failures, 0 warnings · discover: 10 of 16 files
```

Green. The directory already has an owner, so nothing fires. The only trace anywhere in the tool
is the file count:

```console
$ trestle explain ledger
docs/architecture/system.d2

  bound     ledger
            label "Ledger Core", shape rectangle, declared line 13
            @bind packages/ledger/** (docs/architecture/system.d2:4) matches 3 files
              packages/ledger/src/balances.ts
              packages/ledger/src/entries.ts
              packages/ledger/src/fx.ts
            no violations
```

Two files became three. Nothing failed, and nothing was going to.

### Edges

Add two arrows that describe things that have never happened:

```d2
notifications -> db_primary: reads templates
api -> ext_stripe: forwards webhooks
```

```console
$ trestle check
0 failures, 0 warnings · discover: 10 of 16 files
```

Green. Bindings are node→path, so the arrows — who calls whom, which is most of what a system
diagram communicates — are unverified prose. You can draw an edge that never existed, or delete
one that does, and nothing downstream will contradict you.

This is a boundary, not a gap waiting to be filled. Verifying edges means call-graph analysis,
which is a different tool with a different cost. It does mean **a missing edge is a gap, and a
confident wrong edge is a lie nothing will catch** — so edges are the part of a diagram that needs
a human who knows why the system is shaped the way it is.

### A gutted service

Delete every file in a bound directory but one and the glob keeps matching:

```console
$ trestle check
0 failures, 0 warnings · discover: 9 of 15 files
```

A service can lose most of its responsibilities without losing its box. The right response is
usually to relabel the edges rather than delete the node — and nothing will fail if you skip it.

**Treat `trestle check` as a floor, not a proof.** It reliably catches a box with nothing behind
it, and code with no box. That is what a rename or a deletion produces, and those are the most
common way a diagram goes stale. It cannot tell you the diagram is right.

---

## 9. Wiring it into CI

The check is a pure function of the diagrams, the config and one filesystem walk. No network, no
cache, no state — which is what makes it safe to run on every PR.

```yaml
name: Architecture

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  trestle:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        # The ORPHAN hint is `git log --diff-filter=D`, which needs history to
        # answer. Shallow clones make the hint print but return nothing.
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      # Pin the version. An architecture check that changes what it accepts
      # without a PR is one people stop trusting.
      - run: go install github.com/timimsms/trestle/cmd/trestle@v0.1.0

      # --strict promotes warnings to failures. UNBOUND is a warning by
      # default, and UNBOUND is what a phantom node looks like.
      - run: trestle check --strict
```

Exit codes are `0` clean, `1` violations, `2` tool error. `1` and `2` stay distinct so CI can tell
"your diagram is wrong" from "Trestle is broken" — conflating them trains people to ignore both.

If you want to gate only PRs that touch architecture, add `paths:` for `docs/architecture/**` and
whatever your `discover:` rules cover. Running it on everything is cheaper than it sounds: the
target is under 200ms on a 100k-file repo, achieved by walking the tree once and applying every
glob to that single listing.

For anything reading the result rather than looking at it, `--format=json` carries the same
findings with the hint attached to each one, plus two fields the human output only summarizes:
`coverage`, which is the `discover: 9 of 15 files` clause broken out, and `disabled`, which names
any code set to `off` in `.trestle.yml`. A green check on a repo with a non-empty `disabled` is a
check that was told not to look, which is why it travels with the result instead of staying in the
config. [`DOGFOODING.md`](DOGFOODING.md) has a `jq` one-liner for tallying a first run.

---

## Where to go next

- [`../CONVENTIONS.md`](../CONVENTIONS.md) — the authoring contract. `init` writes a copy into your
  repo; it is what you hand an agent along with the diagram.
- [`DESIGN.md`](DESIGN.md) — the five violation codes in full, the config reference, and the CLI
  surface.
- [`DECISIONS.md`](DECISIONS.md) — why the boundaries are where they are. The exclusions in
  section 8 are entries in that ledger, not omissions.
- [`DOGFOODING.md`](DOGFOODING.md) — the protocol for pointing this at a repo you care about,
  including how to count a first run honestly and what to record when it fires.
- [`../examples/repairs-platform/`](../examples/repairs-platform/) — a worked example with
  containers, a queue, an `@ignore` with a real reason, and a diagram with edges someone thought
  about.

The success criterion this project is judged against is that `trestle check` fails on a real PR,
for a reason nobody anticipated when the bindings were written. If it never does that, it is
decoration. That is a falsifiable claim on purpose, and the only way to test it is to run it
somewhere it was not designed.
