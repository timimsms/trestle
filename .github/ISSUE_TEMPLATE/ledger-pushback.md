---
name: A locked decision looks wrong
about: You hit a wall caused by L1–L12 or O8–O11 and worked around it, or could not
title: "[ledger] "
labels: ledger
---

<!--
This is the most useful issue you can open here.

L1–L12 were made before any code existed, so some of them are wrong. Four gaps
were found during the build (O8, O9, O10, O11) exactly this way — someone hit a
wall and reported it instead of quietly coding around it. Working around a bad
decision destroys the evidence that it was bad.

You do not need to be sure. "This cost me an hour and I think the decision is
why" is a complete report.
-->

**Which decision.** <!-- e.g. L11, O11, "five violation codes" -->

**What you were trying to do.**

**What it cost.** <!-- What you had to write, delete, or give up. If you worked
around it, show the workaround — that is the evidence. -->

**What you would do instead**, if you have a view. <!-- Optional. Naming the
problem is worth more than proposing the fix. -->

**Does it change the shape of the tool, or just this case?** <!-- Optional. A
decision that is wrong once is a papercut; one that is wrong structurally is a
scope change. -->
