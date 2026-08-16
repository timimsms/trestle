# Trestle — Design

## 1. Repo layout Trestle expects

```
repo/
├── docs/architecture/
│   ├── system.d2            # diagrams, bindings embedded as comments
│   ├── data-flow.d2
│   └── .trestle.yml         # config: discovery rules, render output
├── app/services/...         # the code being described
└── AGENTS.md                # points at CONVENTIONS.md
```

Trestle discovers `.trestle.yml` by walking up from CWD. Everything is relative to the directory containing it (the "root").

---

## 2. Binding syntax

Bindings are magic comments. D2's parser discards comments, so Trestle does an independent line scan — no fork of the D2 grammar, no risk of a binding breaking a render.

```d2
# @bind svc_billing  app/services/billing/**
# @bind svc_billing  app/jobs/billing/**
# @external stripe
# @ignore legacy_reporting  "deleted Q3, kept for the migration narrative"

svc_billing: Billing Service {
  shape: rectangle
}

stripe: Stripe
```

**Directives**

| Directive | Form | Meaning |
| --- | --- | --- |
| `@bind` | `@bind <node_id> <glob>` | Node is backed by code matching glob. Repeatable — multiple globs per node are ORed. |
| `@external` | `@external <node_id>` | Third-party system outside this repo. Suppresses `UNBOUND`. |
| `@infra` | `@infra <node_id>` | Your own infrastructure with no code representation — databases, queues, caches. Suppresses `UNBOUND`. |
| `@ignore` | `@ignore <node_id> "<reason>"` | Suppresses all violations for this node. Reason string is **required** — an unexplained suppression is how a check dies quietly. |

**Rules**

- Directives are position-independent. The node ID is explicit, so a directive does not have to sit adjacent to its node. This makes them safe for agents to append or reorder.
- Globs are repo-root-relative. `**` matches across directory boundaries.
- Node IDs are matched against D2 node IDs, including nested container paths (`platform.svc_billing`).
- One directive per line. No continuation syntax. Deliberately boring to parse.

---

## 3. Violation taxonomy

`trestle check` emits exactly five violation kinds. The set is small on purpose — every additional kind is another thing a user has to learn before they trust the exit code.

| Code | Trigger | Meaning | Default |
| --- | --- | --- | --- |
| `ORPHAN` | A `@bind` glob — or a `shared:` entry — matches zero files | The diagram or config claims something that no longer exists | **fail** |
| `UNMAPPED` | A path matched by a `discover:` rule is covered by no `@bind` glob | Code exists that the diagram never learned about | **fail** |
| `DANGLING` | A directive names a node ID absent from the `.d2` file | Binding outlived its node — usually a rename | **fail** |
| `UNBOUND` | A node has no `@bind`, no `@external`, no `@ignore` | Node of unknown provenance | **warn** (see O3) |
| `SYNTAX` | Malformed directive | — | **fail** |

Severity is overridable per-code in `.trestle.yml`. Nothing is silently suppressible without a written reason.

### Exit codes

| Code | Condition |
| --- | --- |
| `0` | No failures (warnings may be present) |
| `1` | One or more failing violations |
| `2` | Tool error — bad config, unparseable D2, I/O failure |

`2` is distinct from `1` so CI can tell "your diagram is wrong" apart from "Trestle is broken." Conflating them trains people to ignore both.

---

## 4. Config

```yaml
# .trestle.yml
version: 1

diagrams:
  - docs/architecture/*.d2

# Paths that MUST be covered by some binding.
# Each glob match is treated as one unit that needs an owner.
discover:
  - app/services/*/
  - app/adapters/*/

# Real code that is deliberately owned by no node.
# ENUMERATED, never blanket. `lib/**` here would reintroduce exactly the
# blindspot this tool exists to close. Entries are ORPHAN-checked: a stale
# entry for a deleted directory fails the build.
shared:
  - lib/http_client/**
  - lib/logging/**
  - app/middleware/**

exclude:
  - "**/*_test.*"
  - "**/vendor/**"
  - "**/node_modules/**"

severity:
  UNBOUND: warn

render:
  out: docs/architecture/rendered/
  layout: elk
  theme: 0
```

`discover` is explicit and per-repo (resolving O2 toward configuration over heuristics). A heuristic that guesses at service boundaries will be wrong in a way that generates noise, and a noisy check gets `--no-verify`'d within a week.

### `shared` vs `exclude` — not the same thing

| | Visible to `@bind` | Suppresses `UNMAPPED` | ORPHAN-checked | Means |
| --- | --- | --- | --- | --- |
| `exclude` | ❌ | ❌ (never seen) | ❌ | Not architecturally real: tests, vendored deps, generated output |
| `shared` | ✅ | ✅ | ✅ | Real code, deliberately owned by no single node |

Collapsing these would be a mistake. `exclude` is a blindspot by design; `shared` is an accountable exemption. A `shared` entry pointing at a directory that no longer exists fails the build like any other stale binding — which is what stops the shared layer from silently accumulating dead declarations.

### Overlapping bindings

Two nodes may bind the same path. This is legal and unremarkable — a directory containing both an invoice generator and a payment processor may legitimately appear as two boxes.

Overlap does **not** get a violation code. The taxonomy stays at five. It surfaces through `trestle explain --overlaps`, which lists paths claimed by more than one node, for when you suspect a copy-paste error. Informational only, never a failure.

---

## 5. CLI surface

Four commands. Resist adding a fifth.

```
trestle check [--format=human|json] [--strict]
    Validate bindings. The product. Exit code is the output that matters.
    --strict promotes all warnings to failures.

trestle render [--watch]
    Render diagrams to render.out via embedded D2.
    --watch rebuilds on save.

trestle explain <node_id>
    Show a node's bindings, the files each glob currently matches,
    and any violations. The debugging command — and the one an agent
    calls to orient before editing.

trestle explain --overlaps
    List paths claimed by more than one node. Informational.

trestle init
    Scaffold .trestle.yml, seed discover rules from detected repo layout,
    write CONVENTIONS.md and an AGENTS.md stanza.
```

### `check` output (human)

```
docs/architecture/system.d2

  ORPHAN    svc_billing
            @bind app/services/billing/** matches 0 files
            hint: renamed? `git log --diff-filter=D -- app/services/billing`

  UNMAPPED  app/services/notifications/
            no @bind glob covers this path
            hint: add `# @bind svc_notifications app/services/notifications/**`

2 failures, 0 warnings
```

Every violation carries a hint containing a runnable next step. A failing check that does not tell you what to type is a check people learn to route around.

---

## 6. Preview pane (conditional — see O5)

If built, the pane exists for exactly one reason: **overlay check status onto the rendered diagram.** Nodes with failures get a red outline; unbound nodes get a dashed one. Clicking a node runs `explain`.

Implementation: local HTTP server, static SVG, SSE for reload. No framework.

If the overlay slips or proves fiddly, cut the pane entirely and let `d2 --watch` own rendering. A preview that only shows what `d2 --watch` already shows is not worth a line of code.

---

## 7. Execution model

`check` is a pure function of (diagram files, config, filesystem listing). No network, no cache, no state. This is what makes it safe in CI and cheap to invoke repeatedly from an agent loop.

Performance target: **under 200ms on a 100k-file repo.** Achieved by walking the tree once, applying all globs against that single listing, rather than globbing per binding. If it takes longer than a lint rule, it gets moved to a nightly job, and a check that runs nightly stops blocking the PR that broke it.

---

## 8. Build order

1. Directive parser + `check` with `ORPHAN` and `DANGLING` only — no config file, hardcoded paths. Prove the loop end to end.
2. `.trestle.yml` + `discover` + `UNMAPPED`. This is the half that catches *new* code, and it is where the real value is.
3. `explain`.
4. `render` (embedded D2).
5. `init` + `CONVENTIONS.md` emission.
6. Preview pane, only if O5 resolves in favor.

Steps 1–2 are the MVP. Everything after is convenience.
