# Phase 2 — Parsers & Inputs

**Blocks on:** Phase 0
**Runs in parallel with:** Phase 1
**Blocks:** Phase 3
**Packages:** `internal/directive`, `internal/nodes`, `internal/config`, `internal/walk`

---

Phase 2 builds everything that turns a repo on disk into the four values the check engine takes.
Nothing here decides whether something is a violation — that is Phase 3's only job. If a parser
starts returning `Violation` values for anything other than `SYNTAX`, the seam has slipped.

The four packages are mutually independent and can be built in parallel by separate agents.

---

## 2.1 `internal/directive` — magic comment scanner

Line scan over the raw `.d2` bytes. D2's parser discards comments, so this is independent of the
D2 compiler by design — a malformed binding must never be able to break a render.

```d2
# @bind     <node_id> <glob>
# @external <node_id>
# @infra    <node_id>
# @ignore   <node_id> "<reason>"
```

**Rules**

- One directive per line. No continuations. Deliberately boring — `strings.Fields` and done.
- Position-independent: a directive need not sit near its node.
- `@bind` is repeatable per node; multiple globs are ORed.
- Globs are repo-root-relative; `**` crosses directory boundaries.
- Every directive carries its source `(file, line)`. Violations quote it; without it the hints
  are useless.

**`SYNTAX` cases** — each gets a table-driven test and each must appear in the `syntax/` fixture:

| Input | Why |
| --- | --- |
| `@ignore node` | reason string required — an unexplained suppression is how a check dies quietly |
| `@ignore node unquoted words` | reason must be quoted |
| `@bind node` | no glob |
| `@bind` | no node ID |
| `@external node extra` | unexpected trailing tokens |
| `@bnid node glob` | unknown directive — **do not** guess or fuzzy-match |

A line that is a plain comment (`# just a note`) is not a directive and is not an error. Only a
comment whose first token starts with `@` is a candidate.

**Do not** use a parser generator. **Do not** parse D2 syntax here — this package never looks at
anything but comment lines.

## 2.2 `internal/nodes` — D2 node ID extraction

The one place Trestle must actually understand D2. Gate B proved the approach:

```go
g, _, err := d2compiler.Compile(path, strings.NewReader(src), nil)
// recurse g.Root.ChildrenArray; collect Object.AbsID()
```

**Requirements**

- Return every node ID with container qualification intact (`platform.svc_work_orders`).
- Return the parent/child relation too — Phase 3 needs it for the O9 container rule. A flat
  `[]string` is insufficient; return a tree or a `map[id]parentID`.
- A D2 compile error is exit code **2** (tool error), not a violation. "Trestle is broken" and
  "your diagram is wrong" must stay distinguishable.
- **Version canary test:** compile `examples/repairs-platform/system.d2` and assert exactly 12
  IDs, including the six `platform.*` ones. When a d2 upgrade breaks the AST, this test is what
  tells you, rather than a fixture failing three phases downstream.

**Do not** hand-roll a D2 parser and do not fall back to regex. Gate B removed the excuse.

## 2.3 `internal/config` — `.trestle.yml`

Load, validate, apply defaults. Discovery walks **up** from CWD to find `.trestle.yml`; the
directory containing it is the root, and every path in the system is relative to it.

```yaml
version: 1
diagrams:  [docs/architecture/*.d2]
discover:  [app/services/*/, app/jobs/*/]
shared:    [lib/http_client/**, lib/logging/**]
exclude:   ["**/*_test.*", "**/vendor/**"]
severity:  {UNBOUND: warn}
render:    {out: docs/architecture/rendered/, layout: elk, theme: 0}
```

**Requirements**

- `goccy/go-yaml`, chosen for its error messages. Config errors are user-facing; surface the
  line number and the offending key, not a Go type-assertion panic.
- Validate: unknown top-level key → error. Unknown severity code → error naming the five valid
  codes. Severity value not in `{fail, warn, off}` → error. `version` other than `1` → error.
- **Reject blanket `shared:` entries.** L11 is the whole reason `shared` is safe: a bare `lib/**`
  would swallow a future `lib/dispatch_engine/` — real architectural weight, silently exempted.
  An entry whose glob is a bare directory wildcard immediately under root is an error with a
  hint to enumerate. This is config validation, not a violation code.
- Defaults: absent `severity` → `UNBOUND: warn`, everything else `fail`. Absent `exclude` →
  `.git`, `node_modules`, `vendor`. Absent `discover` is legal and means `UNMAPPED` never fires.
- Any config failure is exit code **2**.

## 2.4 `internal/walk` — the single filesystem walk

**All filesystem I/O in the product lives here.** This is the package that makes Phase 3's purity
achievable rather than aspirational.

- One `io/fs.WalkDir` from root. Produce a sorted `[]string` of repo-relative paths.
- Apply `exclude:` **during** the walk — pruning excluded directories rather than filtering after
  is the difference between hitting and missing the perf target on a repo with a large
  `node_modules`.
- Skip `.git` unconditionally.
- Return directories as well as files, flagged. `discover: app/services/*/` matches directories;
  `@bind app/services/billing/**` matches files. Conflating them breaks `UNMAPPED`.
- **Directories are flagged, not slash-suffixed** (`Entry{Path, IsDir}`), and `Path` carries no
  trailing `/`. Verified against `doublestar v4.10.0`: `app/services/*/` matches
  `app/services/billing/` and does **not** match `app/services/billing`. The shipped example
  config uses the trailing-slash form, so **the check engine must synthesize the trailing slash
  from `IsDir` before matching a `discover:` rule.** Get this wrong and every discover rule
  matches nothing, `UNMAPPED` never fires, and `trestle check` passes while inspecting zero code.
  Guarded by `internal/integration/TestDiscoverGlobNeedsTrailingSlash`.
- **Excluding a directory excludes its subtree.** This is not an optimization — it changes the
  answer. A bare pattern like `node_modules` matches exactly one path, so a filter-after-walk
  implementation would hide one entry and still walk the whole tree. Pruning is required
  behavior, and `config.DefaultExclude()` must agree with it; guarded by
  `internal/integration/TestDefaultExcludePrunes`.
- **An unreadable directory fails the walk (exit 2) rather than being skipped.** A silently
  shortened listing manufactures `ORPHAN` and `UNMAPPED` that are indistinguishable from real
  drift. If CI should tolerate it, that is a written-down config decision, not a default.
- **Symlinks are reported as files and never followed** — `fs.WalkDir` gives Lstat semantics,
  which keeps the walk cycle-free. Consequence: a repo doing `app/services/foo -> packages/foo`
  sees `app/services/foo` as a *file*, so `discover: app/services/*/` will not match it. This
  will read as a Trestle bug the first time someone hits it; CONVENTIONS.md should say so.
- No globbing here. The walk produces a listing; Phase 3 applies every glob to that one listing.
  **Not one walk per binding** — that is the design's stated performance strategy (DESIGN §7).

---

## Acceptance

- [ ] `directive` parses `examples/repairs-platform/system.d2` yielding **6 binds, 2 external,
      2 infra, 1 ignore**
- [ ] `nodes` yields **12 node IDs** from the same file with container paths intact, plus the
      parent relation
- [ ] Table-driven `SYNTAX` tests cover every malformed case in the table above, as **unit
      tests in `internal/directive`**. They do *not* all need to appear in the `syntax/` fixture —
      that fixture asserts exactly 2 SYNTAX per Phase 1, and Phase 1's fixture table is
      authoritative. (An earlier draft of this file said every case had to be in the fixture;
      that contradicted Phase 1 and Phase 1 wins.)
- [ ] `config` round-trips `examples/repairs-platform/.trestle.yml` and rejects: unknown key,
      unknown severity code, bad severity value, blanket `shared:` entry
- [ ] `walk` on a 100k-file synthetic tree completes well inside the 200ms total budget, with a
      committed benchmark
- [ ] Version canary test pins d2 v0.7.2 AST behavior
- [ ] No package in Phase 2 imports `internal/check`

## Do not

- Do not decide violations here. `SYNTAX` from a malformed directive is the sole exception, and
  even that is *reported* by `directive` and *classified* by `check`.
- Do not glob in `walk`.
- Do not read the filesystem in `directive`, `nodes`, or `config` beyond loading their own
  named input file.
