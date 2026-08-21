# Phase 1 — Fixture Repos

**Blocks on:** Phase 0
**Runs in parallel with:** Phase 2
**Blocks:** Phase 3
**Package:** `testdata/repos/`

---

## Why this is first

HANDOFF puts fixtures before the engine on purpose. Writing the expected outputs first forces the
engine's interface to be pinned down by its contract rather than by whatever its implementation
happens to make easy. If Phase 3 starts before this lands, the fixtures get written to match the
engine and prove nothing.

These are **real directory trees, not mocks.** The bug class Trestle exists to catch is a
filesystem bug. Test against a filesystem.

---

## The nine fixtures

Each is a directory under `testdata/repos/` containing at minimum a `.d2` file, a `.trestle.yml`,
some code files, and an `EXPECTED` file.

| Fixture | Contains | Expected result |
| --- | --- | --- |
| `clean/` | Every node bound, every discovered path owned | exit 0, no violations |
| `orphan/` | A `@bind` glob matching zero files | 1 ORPHAN, exit 1 |
| `orphan_shared/` | A `shared:` entry matching zero files | 1 ORPHAN, exit 1 (L11) |
| `unmapped/` | A `discover`-matched dir covered by no binding | 1 UNMAPPED, exit 1 |
| `dangling/` | A directive naming a node absent from the `.d2` | 1 DANGLING, exit 1 |
| `unbound/` | A node with no directive of any kind | 1 UNBOUND **warning**, exit 0 |
| `overlap/` | Two nodes binding the same path | exit 0, no violation (L12) |
| `syntax/` | `@ignore` with no reason; `@bind` with no glob | 2 SYNTAX, exit 1 |
| `nested/` | Container-qualified IDs, bound and checked | exit 0 |

### `nested/` carries extra weight

Gate B's O8 resolution lives or dies here. `nested/` must cover **three** cases, not one:

1. A directive using the fully-qualified ID (`@bind platform.svc_x app/x/**`) → matches.
2. A directive using the unqualified suffix (`@bind svc_x app/x/**`) → matches
   `platform.svc_x` by suffix rule.
3. A container (`platform`) with no directive of its own but all descendants accounted for →
   **no `UNBOUND`** (O9).

Add a tenth fixture, `ambiguous/`, if case 2 needs a negative twin: two containers each holding a
node with the same leaf ID, one unqualified directive → **1 SYNTAX**, exit 1, hint naming both
candidates. Prefer a tenth fixture over overloading `nested/`; a fixture that tests two things
tells you nothing when it fails.

### `overlap/` can only ever prove a negative

Worth understanding before it misleads someone: because L12 says overlapping bindings get no
violation code, `overlap/` passes trivially against an engine that never implements overlap
detection at all. It cannot fail for the right reason — only for the wrong one, if someone
invents a sixth code. Read it as a **regression guard against an `OVERLAP` code**, not as
coverage of the feature. The real test of L12 is `explain --overlaps` in Phase 5.

### `unbound/` is the one people get wrong

`UNBOUND` is a **warning** and the fixture exits **0**. O3 resolved this deliberately: in the
worked example `UNBOUND` fired on a queue node, which was a modeling gap, not an error. Failing
by default would have trained a suppression reflex on the first diagram anyone wrote. The fixture
must assert `exit 0 with 1 warning`, and a second assertion that `--strict` promotes it to exit 1.

---

## The `EXPECTED` file format

One per fixture. Machine-readable, because Phase 3 and Phase 4 both consume it and a prose file
gets read by neither.

```
exit: 1
strict_exit: 1
violations:
  ORPHAN  svc_billing  app/services/billing/**
warnings: 0
```

`strict_exit:` was added during implementation: the documented format had no way to record the
`--strict` assertion this phase requires. It is optional, and its absence is surfaced explicitly
rather than inferred — inferring it would mean the fixture parser deciding severity, which is the
engine's job.

Rules:

- `exit:` — 0, 1, or 2.
- `violations:` — one line per expected violation, `CODE  node_or_path  detail`. Order-independent;
  the comparison is set-based. An empty list is written as `violations: none`.
- `warnings:` — count of warning-severity violations. They appear in `violations:` too; this is a
  redundant cross-check and it is there on purpose.

Keep it boring to parse. `strings.Fields` should be enough.

---

## Fixture content guidance

- Keep trees **small** — five to fifteen files. A fixture you cannot read in one screen is a
  fixture nobody will maintain, and these will be edited every time the engine changes.
- Use real-looking paths (`app/services/billing/billing.rb`), not `foo/bar/baz`. When a golden
  file fails, realistic paths make the diff legible.
- Every fixture gets a one-line comment at the top of its `.d2` saying what it is testing.
- Include an `exclude:` entry exercised by at least one fixture — `exclude` and `shared` differ in
  three ways (DESIGN §4) and untested config semantics are how the distinction quietly collapses.
- `orphan_shared/` must contain a *valid* shared entry alongside the stale one. A fixture where
  everything is broken cannot catch an engine that flags everything.

---

## Acceptance

- [ ] All nine trees exist under `testdata/repos/` as real directories with real files
- [ ] Each has an `EXPECTED` file stating exit code and violation set in the format above
- [ ] `nested/` covers qualified, suffix-resolved, and container-with-accounted-descendants
- [ ] `unbound/` asserts both the default (exit 0, 1 warning) and `--strict` (exit 1) outcomes
- [ ] `orphan_shared/` contains at least one healthy `shared:` entry as a control
- [ ] At least one fixture exercises `exclude:` and proves it differs from `shared:`
- [ ] A Go helper — `testdata/expected.go` or similar — parses `EXPECTED` into a comparable
      struct. Phases 3 and 4 both import it; neither reimplements the parse.

## Do not

- Do not write the check engine here. If you find yourself deciding what the engine *should*
  output rather than what the fixture *contains*, stop — that decision belongs in DESIGN, and if
  DESIGN does not answer it, that is a finding to report, not to resolve in a fixture.
- Do not add a fixture for a sixth violation code. There are five.
