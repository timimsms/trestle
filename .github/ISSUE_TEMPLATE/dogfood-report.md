---
name: Dogfood report
about: You pointed Trestle at a real repo. Report what happened, especially the bad parts
title: "[dogfood] "
labels: dogfood
---

<!--
See docs/DOGFOODING.md for the protocol. The short version: triage the first
run into three buckets and write the counts down BEFORE fixing anything, since
the ratio is the result and it is unrecoverable once you edit the diagram.

A report where nothing fired is still worth filing. OVERVIEW says that if the
check never fires, Trestle is decoration and should be deleted — a null result
is evidence about the tool, not a failed errand.
-->

**Repo shape.** <!-- Language, rough file count, and how much structural churn
it has seen. A stable, well-factored codebase produces no signal, and that is a
property of the repo rather than of the tool. -->

**Spike probe result**, if you ran `make spike REPO=... DEPTH=2`:

```
Q2:        Q3:        Q4:        signal:
```

## First run triage

| Bucket | Count |
| --- | --- |
| True positive — diagram and code genuinely disagree | |
| Modeling gap — neither wrong, diagram never described it | |
| **Noise — wrong or unactionable** | |

**The noise, in detail.** <!-- This is the section that matters. Noise is what
earns a --no-verify reflex in about a week, and it is the number that decides
whether this ships. Paste the findings you disagreed with. -->

**Did you have to weaken the config to get to green?** <!-- Which rule, and
what it stopped catching. A `discover:` rule deleted for being noisy is one
that will never catch anything again. -->

## On a PR

**Did it fire on a real PR?** <!-- What for. Was the reason anticipated when
the bindings were written? That question is the MVP's evaluation gate. -->

## Open questions this bears on

<!-- Delete any you cannot speak to. -->

- **O7 — does enumerated `shared:` scale?** How many entries did you need?
  L11 assumes 5–20; every fixture has one or two, so this is untested.
- **O11 — is the double-report noisy?** Did a rename give you a `DANGLING` and
  an `UNMAPPED` for one change, and did that read as thorough or as piling on?
- **Is `UNBOUND`-as-warning right?** Did anything important arrive as a warning
  and get ignored?
- **Did a hint send you the wrong way?** They are golden-tested but were written
  against fixtures. The first misleading one is a real bug.
