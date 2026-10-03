#!/usr/bin/env bash
# Starts one prepaid session on every simulated charger so the live board shows
# them running side by side. It returns once the sessions are accepted.
set -euo pipefail

count="${SIM_CHARGER_COUNT:-10}"
platform_url="http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}"
# Different prepaid amounts, in paise, so the sessions finish at different times.
amounts=(6000 8000 10000 12000 15000)

started=0
for ((i = 0; i < count; i++)); do
  ocpp_id="$(printf 'CHG-MUM-%04d' $((i + 1)))"
  prepaid_paise="${amounts[$((i % ${#amounts[@]}))]}"
  idempotency_key="$(uuidgen | tr '[:upper:]' '[:lower:]')"
  body="$(printf '{"idempotency_key":"%s","phone":"9876543210","ocpp_id":"%s","prepaid_paise":%d,"card":{"number":"4242424242424242","expiry_month":12,"expiry_year":2030,"cvv":"123"}}' \
    "$idempotency_key" "$ocpp_id" "$prepaid_paise")"
  response="$(curl --silent --show-error -X POST "$platform_url/internal/v1/prepaid-sessions" \
    -H 'Content-Type: application/json' --data-binary "$body")"
  state="$(printf '%s' "$response" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("session", {}).get("state", "error"))')"
  printf '%s  prepaid ₹%d.%02d  %s\n' "$ocpp_id" $((prepaid_paise / 100)) $((prepaid_paise % 100)) "$state"
  if [[ "$state" != "declined" && "$state" != "error" ]]; then
    started=$((started + 1))
  fi
done

echo "Started $started of $count sessions. Watch them at $platform_url/admin/live"
[[ "$started" -eq "$count" ]]
