#!/usr/bin/env bash
set -euo pipefail

idempotency_key="$(uuidgen | tr '[:upper:]' '[:lower:]')"
body="$(printf '{"idempotency_key":"%s","phone":"9876543210","ocpp_id":"CHG-MUM-0001","prepaid_paise":1432,"card":{"number":"4242424242420002","expiry_month":12,"expiry_year":2030,"cvv":"123"}}' "$idempotency_key")"
url="http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/prepaid-sessions"

echo "Declining a card that ends in 0002"
created="$(curl --silent --show-error --fail-with-body -w '\n%{http_code}' -X POST "$url" -H 'Content-Type: application/json' --data-binary "$body")"
replay="$(curl --silent --show-error --fail-with-body -w '\n%{http_code}' -X POST "$url" -H 'Content-Type: application/json' --data-binary "$body")"

python3 - "$created" "$replay" <<'PY'
import json, sys

def split(raw):
    body, _, code = raw.rpartition("\n")
    return json.loads(body), int(code)

created, created_code = split(sys.argv[1])
replay, replay_code = split(sys.argv[2])
session = created["session"]
if created_code != 201 or session["state"] != "declined" or session["limit_energy_wh"] is not None:
    raise SystemExit(f"declined card was not stored: http={created_code} body={created}")
if replay_code != 200 or replay["session"]["id"] != session["id"] or replay["session"]["state"] != "declined":
    raise SystemExit(f"declined replay did not return the same session: http={replay_code} body={replay}")
if "4242424242420002" in json.dumps(created) or "4242424242420002" in json.dumps(replay):
    raise SystemExit("response included the full card number")
print(f"Declined card stored once: session={session['id']} card_last4={session['card_last4']}")
PY
