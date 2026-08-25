# Changelog

Notable changes per release. Anything that changes what `trestle check` reports on an unchanged
repo is called out as **behaviour change**, because that is the line between a release you can take
without reading and one you cannot.

## v0.1.1 — 2026-08-25

Hints and detection. No change to what `check` reports on an unchanged repo — only to what it
proposes, and to what its hints say.

All three fixes came from the same place: **writing the guided tour**. Walking the tool end to end
as a newcomer found things that eleven controlled agent trials and three field trials had not,
which is a decent argument for the tour existing.

### Fixed

- **A JS `lib/` layer was undiscoverable.** `init` proposed `packages/*/`, `apps/*/` and `src/*/`
  for JS repos but not `lib/*/`, which was Rails-only — so a repo without workspaces got nothing
  proposed for its shared layer and `UNMAPPED` could never fire there. The coverage clause does not
  catch this: the other rules matched fine, so the number reads healthy while a whole directory
  goes unwatched. ([#29](https://github.com/timimsms/trestle/pull/29))
- **The blanket-`shared:` hint invented directory names.** Rejecting `shared: ["packages/**"]` is
  correct, but the hint suggested `packages/http_client/**, packages/logging/**` — a fixed string
  with your prefix interpolated in. On a repo holding `packages/http-client` half of it was wrong,
  and the half that was right made the whole thing read as derived from the tree. Now a
  placeholder. ([#28](https://github.com/timimsms/trestle/pull/28))
- **The `UNBOUND` hint ignored the node it was about**, closing with "for a database or queue the
  answer is usually `@infra`" even on a node named `ext_stripe`. It now reads the ID's prefix.
  ([#29](https://github.com/timimsms/trestle/pull/29))

The through-line: **a hint that cannot know something should not sound like it does.** "Every
violation carries a runnable hint" is a golden-tested contract, and a confidently wrong hint costs
more than a vague one.

### Documentation

- [**A guided tour**](https://timimsms.github.io/trestle/TOUR) — a repository walked from nothing
  to a working check, then broken on purpose, with real output at every step. Including a section
  on what the check will *not* catch, demonstrated rather than asserted.
- A [documentation site](https://timimsms.github.io/trestle/), served from `docs/`.
- The build's planning tree is retired. What outlived it was promoted: `docs/DESIGN.md`,
  `docs/DECISIONS.md`, and the spike beside the script that runs it. 52 stale citations in Go
  comments now point at the new homes.

## v0.1.0 — 2026-08-24

First tagged release. Four commands, five violation codes.

### Commands

- **`check`** — validates bindings against the file tree. Exit 0 clean, 1 violations, 2 tool error.
  `--format=json` for machines, `--strict` to promote warnings.
- **`render`** — SVG per diagram through the embedded D2 library. No `d2` binary required.
  `--watch` rebuilds on save, debounced, and survives the syntax errors that exist between one
  keystroke and the next.
- **`explain`** — the node inventory: every node Trestle parsed, its bindings, and what each glob
  matches right now. `--overlaps` lists paths claimed by more than one node.
- **`init`** — scaffolds a config, the agent contract, and an empty starter diagram.

### What it does not do

Stated here because it is easier to adopt something whose limits are written down. Bindings are
node→path, so **edges are never verified** — an arrow that was always false, or one that became
false, passes. Architecture added *inside* an already-bound directory is invisible. A service
gutted to one remaining file keeps its binding.

`check` reports how much of the repo `discover:` actually watches, so a green run over 4% of a
codebase cannot be mistaken for a green run over all of it.

### Validated against

Three repositories it was not designed alongside — Go, Rails and a JS/TS monorepo — plus eleven
controlled agent trials. Every one produced a fix. Among them: a Go repo whose module sits one
directory down got zero `discover:` rules and a permanently green check over zero coverage; a node
bound to a directory holding only `.keep` reported `matches 1 file` and passed; and
`# @astrojs/language-server is a devDependency` failed as an unknown directive, which would have
fired in essentially every JS repo.
