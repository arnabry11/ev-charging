#!/usr/bin/env bash
set -euo pipefail

idempotency_key="$(uuidgen | tr '[:upper:]' '[:lower:]')"
body="$(printf '{"idempotency_key":"%s","phone":"9876543210","ocpp_id":"CHG-MUM-0001","prepaid_paise":1432,"card":{"number":"4242424242424242","expiry_month":12,"expiry_year":2030,"cvv":"123"}}' "$idempotency_key")"

echo "Charging a card for a 240 Wh prepaid session"
created="$(curl --fail --silent --show-error -X POST "http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions" -H 'Content-Type: application/json' --data-binary "$body")"
replay="$(curl --fail --silent --show-error -X POST "http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions" -H 'Content-Type: application/json' --data-binary "$body")"
session_id="$(python3 - "$created" "$replay" <<'PY'
import json, sys
first = json.loads(sys.argv[1])["session"]["id"]
second = json.loads(sys.argv[2])["session"]["id"]
if first != second:
    raise SystemExit(f"replay created a second session: {second}")
print(first)
PY
)"
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

invoice=""
for _ in {1..20}; do
  if invoice="$(curl --fail --silent --show-error "http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions/$session_id/invoice")"; then
    break
  fi
  invoice=""
  sleep 1
done

python3 - "$invoice" <<'PY'
import sys
html = sys.argv[1]
def amount(name):
    marker = f'data-amount="{name}" data-paise="'
    start = html.index(marker) + len(marker)
    return int(html[start:html.index('"', start)])

total = amount("total")
refund = amount("refund")
if total + refund != 1432 or total != 1432 or refund != 0:
    raise SystemExit(f"invoice total={total} refund={refund}, want 1432 and 0")
print(f"Invoice settled: total_paise={total} refund_paise={refund}")
PY
