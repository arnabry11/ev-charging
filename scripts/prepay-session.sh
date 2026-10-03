#!/usr/bin/env bash
set -euo pipefail

idempotency_key="$(uuidgen | tr '[:upper:]' '[:lower:]')"
body="$(printf '{"idempotency_key":"%s","phone":"9876543210","ocpp_id":"CHG-MUM-0001","prepaid_paise":1432,"card":{"number":"4242424242424242","expiry_month":12,"expiry_year":2030,"cvv":"123"}}' "$idempotency_key")"

echo "Charging a card for a 240 Wh prepaid session"
created="$(curl --fail --silent --show-error -X POST "http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions" -H 'Content-Type: application/json' --data "$body")"
session_id="$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["id"])')"
echo "Session $session_id"

result=""
for _ in {1..40}; do
  result="$(curl --fail --silent --show-error "http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions/$session_id")"
  if printf '%s' "$result" | python3 -c 'import json,sys; raise SystemExit(0 if json.load(sys.stdin)["session"]["state"]=="stopped" else 1)'; then
    break
  fi
  sleep 1
done

python3 - "$result" <<'PY'
import json, sys
session = json.loads(sys.argv[1])["session"]
delivered = (session.get("meter_stop_wh") or 0) - (session.get("meter_start_wh") or 0)
if session["state"] != "stopped" or delivered != 240:
    raise SystemExit(f"prepaid session did not stop at 240 Wh: state={session['state']} delivered={delivered}")
print(f"Stopped after card prepay: delivered_wh={delivered} card_last4={session['card_last4']}")
PY
