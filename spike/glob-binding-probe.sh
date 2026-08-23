#!/usr/bin/env bash
#
# glob-binding-probe.sh — Spike 01
#
# Determines what a glob-based architecture check WOULD have flagged over a
# trailing window, by comparing unit inventories at two points in history.
# Read-only: touches nothing but git plumbing.
#
# Usage:
#   ./glob-binding-probe.sh --repo ~/code/myrepo [--days 180] [--unit-depth 2]
#
#   --unit-depth N   a "unit" is a directory N path segments deep.
#                    depth 1 = app/, depth 2 = app/services/, etc.
#                    Sweep 1, 2 and 3 — pick the one matching how you'd
#                    actually draw boxes on the diagram.
#
# Exit: 0 always. This is a measurement, not a check.

set -uo pipefail

REPO="."
DAYS=180
UNIT_DEPTH=2
GUT_THRESHOLD=70   # % of a unit's original files gone => "gutted but alive"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)       REPO="$2"; shift 2 ;;
    --days)       DAYS="$2"; shift 2 ;;
    --unit-depth) UNIT_DEPTH="$2"; shift 2 ;;
    --threshold)  GUT_THRESHOLD="$2"; shift 2 ;;
    -h|--help)    sed -n '2,19p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

cd "$REPO" 2>/dev/null || { echo "cannot cd to $REPO" >&2; exit 2; }
git rev-parse --git-dir >/dev/null 2>&1 || { echo "not a git repo: $REPO" >&2; exit 2; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

hr()  { printf '%s\n' "------------------------------------------------------------"; }
say() { printf '%s\n' "$*"; }

# Directories that are never a box on an architecture diagram.
#
# Dot-directories are tooling and process by convention — .github, .changes,
# .claude, .next. They churn hard by design, and that made them the single
# largest source of false Q3 events: across the repos probed so far, every Q3
# hit that produced a FAIL verdict was a non-architectural directory, and one
# was a changelog-fragment tree that turns over every release. A check that
# never watched `.changes/v1.14` cannot have a false negative there.
#
# This changes recorded numbers: Gate A's repo counted `.agents`, `.claude` and
# `.github` among its new units. Re-run and confirm the verdict still holds
# rather than assuming it does.
EXCLUDE_RE='(^|/)(node_modules|vendor|dist|build|tmp|coverage|target|fixtures?|\.[^/]+)(/|$)'

# path -> unit directory at UNIT_DEPTH segments. Files shallower than the
# depth have no unit and are dropped.
unitize() {
  awk -F/ -v d="$UNIT_DEPTH" '
    NF > d { p=$1; for (i=2; i<=d; i++) p = p "/" $i; print p }
  '
}

# --- resolve the window boundary commit --------------------------------------
BASE="$(git rev-list -1 --before="${DAYS} days ago" HEAD 2>/dev/null)"
if [[ -z "$BASE" ]]; then
  BASE="$(git rev-list --max-parents=0 HEAD | tail -1)"
  TRUNCATED="  (window predates repo; using first commit)"
else
  TRUNCATED=""
fi
BASE_DATE="$(git log -1 --format=%as "$BASE")"
HEAD_DATE="$(git log -1 --format=%as HEAD)"

# --- file inventories at both ends -------------------------------------------
git ls-tree -r --name-only "$BASE" | grep -Ev "$EXCLUDE_RE" | sort > "$TMP/files_base"
git ls-files                       | grep -Ev "$EXCLUDE_RE" | sort > "$TMP/files_now"

unitize < "$TMP/files_base" | sort | uniq -c | awk '{print $2" "$1}' | sort > "$TMP/units_base"
unitize < "$TMP/files_now"  | sort | uniq -c | awk '{print $2" "$1}' | sort > "$TMP/units_now"

cut -d' ' -f1 "$TMP/units_base" > "$TMP/ub"
cut -d' ' -f1 "$TMP/units_now"  > "$TMP/un"

say ""
say "Trestle · Spike 01 — glob-binding probe"
say "repo:       $(pwd)"
say "window:     ${BASE_DATE} -> ${HEAD_DATE}  (${DAYS}d)${TRUNCATED}"
say "unit depth: ${UNIT_DEPTH}"
hr

# --- Q1: inventory ------------------------------------------------------------
UNIT_COUNT=$(wc -l < "$TMP/units_now" | tr -d ' ')
BASE_COUNT=$(wc -l < "$TMP/units_base" | tr -d ' ')
SINGLETONS=$(awk '$2 <= 2' "$TMP/units_now" | wc -l | tr -d ' ')

say ""
say "Q1 · INVENTORY — units at depth ${UNIT_DEPTH}"
say "    units today: ${UNIT_COUNT}   (at window start: ${BASE_COUNT})"
say "    units with <=2 files: ${SINGLETONS}"
say ""
say "    largest:"
sort -k2 -rn "$TMP/units_now" | head -10 | awk '{printf "      %-50s %s files\n", $1, $2}'

# --- Q2: ORPHAN true positives ------------------------------------------------
comm -23 "$TMP/ub" "$TMP/un" > "$TMP/q2"
Q2=$(wc -l < "$TMP/q2" | tr -d ' ')

say ""
hr
say "Q2 · ORPHAN true positives — units present at window start, gone today"
say "    count: ${Q2}"
if [[ "$Q2" -gt 0 ]]; then
  say ""
  say "    (a glob bound to these WOULD have fired ORPHAN)"
  head -20 "$TMP/q2" | sed 's/^/      /'
fi

# --- Q3: false-negative risk --------------------------------------------------
: > "$TMP/q3"
while read -r unit; do
  base_n=$(awk -v u="$unit" '$1==u{print $2}' "$TMP/units_base")
  [[ -z "$base_n" || "$base_n" -eq 0 ]] && continue
  grep -E "^${unit}/" "$TMP/files_base" | sort > "$TMP/_b"
  grep -E "^${unit}/" "$TMP/files_now"  | sort > "$TMP/_n"
  lost=$(comm -23 "$TMP/_b" "$TMP/_n" | wc -l | tr -d ' ')
  pct=$(( lost * 100 / base_n ))
  now_n=$(awk -v u="$unit" '$1==u{print $2}' "$TMP/units_now"); now_n=${now_n:-0}

  # High turnover alone is not Q3. A unit that replaced its original files and
  # ended up LARGER has not been gutted, rewritten-down, or absorbed — it grew,
  # which is Q4's story rather than a false-negative risk.
  #
  # Without this, a directory that went from 1 file to 9 scores "100% replaced"
  # and trips the Q3 > Q2 FAIL verdict on its own. That happened on a real repo
  # and would have read as "do not build" to anyone running the probe cold.
  if [[ "$pct" -ge "$GUT_THRESHOLD" && "$now_n" -le "$base_n" ]]; then
    printf '%s %s %s %s\n' "$unit" "$pct" "$lost" "$now_n" >> "$TMP/q3"
  fi
done < <(comm -12 "$TMP/ub" "$TMP/un")

Q3=$(wc -l < "$TMP/q3" 2>/dev/null | tr -d ' '); Q3=${Q3:-0}

say ""
hr
say "Q3 · FALSE-NEGATIVE RISK — units that lost >=${GUT_THRESHOLD}% of their"
say "     original files but still exist"
say "    count: ${Q3}"
if [[ "$Q3" -gt 0 ]]; then
  say ""
  say "    (a glob bound to these stays GREEN — this is silent staleness)"
  sort -k2 -rn "$TMP/q3" | head -20 \
    | awk '{printf "      %-42s %3s%% replaced  (%s lost, %s files now)\n", $1, $2, $3, $4}'
  say ""
  say "    Read this list before trusting the verdict below. Q3 only counts against"
  say "    the design for units you would actually bind to a diagram node. A gutted"
  say "    docs/ or scripts/ directory is real churn and nobody would have drawn a"
  say "    box for it, so it cannot be a false negative in a check that never"
  say "    watched it. Discount those and re-read the numbers."
fi

# --- Q4: UNMAPPED true positives ----------------------------------------------
comm -13 "$TMP/ub" "$TMP/un" > "$TMP/q4"
Q4=$(wc -l < "$TMP/q4" | tr -d ' ')

say ""
hr
say "Q4 · UNMAPPED true positives — units that did not exist at window start"
say "    count: ${Q4}"
if [[ "$Q4" -gt 0 ]]; then
  say ""
  say "    (each needed a new binding; unbound ones fire UNMAPPED)"
  head -20 "$TMP/q4" | sed 's/^/      /'
fi

# --- verdict ------------------------------------------------------------------
SIGNAL=$(( Q2 + Q4 ))

say ""
hr
say "VERDICT"
say ""
printf '    Q1 units today:  %s\n' "$UNIT_COUNT"
printf '    Q2 orphans:      %s\n' "$Q2"
printf '    Q3 silent risk:  %s\n' "$Q3"
printf '    Q4 new units:    %s\n' "$Q4"
printf '    signal (Q2+Q4):  %s\n' "$SIGNAL"
say ""

if [[ "$Q3" -gt "$Q2" ]]; then
  say "    >> FAIL — Q3 > Q2. Silent staleness outweighs detectable drift."
  say "       Globs are too coarse for this repo. Do NOT build as specified."
  say "       Bindings need a content signal, not just existence. Redesign."
elif [[ "$SIGNAL" -eq 0 ]]; then
  say "    >> FAIL — no drift events in window."
  say "       Re-run with --days 365, and sweep other --unit-depth values."
  say "       If still zero, the problem is not present here. Do not build."
elif [[ "$SIGNAL" -lt 5 ]]; then
  say "    >> MARGINAL — real but thin signal."
  say "       Proceed with the success criterion genuinely at risk."
  say "       Probe a second repo before committing to the build."
else
  say "    >> PROCEED — the check would have fired ${SIGNAL} times on real drift."
fi

say ""
say "    Sweep --unit-depth 1/2/3 before deciding; depth drives every number here."
say "    Record the outcome as an amendment to O1 in OVERVIEW.md."
say ""
