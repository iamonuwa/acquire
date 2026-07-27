#!/usr/bin/env bash
#
# Architectural test for adding a source.
#
# The table-driven claim is that adding a jurisdiction costs a config row plus
# human encoding work, not code. This script checks that claim mechanically,
# because an agent asked to self-assess whether it stayed inside the boundary
# will say yes.
#
# Adding a source may touch:
#   config/sources.poll.yaml   the poll row
#   internal/fetch/            at most ONE new strategy file
#   internal/normalize/        at most ONE new normalizer file
#   internal/gate/             jurisdiction PHI patterns
#   testdata/                  fixtures
#
# Anything else is a design failure and should be reported, not worked around.
#
# Usage: architectural_test.sh [base-ref]     (default: origin/main)

set -euo pipefail

BASE="${1:-origin/main}"

if ! git rev-parse --verify "$BASE" >/dev/null 2>&1; then
  echo "FAIL: base ref '$BASE' does not exist" >&2
  exit 2
fi

changed="$(git diff --name-only "$BASE"...HEAD)"

if [ -z "$changed" ]; then
  echo "No changes against $BASE. Nothing to test."
  exit 0
fi

violations=()
fetch_new=0
normalize_new=0

# Files that existed at BASE are modifications; files that did not are additions.
existed_at_base () {
  git cat-file -e "$BASE:$1" 2>/dev/null
}

while IFS= read -r f; do
  [ -z "$f" ] && continue
  case "$f" in
    config/sources.poll.yaml)
      ;;
    testdata/*)
      ;;
    internal/gate/*)
      ;;
    internal/fetch/*)
      if ! existed_at_base "$f"; then
        fetch_new=$((fetch_new + 1))
      fi
      ;;
    internal/normalize/*)
      if ! existed_at_base "$f"; then
        normalize_new=$((normalize_new + 1))
      fi
      ;;
    *_test.go)
      ;;
    *)
      violations+=("$f")
      ;;
  esac
done <<< "$changed"

status=0

echo "Changed against $BASE:"
echo "$changed" | sed 's/^/  /'
echo

if [ "$fetch_new" -gt 1 ]; then
  echo "FAIL: $fetch_new new files in internal/fetch/. A source needs at most one new strategy."
  echo "      More than one means per-source code is leaking into the strategy enum."
  status=1
fi

if [ "$normalize_new" -gt 1 ]; then
  echo "FAIL: $normalize_new new files in internal/normalize/. A source needs at most one new normalizer."
  status=1
fi

if [ "${#violations[@]}" -gt 0 ]; then
  echo "FAIL: changes outside the allowed surface:"
  printf '      %s\n' "${violations[@]}"
  echo
  echo "      Adding a source should cost a config row, at most one fetch strategy,"
  echo "      at most one normalizer, and fixtures. If the above were genuinely"
  echo "      required, the table-driven design has failed. Report that finding"
  echo "      rather than making the change fit."
  status=1
fi

if [ "$status" -eq 0 ]; then
  echo "PASS: config row + $fetch_new fetch strategy + $normalize_new normalizer + fixtures."
  echo "      Table-driven design holds."
fi

exit "$status"