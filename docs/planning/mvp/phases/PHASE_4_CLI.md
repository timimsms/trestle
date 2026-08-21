# Phase 4 — `trestle check` CLI

**Blocks on:** Phase 3
**Blocks:** the MVP stop gate
**Packages:** `cmd/trestle`, `internal/report`

---

The last MVP phase. Wires the pure engine to a real repo and a real exit code.

```
trestle check [--format=human|json] [--strict]
```

`cmd/trestle` stays thin: find root, load config, walk, parse, compile, call `check.Check`,
format, exit. No logic. If a decision is being made in `cmd/`, it belongs in `internal/`.

---

## Command wiring

Cobra, per TECH_STACK — it earns its place on help output and shell completion, and three more
commands are coming.

**Order of operations**, and each failure mode's exit code:

1. Walk up from CWD for `.trestle.yml`. Not found → **exit 2**, hint `trestle init`.
2. Load + validate config → **exit 2** on failure, with file, line, and offending key.
3. Expand `diagrams:` globs. Zero diagrams matched → **exit 2**; a check with nothing to check
   is a broken setup, not a clean repo. This one is easy to get wrong and silently reports success.
4. `internal/walk` once from root.
5. Per diagram: `directive.Parse` + `nodes.Extract`. D2 compile failure → **exit 2**.
6. `check.Check` once over the union.
7. Format, print, exit 0/1.

`--strict` promotes every warning to a failure at the **exit-code stage only**. The violation's
own severity is unchanged in `--format=json`; add a top-level `"strict": true` instead. A flag
that rewrites the data makes JSON consumers unable to tell a warning from a failure.

---

## Human output

DESIGN §5 is the spec. Reproduce it exactly — this is a golden file, not a suggestion:

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

- Group by diagram file, header is the repo-relative path.
- Code column padded to 10; detail and hint aligned under the node.
- **Every violation carries a hint.** No exceptions. A violation whose hint is empty is a
  golden-test failure, and that is the point.
- Summary line always prints, including on success: `0 failures, 0 warnings`. Silence on
  success reads as "didn't run."
- Warnings print with the failures, marked, and do not affect exit unless `--strict`.
- Colorize by severity when stdout is a TTY; **plain when it is not.** CI logs full of ANSI
  escapes are how people stop reading CI logs. Golden tests run with color off.

## JSON output

```json
{
  "version": 1,
  "strict": false,
  "summary": {"failures": 2, "warnings": 0},
  "violations": [
    {
      "code": "ORPHAN",
      "severity": "fail",
      "node": "platform.svc_billing",
      "path": null,
      "source": {"file": "docs/architecture/system.d2", "line": 4},
      "detail": "@bind app/services/billing/** matches 0 files",
      "hint": "renamed? `git log --diff-filter=D -- app/services/billing`"
    }
  ]
}
```

- Node IDs are **fully qualified** in JSON even when the directive wrote a suffix. The human
  format may echo what the author typed; the machine format should be unambiguous.
- `"version": 1` from day one. This output will be consumed by agents and by CI, and adding a
  version later means guessing.
- Stable ordering — sort by file, then line, then code. Unstable JSON ordering makes diffing two
  runs useless.

---

## Acceptance

- [ ] `trestle check` run inside each Phase 1 fixture directory matches that fixture's `EXPECTED`
      — both the violation set and the exit code
- [ ] `--format=json` round-trips to the same violation set as `--format=human` for every fixture
- [ ] Golden files committed for human output on all fixtures; **the hints are part of the
      golden file**
- [ ] `--strict` flips `unbound/` from exit 0 to exit 1 without changing the JSON severity field
- [ ] Exit 2 verified distinctly for: missing config, malformed config, unparseable D2, zero
      diagrams matched
- [ ] Color suppressed when stdout is not a TTY, asserted by test
- [ ] `trestle check` on `examples/repairs-platform/` behaves sensibly — the example is a live
      test input, not decoration. Note that its `discover:` paths do not exist in this repo; either
      give the example a small real tree or document the expected result
- [ ] End-to-end wall time on a 100k-file repo stays within the 200ms budget

---

## → STOP HERE. Report before continuing.

Phases 1–4 are the MVP. Do not start Phase 5, 6, or 7 without re-scoping them against what the
MVP taught. Report three things, and the third matters most:

1. **What passed.** Fixtures, benchmarks, golden files.
2. **What surprised you.** Gate B produced two spec gaps in thirty minutes; Phases 1–4 will
   produce more.
3. **Which locked decisions the implementation pushed back on.** L1–L12 were made without code.
   Some are wrong. Naming which ones, and why, is worth more than quietly working around them.

Then **dogfood before building more.** The OVERVIEW success criterion — `trestle check` fails on
a real PR for an unanticipated reason — is the MVP's evaluation gate, and Gate A's caveat means
it is genuinely at risk. Put `trestle` on a real repo and find out. Building `explain`, `render`,
and `init` on top of a check that never fires is three phases of work on a tool that OVERVIEW
says should be deleted.
