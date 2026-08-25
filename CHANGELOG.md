# Changelog

Notable changes per release. Anything that changes what `trestle check` reports on an unchanged
repo is called out as **behaviour change**, because that is the line between a release you can take
without reading and one you cannot.

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
