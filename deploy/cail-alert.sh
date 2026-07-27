#!/usr/bin/env bash
# On failure: grab the last journal lines and POST them if ALERT_WEBHOOK is set.
set -euo pipefail

UNIT="cail-acquire.service"
SUBJECT="cail-acquire failed on $(hostname)"
BODY="$(journalctl -u "$UNIT" -n 50 --no-pager 2>&1 || echo '(journal unavailable)')"

if [[ -n "${ALERT_WEBHOOK:-}" ]]; then
  curl -fsS -m 15 -X POST -H 'Content-Type: text/plain' \
    --data "${SUBJECT}"$'\n\n'"${BODY}" "$ALERT_WEBHOOK" || true
fi

printf '%s\n\n%s\n' "$SUBJECT" "$BODY"
