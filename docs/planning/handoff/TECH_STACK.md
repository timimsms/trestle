# Trestle — Tech Stack

## Language: Go

Not the obvious pick given your recent work, so the argument has to carry it:

1. **D2 is written in Go and exposes a library API.** Embedding it means `trestle render` needs no `d2` binary on PATH, and version skew between the renderer and the parser becomes impossible. Shelling out to `d2` from any other language reintroduces an install prerequisite — a real adoption tax for a tool whose whole pitch is "add it to CI."
2. **Single static binary.** `go install`, or drop it in a container. No runtime, no version manager, no lockfile. For a CI-resident tool this matters more than it does for an app.
3. **Fast startup.** The tool runs on every commit and inside agent loops. Interpreter boot time is a real cost at that call frequency.

**Escape hatch:** if Go friction is high enough to slow the spike, TypeScript + shelling out to the `d2` CLI gets to the same MVP, and the parser is the easy part in any language. The cost is the install prerequisite and a slower `check`. Take the hatch if it is the difference between building and not building — a shipped TS version beats an unstarted Go one. Record it as a ledger amendment if taken.

## Dependencies

| Concern | Choice | Note |
| --- | --- | --- |
| Diagram render | `oss.terrastruct.com/d2` | Library, not subprocess. Pin the version — it moves. |
| CLI | `spf13/cobra` | Four commands; `flag` would also do. Cobra earns it on help output and shell completion. |
| Config | `goccy/go-yaml` | Better errors than `gopkg.in/yaml.v3`; config errors are user-facing. |
| Globs | `bmatcuk/doublestar` | stdlib `filepath.Match` has no `**`. Non-negotiable. |
| File walk | stdlib `io/fs.WalkDir` | Single walk, all globs applied to the result. See DESIGN §7. |
| Watch | `fsnotify/fsnotify` | Only if `render --watch` is built. |
| Directive parsing | stdlib | Line scan + `strings.Fields`. No parser generator. The format is deliberately trivial. |

Explicitly avoided: any D2 grammar fork, any AST manipulation of D2, any embedded database.

## Project layout

```
trestle/
├── cmd/trestle/main.go
├── internal/
│   ├── directive/     # magic comment scanner
│   ├── nodes/         # D2 node ID extraction
│   ├── check/         # violation engine — the core, heavily tested
│   ├── config/
│   └── render/        # D2 library wrapper
├── testdata/
│   └── repos/         # fixture repos: clean, orphan, unmapped, dangling
└── docs/
```

`internal/check` is the product. It should be testable with zero I/O — take a file listing and a directive set, return violations. Fixture repos in `testdata/` exercise the integration seam.

## Node ID extraction

The one place Trestle must actually understand D2. Two options:

- **Preferred:** use the D2 library's parser to walk the AST and collect node IDs, including container paths. Correct by construction.
- **Fallback:** regex the ID position from declaration lines. Fragile on nested containers and connection-only node declarations.

Take the preferred path. If the D2 library's AST surface turns out to be unstable across versions, pin hard and revisit — but do not hand-roll a D2 parser. That is a tar pit and it is not the problem being solved.

## Testing

- Table-driven unit tests on `internal/check`. Every violation code gets a positive and a negative case.
- Fixture repos under `testdata/repos/` — real directory trees, not mocks. The bug class this tool exists to catch is a filesystem bug; test against a filesystem.
- Golden-file tests on `check --format=human` output. The hints are part of the contract.

## Distribution

`go install` for v1. Homebrew tap and a GitHub Action wrapper when there is a second user. Not before.
