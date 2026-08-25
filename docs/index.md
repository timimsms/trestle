# Trestle

**Keeps architecture diagrams honest by binding diagram nodes to real paths in the repo, and
failing CI when they diverge.**

Architecture diagrams drift. Not because people are careless, but because nothing connects the
diagram to the thing it describes. A service gets renamed, a module gets deleted, a new subsystem
appears — and the diagram stays exactly as correct-looking as it was the day it was drawn.

Diagram-as-code fixes *reviewability*. It does not fix *truth*. A D2 file can be perfectly
version-controlled and completely wrong.

```console
go install github.com/timimsms/trestle/cmd/trestle@v0.1.0
```

## Start here

| | |
| --- | --- |
| [**Design**](DESIGN.md) | Binding syntax, the five violation codes, config, the CLI surface |
| [**Decisions**](DECISIONS.md) | Why it is shaped this way — the locked ledger, the open questions, and the resolutions taken while building |
| [**Dogfooding**](DOGFOODING.md) | How to run a trial that can actually falsify the thing |

Source, issues and the README: [github.com/timimsms/trestle](https://github.com/timimsms/trestle).

## What a green check does not mean

Worth knowing before adopting it. Bindings are node→path, so:

- **Edges are never verified.** An arrow that was always false, or one that became false, passes.
- **Architecture added inside an already-bound directory is invisible.**
- **A service gutted to one remaining file keeps its binding.**

The check is a floor, not a proof. It catches a service moving or disappearing — the most common
way a diagram goes stale — and tells you exactly what to type when it fires.
