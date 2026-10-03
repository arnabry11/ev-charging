#!/usr/bin/env bash
set -euo pipefail

session_ref="$(uuidgen | tr '[:upper:]' '[:lower:]')"
first_id="$(uuidgen | tr '[:upper:]' '[:lower:]')"
second_id="$(uuidgen | tr '[:upper:]' '[:lower:]')"
occurred_at="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
first="$(printf '{"event_id":"%s","event_type":"session.meter_values","session_ref":"%s","sequence":1,"occurred_at":"%s","payload":{"energy_wh":120}}' "$first_id" "$session_ref" "$occurred_at")"
second="$(printf '{"event_id":"%s","event_type":"session.meter_values","session_ref":"%s","sequence":2,"occurred_at":"%s","payload":{"energy_wh":240}}' "$second_id" "$session_ref" "$occurred_at")"
url="http://127.0.0.1:${PLATFORM_HTTP_PORT:-3000}/internal/v1/gateway-events"

post_event() {
  local body="$1"
  local timestamp signature
  timestamp="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
  signature="$(printf '%s' "$timestamp.$body" |
    openssl dgst -sha256 -hmac "${GATEWAY_SIGNING_SECRET:-dev-gateway-secret}" -hex |
    awk '{print $NF}')"
  curl --silent --show-error -w '\n%{http_code}' \
    -X POST "$url" \
    -H 'Content-Type: application/json' \
    -H "X-Timestamp: $timestamp" \
    -H "X-Signature: $signature" \
    --data-binary "$body"
}

echo "Posting gateway events out of order for $session_ref"
gap="$(post_event "$second")"
accepted="$(post_event "$first")"
duplicate="$(post_event "$first")"
followed="$(post_event "$second")"

python3 - "$gap" "$accepted" "$duplicate" "$followed" <<'PY'
import json, sys

def split(raw):
    body, _, code = raw.rpartition("\n")
    return json.loads(body), int(code)

checks = [
    ("gap", split(sys.argv[1]), 409, "sequence_gap"),
    ("accepted", split(sys.argv[2]), 200, "accepted"),
    ("duplicate", split(sys.argv[3]), 200, "duplicate"),
    ("followed", split(sys.argv[4]), 200, "accepted"),
]
for name, (body, code), want_code, marker in checks:
    if code != want_code or marker not in body.values():
        raise SystemExit(f"{name}: http={code} body={body}, want {want_code} {marker}")
print("Gateway events kept order: gap, accepted, duplicate, then the next sequence")
PY

stored="$(docker compose exec -T platform-db \
  psql -U "${POSTGRES_USER:-ev}" -d "${PLATFORM_DB:-ev_platform}" -At \
  -c "SELECT count(*) FROM processed_gateway_events WHERE session_ref = '$session_ref';")"
if [[ "$stored" != "2" ]]; then
  echo "stored $stored gateway events, want 2" >&2
  exit 1
fi
echo "Stored 2 gateway events after the duplicate replay"
