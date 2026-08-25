# Trestle

**Keeps architecture diagrams honest by binding diagram nodes to real paths in the repo, and
failing CI when they diverge.**

Architecture diagrams drift. Not because people are careless, but because nothing connects the
diagram to the thing it describes. A service gets renamed, a module gets deleted, a new subsystem
appears — and the diagram stays exactly as correct-looking as it was the day it was drawn.

Diagram-as-code fixes *reviewability*. It does not fix *truth*. A D2 file can be perfectly
version-controlled and completely wrong.

```console
go install github.com/timimsms/trestle/cmd/trestle@v0.1.0
```

Go 1.25+, and nothing else — D2 is embedded, so there is no `d2` binary to install.

---

## What it looks like when it fires

A service was renamed. Nothing else in the toolchain noticed:

```console
$ trestle check

docs/architecture/system.d2

  ORPHAN    svc_reporting
            @bind app/services/reporting/** matches 0 files
            hint: renamed? `git log --diff-filter=D -- app/services/reporting`

.trestle.yml

  UNMAPPED  app/services/analytics/
            no @bind glob covers this path
            hint: add `# @bind svc_analytics app/services/analytics/**` to a diagram,
                  or add `app/services/analytics/**` to `shared:`

2 failures, 0 warnings · discover: 41 of 96 files
```

Two findings for one rename: the binding points nowhere, and the new directory has no owner.
**Every violation carries a runnable next step** — that is a golden-tested contract, not a
courtesy. A failing check that does not tell you what to type is one people learn to route around.

The last line reports how much of the repo is actually being watched. A green run over 4% of a
codebase should not look like a green run over all of it.

---

## The five codes

There are five, and there will not be a sixth. Five is the number people learn before they trust
an exit code.

| | fires when | default |
| --- | --- | --- |
| `ORPHAN` | a binding — or a `shared:` entry — matches no files | fail |
| `UNMAPPED` | code exists under a `discover:` rule that no box claims | fail |
| `DANGLING` | a directive names a node that is not in the diagram | fail |
| `UNBOUND` | a node has no directive of any kind | **warn** |
| `SYNTAX` | a malformed directive | fail |

`UNBOUND` warns rather than fails on purpose. The first time it fired in anger it was pointing at a
message queue — a modeling gap, not an error. Failing by default would have trained a suppression
reflex on the first diagram anyone wrote.

---

## Bindings live in the diagram

Not in a sidecar. A sidecar bindings file is itself a drift surface, and co-location turned out to
matter more than expected: in six controlled trials, agents that had **never been told Trestle
existed** maintained the bindings correctly, because reading the diagram means reading the
convention.

```d2
# @bind     svc_billing app/services/billing/**
# @bind     svc_billing app/jobs/billing/**
# @infra    db_primary
# @external ext_stripe

svc_billing: Billing Service
```

---

## Start here

| | |
| --- | --- |
| [**Design**](DESIGN.md) | Binding syntax, the five violation codes, config, the CLI surface |
| [**Decisions**](DECISIONS.md) | Why it is shaped this way — the locked ledger, the open questions, and the resolutions taken while building |
| [**Dogfooding**](DOGFOODING.md) | How to run a trial that can actually falsify the thing |

Source, issues and the README: [github.com/timimsms/trestle](https://github.com/timimsms/trestle).

---

## Trestle, checking Trestle

Every box below is bound to a Go package in this repo, and `make self-check` runs in CI. The
`lang` node exists because adding that package failed the check until the diagram learned about
it — the tool has caught its own author four times now.

<p align="center">
  <img src="img/trestle-self.svg" alt="Trestle's architecture: a CLI layer, an engine and four input packages, each bound to a Go package" width="900">
</p>

<sub>Rendered by <code>trestle render</code> through the embedded D2 library.</sub>

---

## Putting it in CI

It reads the file tree and nothing else — no database, no language runtime, no network:

```yaml
architecture:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: stable
    - run: go install github.com/timimsms/trestle/cmd/trestle@v0.1.0
    - run: trestle check --strict
```

Run it on **every** pull request rather than scoping it to diagram paths. The failure it catches
most often is a directory *appearing*, so path-scoping would switch it off for exactly the changes
that need it.

`--strict` promotes `UNBOUND` warnings to failures. Exit codes are `0` clean, `1` violations,
`2` tool error — `1` and `2` stay distinct so CI can tell "your diagram is wrong" from "Trestle is
broken."

## What a green check does not mean

Worth knowing before adopting it. Bindings are node→path, so:

- **Edges are never verified.** An arrow that was always false, or one that became false, passes.
- **Architecture added inside an already-bound directory is invisible.**
- **A service gutted to one remaining file keeps its binding.**

The check is a floor, not a proof. It catches a service moving or disappearing — the most common
way a diagram goes stale — and tells you exactly what to type when it fires.
