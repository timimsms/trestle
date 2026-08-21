# Phase 7 — `trestle init` (post-MVP)

**Blocks on:** Phase 4 stop gate; benefits from Phase 5
**Status:** not authorized. Re-scope after the MVP.

---

```
trestle init
```

Scaffold `.trestle.yml`, seed `discover:` from the detected repo layout, write `CONVENTIONS.md`
into the target repo, and add an `AGENTS.md` pointer.

## `CONVENTIONS.md` ships as part of the product

Not internal docs. It is the agent contract — the thing that makes L7 ("agents edit by node ID,
never by whole-file regeneration") enforceable by convention, since it is not enforceable by type
system. It lives at the repo root here and gets **embedded in the binary** via `go:embed` so
`init` can write it without a network fetch or a path assumption.

Keep the root copy canonical and embedded. Do not maintain a second copy — a duplicated contract
file is a drift surface, which is the thing this entire tool exists to close.

## Seeding `discover:`

**Gate A settled the depth: seed at depth 2.** Depth 2 on a real 4k-file repo produced 64 units
that correspond to boxes you would actually draw — `ui/src`, `server/src`, `packages/db`,
`packages/adapters`. Depth 3 produced 118 units with 35 holding ≤2 files; that is authoring
burden, not architecture.

Detect layout by looking for the conventional shapes — `app/services/*/`, `packages/*/`,
`src/*/`, `cmd/*/`, `internal/*/` — and propose what matched. **Propose, do not impose:** print
the seeded rules and what each currently matches, and let the user delete the wrong ones before
writing. A `discover:` rule the user did not agree to is a source of `UNMAPPED` noise, and a
noisy check gets `--no-verify`'d within a week.

O2 resolved toward configuration over heuristics for exactly this reason. `init` is allowed to
guess *once*, visibly, at setup time. `check` never guesses.

## Seeding `shared:`

This is the hard part and the reason `init` sits after `explain`. L11 requires enumeration, never
blanket — `lib/**` would swallow a future `lib/dispatch_engine/`, which is real architectural
weight silently exempted.

So `init` cannot write `shared:` correctly on its own. It should:

- Leave `shared:` empty with the L11 warning as a comment.
- Point at the test from CONVENTIONS.md: *would you mention it by name in an architecture review?*
  `lib/pricing_engine` encodes a domain boundary — that is a node. `lib/http_client` is plumbing —
  that is `shared:`.
- Let the first `trestle check` produce the `UNMAPPED` list, and let the user triage it into
  nodes and shared entries. That is a better first-run experience than a wrong guess, and it puts
  the enumeration decision where it belongs.

## O7 is still open

L11 assumes a repo has 5–20 shared subsystems. Gate A suggests ~13 for a 4k-file repo — inside
the range, but only just. Before `init` ships, probe a second repo and count. If a real repo needs
50+ entries, enumeration is unusable and L11 needs revisiting — though 50 shared subsystems is
itself a finding about the codebase, not just about Trestle.

## Acceptance (draft)

- [ ] `trestle init` in a fresh repo writes `.trestle.yml`, `CONVENTIONS.md`, and an `AGENTS.md`
      stanza (appending, never clobbering an existing `AGENTS.md`)
- [ ] `discover:` seeded at depth 2 from detected layout, shown to the user before writing
- [ ] `shared:` written empty with the L11 enumeration warning inline
- [ ] `CONVENTIONS.md` is `go:embed`-ed from the root copy — one source of truth
- [ ] Running `init` twice does not clobber a customized `.trestle.yml`; it diffs or refuses
- [ ] `trestle check` immediately after `init` runs and exits without a tool error
