# Dogfooding trial — pointing Trestle at a real repo

**Why this document exists.** OVERVIEW states one success criterion and makes it falsifiable:

> `trestle check` fails on a real PR, at least once in the first month, for a reason that was not
> anticipated when the bindings were written.

If it never fires, Trestle is decoration and should be deleted. That is not a figure of speech —
it is the MVP's evaluation gate. This document is how you run the trial that decides it.

**Status:** one catch so far, on Trestle itself. See the dogfood amendment in
[`planning/handoff/OVERVIEW.md`](planning/handoff/OVERVIEW.md). That is a sample size of one, on
the tool's own repo, so it does not settle the question.

---

## Pick the right repo

The Spike 01 probe already told us which repos have something to catch, and it is worth
re-reading before choosing:

- **A stable, well-factored codebase produces no signal.** The probe found zero drift events over
  a 900-day window on a mature library. For those repos Trestle has nothing to offer, and a null
  result there says nothing about the tool.
- **The interesting repos are the ones under active structural change** — services being split,
  directories moving, subsystems appearing.

Run the probe before committing to a target. It is read-only and touches nothing but git plumbing:

```console
make spike REPO=~/code/candidate DEPTH=2
```

Read `Q2 + Q4`. Zero means nothing happened in the window and the trial will be uninformative;
try `--days 365` or a different repo. What you want is a repo where directories genuinely moved.

---

## Set it up

```console
cd ~/code/target
trestle init          # proposes `discover:` rules; nothing is written until you agree
trestle check
```

Expect the first run to be noisy. That is the point of the first run: it is an inventory, not a
verdict.

`init` scaffolds the starter diagram **empty**, so that first `check` reports one `UNMAPPED` per
discovered directory and exits 1. Do not read that as a failed setup — it is the list of things
the repo has that nobody has drawn yet, and each line carries the `@bind` that fixes it. **Write
the count down before you fix anything**; it is the denominator for everything below.

The proposal `init` prints is the moment to narrow the rules. Deleting a rule at the prompt
because you can see it matched `src/.cache` is triage. Deleting it a week later because the check
is noisy is the failure mode this trial is measuring.

### Writing the config by hand

`init` is the fast path, not the only one. The shape it produces, minus the comments:

```yaml
version: 1

diagrams:
  - docs/architecture/*.d2

# Depth 2. The Spike 01 sweep found this is where a unit corresponds to a box
# you would actually draw; depth 3 fragmented a real repo into 118 units with
# 35 of them holding two files or fewer.
discover:
  - app/services/*/
  - packages/*/

# Enumerated, never blanket (L11). `lib/**` here would exempt every future
# subsystem that lands there, including ones that genuinely needed a box.
shared:
  - lib/http_client/**

exclude:
  - "**/*_test.*"
  - "**/vendor/**"
  - "**/node_modules/**"

severity:
  UNBOUND: warn
```

Then write one `.d2` describing the system, with a binding for every node. Copy the shape from
[`../examples/repairs-platform/system.d2`](../examples/repairs-platform/system.d2), and follow
[`../CONVENTIONS.md`](../CONVENTIONS.md) — node IDs must be real code identifiers, and a node
without a binding is incomplete work rather than a follow-up task.

**Do not** weaken the config to get to green. A `discover:` rule you deleted because it was noisy
is a rule that will never catch anything, and the whole trial is about whether the noise was
signal.

---

## Triage the first run

Sort every finding into one of three buckets. **Write the count down before you fix anything** —
the ratio is the result of the trial, and it is unrecoverable once you have edited the diagram.

| Bucket | Meaning | What it tells you |
| --- | --- | --- |
| **True positive** | The diagram and the code genuinely disagree | The tool works on this repo |
| **Modeling gap** | Neither wrong; the diagram never described this | Prompt-shaped. Usually `UNBOUND` or a missing node |
| **Noise** | The finding is wrong or unactionable | The thing that kills adoption |

A `--no-verify` reflex is earned in about a week of noise, so the noise count is the number that
decides whether this ships. If noise dominates, the fix is almost never "suppress the code" — it
is `discover:` pointed at the wrong granularity.

`--format=json` is easier to tally than reading the human output:

```console
trestle check --format=json | jq -r '.violations | group_by(.code)[] | "\(length)\t\(.[0].code)"'
```

---

## Then let it run on PRs

The criterion is about a **real PR**, not a first-run inventory. A first run finds accumulated
drift, which is interesting but expected. What is being tested is whether the check catches drift
*as it happens* — and that only shows up once it is wired in:

```yaml
- run: trestle check
```

Then leave it alone and see what it says. Add a note to the ledger the first time it fails for a
reason you did not predict — and, just as importantly, the first time it fails for a reason you
consider wrong.

---

## What to record

Append to OVERVIEW's ledger as a dogfood amendment. A result that is not written down gets
re-litigated in six weeks by someone who no longer remembers the numbers.

- **The repo, the date, and the config** you used.
- **The three bucket counts** from the first run.
- **Every finding you disagreed with**, and why. These are worth more than the true positives:
  a true positive confirms the design, a false positive changes it.
- **Whether it fired on a PR**, for what, and whether that reason was anticipated.
- **Any locked decision (L1–L12) the trial pushed back on.** Four spec gaps were already found
  this way during the build (O8–O11). A real repo will find more, and naming them is the point.

### Questions the build left open that only a real repo can answer

- **O7 — does enumerated `shared:` stay practical?** L11 assumes 5–20 shared subsystems. Nothing
  in the fixtures tests this; every fixture list has one or two entries. **Count `lib/*/` and
  equivalents in the target before you start.** If a real repo needs 50+, enumeration is unusable
  and L11 needs revisiting — though 50 shared subsystems is itself a finding about the codebase.
- **O11 — is the double-report noisy?** One rename can produce a `DANGLING` *and* an `UNMAPPED`.
  Both are true; whether it reads as thorough or as piling on is an empirical question. This was
  flagged during the build as the resolution most likely to be wrong, and it is cheap to reverse.
- **Is `UNBOUND`-as-warning right?** O3 chose warn to avoid training a suppression reflex. The
  one catch so far *was* an `UNBOUND`, which under default severity does not fail a build. If
  that keeps happening, O3 deserves another look.
- **Does the hint tell you what to type?** Every violation carries one and they are golden-tested,
  but they were written against fixtures. The first hint that sends someone the wrong way is a
  bug report worth filing.
