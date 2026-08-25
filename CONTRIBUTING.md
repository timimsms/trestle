# Contributing to Trestle

**Status: pre-1.0, all four commands built.** `check`, `explain`, `render` and `init` all work;
the command surface is closed at four and the violation taxonomy at five. What is still missing is
evidence: Trestle has run against fixtures, the worked example and its own repo, and not much
else. If you want to help, [`docs/DOGFOODING.md`](docs/DOGFOODING.md) is worth more right now than
a pull request — especially a report where nothing fired.

## Getting set up

Go 1.25+. No other prerequisite — D2 is embedded as a library, so there is no `d2` binary to
install.

```console
git clone git@github.com:timimsms/trestle.git
cd trestle
make            # fmt, vet, test, build
make self-check # Trestle checks Trestle, --strict
```

`make self-check` is the interesting one. It runs the tool against this repo using the only
configuration in it that was not written to make something pass.

## What to read, in order

| File | Why |
| --- | --- |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | Scope, non-goals, and the decision ledger. **Read the ledger before proposing anything.** |
| [`docs/planning/mvp/GAMEPLAN.md`](docs/planning/mvp/GAMEPLAN.md) | Architecture, resolved ambiguities, ranked risks |
| [`docs/DESIGN.md`](docs/DESIGN.md) | Binding syntax and check semantics |
| [`AGENTS.md`](AGENTS.md) | The working contract — applies to humans too |
| [`CONVENTIONS.md`](CONVENTIONS.md) | Diagram authoring. Ships with the product |

## Architecture

```
conventions.go          package `trestle`: go:embed of CONVENTIONS.md, nothing else.
cmd/trestle/            main + cobra wiring. Thin. No logic.
internal/
  config/               .trestle.yml load, validate, defaults, root discovery
  directive/            magic-comment line scanner
  nodes/                D2 AST -> node IDs (d2compiler)
  walk/                 the single filesystem walk. All I/O lives here.
  check/                THE PRODUCT. Pure. Zero I/O. Heavily tested.
  render/               D2 library wrapper (Phase 6)
  scaffold/             `trestle init` — layout detection, the emitted files (Phase 7)
  report/               human + json formatting, golden-tested
  expected/             fixture EXPECTED parser, shared by Phases 3 and 4
  integration/          cross-seam guards — where two packages must agree
testdata/repos/         ten fixture trees
examples/repairs-platform/   the worked example — a live test input, not a doc
spike/                  glob-binding-probe.sh (Gate A, keep for re-runs)
```

Dependency direction is one-way: `cmd` → everything; `check` → nothing but `config`,
`directive`, `nodes` types. `check` importing `walk` is a design failure, and CI should
eventually enforce that with an import-graph test.

**Restating the I/O rule precisely**, because the original phrasing contradicted itself: "all
filesystem I/O lives in `walk`" was written alongside explicit permission for `directive`,
`nodes`, and `config` to open their own named input file. The rule that is actually meant:

> **The repo walk lives in `walk`. `check` does no I/O at all.**

`directive`, `nodes`, and `config` each expose a pure `Parse(path, src []byte)` primary with a
thin `ParseFile` convenience. That is the shape they were built in, and it is the shape Phase 4
should wire.

**Pinned dependencies** (TECH_STACK, confirmed by Gate B):

| Concern | Module |
| --- | --- |
| D2 | `oss.terrastruct.com/d2 v0.7.2` — pinned exactly |
| CLI | `spf13/cobra` |
| Config | `goccy/go-yaml` |
| Globs | `bmatcuk/doublestar/v4` — non-negotiable, stdlib has no `**` |
| Walk | stdlib `io/fs.WalkDir` |
| Watch | `fsnotify/fsnotify` — Phase 6 only |
| Directives | stdlib. Line scan + `strings.Fields`. No parser generator. |

---

## The constraints, and why they are not negotiable by accident

These come from the ledger. They can change — but only by amending the ledger and saying why, not
by a PR that quietly steps around them.

- **Five violation codes.** Not four, not six. Five is the number people will learn before they
  trust an exit code. A new failure mode folds into an existing code or surfaces through
  `explain`.
- **Four top-level commands.** Same reasoning.
- **`internal/check` does no I/O.** It is a pure function of (listing, nodes, directives, config),
  and an import-graph test fails the build if that changes. If you need the filesystem in there,
  the seam is in the wrong place.
- **One filesystem walk.** Every glob applies to that single listing. The performance target
  depends on it, and a check slower than a lint rule gets moved to a nightly job — where it stops
  blocking the PR that broke it.
- **No hand-rolled D2 parser.** `d2compiler` works and is pinned. A grammar fork is a tar pit and
  is not the problem being solved.
- **Every violation carries a runnable hint.** Golden-tested. A failing check that does not tell
  you what to type is one people learn to route around.

## If a locked decision looks wrong

**Say so. Do not route around it.**

This is the single most useful thing a contributor can do here. L1–L12 were made before any code
existed and some of them are wrong. Four gaps were found exactly this way during the build (O8,
O9, O10, O11) — each because someone hit a wall and reported it instead of quietly coding around
it. An agent or a human who works around a bad decision destroys the evidence that it was bad.

Open an issue that names the decision, what you hit, and what it cost. That is a better
contribution than most patches.

## Pull requests

- **Tests ship with the logic**, in the same change. Not as a follow-up.
- `make` must be clean: `gofmt`, `go vet`, `go test`, `go build`. CI also runs `-race`,
  `golangci-lint` (pinned to v2.8.0 — the same version `make lint` runs), and `make self-check`.
- **New dependency? Say why stdlib was insufficient**, in the commit message.
- **Commit messages carry the reasoning, not the diff.** The diff is already in the commit; what
  is not recoverable later is why you chose this over the obvious alternative. `git log` in this
  repo is the design record.
- If you touch a `.d2`, [`CONVENTIONS.md`](CONVENTIONS.md) applies: edit by node ID, never
  regenerate the file, add the binding in the same change as the node, and do not restyle
  unrelated parts.

## Where help is most useful

1. **Dogfooding.** Point it at a repo under active structural change and report what happened —
   including and especially the findings you disagreed with. See
   [`docs/DOGFOODING.md`](docs/DOGFOODING.md).
2. **Answering O7.** Does enumerated `shared:` stay practical at scale? Every fixture has one or
   two entries, so this is completely untested. Counting shared subsystems in a real repo is a
   ten-minute contribution that could invalidate a locked decision.
3. **Hints that send you the wrong way.** They are golden-tested but were written against
   fixtures. The first one that misleads a real user is a bug worth filing.

## What not to build

Named here so they do not get re-proposed. Each is a deliberate exclusion with reasoning in
OVERVIEW — not a gap waiting to be filled.

- A diagram editor or WYSIWYG surface. Layout belongs to the layout engine; manual positioning
  re-breaks agent editability.
- A rendering engine. D2 renders; Trestle embeds it and gets out of the way.
- A hosted or multi-user product. Local-first CLI on a repo.
- Structurizr-style model/view separation. Real value, wrong time — it pays off only once there
  are enough views to contradict each other.
- The preview pane, unless O5 resolves in its favor. A preview showing what `d2 --watch` already
  shows is not worth a line of code.

## License

Trestle is [MIT licensed](LICENSE). Contributions are accepted under the same terms — there is no
CLA and no copyright assignment.
