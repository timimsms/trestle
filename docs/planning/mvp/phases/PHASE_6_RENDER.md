# Phase 6 — `trestle render` (post-MVP)

**Blocks on:** Phase 4 stop gate
**Status:** not authorized. Re-scope after the MVP.

---

```
trestle render [--watch]
```

Render `diagrams:` to `render.out` using the **embedded D2 library, not a subprocess**. This is
the entire argument for choosing Go (L8, TECH_STACK §1): no `d2` binary on PATH, no version skew
between the renderer and the parser Trestle already uses for node extraction.

Gate B already established the library works and pinned it at `v0.7.2`. `internal/nodes` compiles
diagrams; `internal/render` renders them. Both go through the same pinned dependency — if they
diverge, that is the exact failure L8 was chosen to prevent.

## Scope

- `render.out`, `render.layout` (elk default), `render.theme` from `.trestle.yml`.
- `--watch` via `fsnotify`. Debounce — editors write files two or three times per save, and an
  undebounced watcher renders three times and looks broken.
- Output SVG. Rendered artifacts are generated: CONVENTIONS.md already says never edit an SVG,
  edit the `.d2`. Make sure `trestle init` gitignores the output directory, or accept the churn
  deliberately.

## The one thing to be careful about

D2 rendering is slow relative to everything else Trestle does. **Do not let it into the `check`
path.** `check` must stay a lint-speed operation under 200ms; if rendering ever becomes a
prerequisite for checking, the performance target is gone and the check moves to a nightly job,
which is where checks go to stop blocking the PR that broke them.

`render` and `check` share the config loader and the D2 compile, nothing else.

## Not in scope

**The preview pane is not this phase and is not authorized.** O5 is unresolved. Per OVERVIEW and
the standing constraints, the pane earns its place only if it overlays check status onto the
rendered diagram — red outline on failing nodes, dashed on unbound, click to `explain`. A preview
that shows what `d2 --watch` already shows is not worth a line of code. If the overlay looks
fiddly when scoped, cut the pane entirely and let `d2 --watch` own rendering.

## Acceptance (draft)

- [ ] `trestle render` on `examples/repairs-platform/` produces an SVG with elk layout, no `d2`
      binary installed
- [ ] `--watch` rebuilds on save, debounced, and does not exit on a transient parse error —
      report it and keep watching
- [ ] A D2 compile failure is exit 2, consistent with `check`
- [ ] `render` and `nodes` demonstrably use the same pinned d2 version
