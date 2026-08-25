# Spike 01 — Is glob-level binding good enough?

**Resolves:** O1
**Timebox:** one sitting. Day 2, before any Go is written.
**Verdict required before:** any build work. This assumption is load-bearing; if it fails, the tool changes shape.

---

## The assumption under test

> Binding a diagram node to a **path glob** — rather than to a code symbol — is sufficient to catch the kinds of architectural drift that actually happen in this repo.

It is attractive because it is language-agnostic and nearly free. It is risky in two directions:

- **Too coarse (false negatives).** A service is gutted, rewritten, or absorbed into another, but one file remains under the glob. The check stays green while the diagram is now a lie. *This is the failure mode that kills the tool*, because it is silent.
- **Too noisy (false positives).** Routine refactors — moving a directory, splitting a module — fire `ORPHAN` constantly. The check gets bypassed within two weeks and the tool is dead by neglect rather than by argument.

## Method

Replay real history. Run `spike/glob-binding-probe.sh` against one or more repos you actually work in. It reconstructs, from git history, what a glob-based check *would have done* over the trailing window, without having to have run it at the time.

The probe answers four questions:

| Q | Measures | Bears on |
| --- | --- | --- |
| **Q1** | How many candidate units exist, and does each map to one clean glob? | Authoring cost |
| **Q2** | Unit directories fully deleted or renamed in-window | `ORPHAN` true positives |
| **Q3** | Unit directories that lost >70% of their files but not all | **False-negative risk** — the silent killer |
| **Q4** | Unit directories newly added in-window | `UNMAPPED` true positives |

```bash
./spike/glob-binding-probe.sh --repo ~/code/nomad --days 180 --unit-depth 2
```

## Falsification criteria

Decide these **before** looking at output. Written down here so the result cannot be rationalized after the fact.

| Result | Reading | Action |
| --- | --- | --- |
| Q2 + Q4 ≥ 5 combined events | The check would have fired on real drift several times | ✅ Proceed as specified |
| Q2 + Q4 = 1–4 events | Marginal. Real but thin. | ⚠️ Proceed, but treat the success criterion in OVERVIEW as genuinely at risk. Consider widening scope to a second repo before committing. |
| Q2 + Q4 = 0 events | Nothing to catch. Either the repo is too stable or the window is too short. | ❌ Re-run at 365 days. If still zero: **do not build.** The problem is not present here. |
| Q3 > Q2 | More silent gutting than clean deletion | ❌ Globs are too coarse. Scope change: bindings need a *content* signal, not just existence — e.g. minimum file count, or symbol-level binding. Return to design. |
| Q1 shows units needing >2 globs each on average | Authoring burden too high | ⚠️ Revisit `discover` granularity or unit depth before proceeding. |

The **Q3 > Q2 case is the one to watch for.** It is the result that invalidates the design rather than merely discouraging it, and it is the one most likely to be explained away in the moment. Trust the number.

## Probe validation (already done)

The probe was run against a synthetic repo containing one deleted service, one new
service, and one service gutted to 20% of its original files. All three detectors fired
correctly (Q2=1, Q3=1, Q4=1), so a zero result from a real repo means *no drift*, not a
broken probe.

Also run against a mature open-source library over a 900-day window: **zero events at
every depth.** That is a genuine finding and worth internalizing before you interpret
your own numbers — stable, well-factored codebases produce no signal here, and for those
repos this tool has nothing to offer. The interesting repos are the ones under active
structural change.

**`--unit-depth` counts path segments, and it dominates every number in the output.**
`app/services/billing` is depth 3, not depth 2. Sweep 1–4 and read the Q1 inventory to
find the depth where units correspond to boxes you'd actually draw. Getting this wrong
produces a false FAIL.

## Recording the outcome

Append the verdict to `OVERVIEW.md` as an amendment to O1 — the numbers, the date, the repo, and the decision taken. A spike whose result is not written down gets re-litigated in six weeks by someone who no longer remembers why.
