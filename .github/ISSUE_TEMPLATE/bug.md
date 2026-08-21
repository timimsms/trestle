---
name: Bug
about: Trestle did the wrong thing
title: ""
labels: bug
---

**What you ran.**

```console
$ trestle check
```

**What it printed.** <!-- Paste the output. `--format=json` is often clearer
about what the engine actually decided. -->

**What you expected instead**, and why.

**`.trestle.yml`** <!-- Trimmed to the relevant rules. -->

```yaml
```

**The relevant directives** from the `.d2`, and the paths they were meant to
match:

```d2
```

**Version.** <!-- `trestle --version`, or the commit SHA. -->

---

<!--
Two things worth checking first, because they are behaviors rather than bugs
and both look like bugs:

- A `discover:` rule that matches a directory containing no files always fires
  UNMAPPED. An empty unit cannot be covered, and that is usually a real finding.
- A directive that is SYNTAX or DANGLING participates in nothing else — it does
  not bind and confers no coverage (O11). So one bad line can produce a second,
  apparently unrelated violation elsewhere.

If you think either of those is the wrong call, that is a ledger issue rather
than a bug, and it is a more interesting one.
-->
