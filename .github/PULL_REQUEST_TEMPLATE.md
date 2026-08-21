<!--
Keep this short. The diff is already in the diff; what is not recoverable
later is why you chose this over the obvious alternative.
-->

## What and why

<!-- One paragraph. The reasoning, not the changelog. -->

## Constraints checked

- [ ] Still five violation codes
- [ ] Still four top-level commands
- [ ] `internal/check` still does no I/O
- [ ] Still one filesystem walk; all globs apply to that listing
- [ ] Every new violation carries a runnable hint
- [ ] Tests ship in this change, not as a follow-up
- [ ] `make` and `make self-check` pass

<!-- If you unchecked one, say why here. An amended ledger entry is a fine
     answer; a silent exception is not. -->

## Did anything push back?

<!-- Did a locked decision (L1-L12) or a resolution (O8-O11) turn out to be
     wrong, awkward, or more expensive than it looked?

     This section is the most valuable one in the template. Four spec gaps
     were found during the build precisely because someone hit a wall and
     said so instead of coding around it. "Nothing" is a legitimate answer —
     but if you worked around something, name it. -->

## New dependency?

<!-- If yes: what it does, and why stdlib was insufficient. Delete if not. -->
