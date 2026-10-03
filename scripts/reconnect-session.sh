#!/usr/bin/env bash
set -euo pipefail

max_energy_wh=400
random_uuid() {
  local value
  value="$(openssl rand -hex 16)"
  printf '%s-%s-%s-%s-%s' \
    "${value:0:8}" "${value:8:4}" "${value:12:4}" "${value:16:4}" "${value:20:12}"
}

command_id="$(random_uuid)"
session_ref="$(random_uuid)"
id_tag="S-${session_ref:0:8}"
body="$(printf '{"command_id":"%s","session_ref":"%s","charger_id":"%s","connector_id":1,"id_tag":"%s","limits":{"max_energy_wh":%d,"max_duration_s":3600},"expires_at":"2099-12-31T23:59:59Z"}' \
  "$command_id" "$session_ref" "${CHARGER_ID:-CHG-MUM-0001}" "$id_tag" "$max_energy_wh")"
timestamp="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
signature="$(printf '%s' "$timestamp.$body" |
  openssl dgst -sha256 -hmac "${PLATFORM_SIGNING_SECRET:-dev-platform-secret}" -hex |
  awk '{print $NF}')"
charger_url="http://127.0.0.1:${GATEWAY_HTTP_PORT:-8080}/internal/v1/chargers/${CHARGER_ID:-CHG-MUM-0001}"

boot_at() {
  curl --fail --silent --show-error "$charger_url" |
    python3 -c 'import json,sys; body=json.load(sys.stdin); print(body["connection_state"] + " " + body.get("last_boot_at",""))'
}

echo "Starting energy-limited session $session_ref before a reconnect"
curl --fail --silent --show-error \
  -X POST "http://127.0.0.1:${GATEWAY_HTTP_PORT:-8080}/internal/v1/commands/start-session" \
  -H 'Content-Type: application/json' \
  -H "X-Timestamp: $timestamp" \
  -H "X-Signature: $signature" \
  --data-binary "$body"
echo

reading=""
for _ in {1..30}; do
  reading="$(docker compose exec -T gateway-db \
    psql -U "${POSTGRES_USER:-ev}" -d "${GATEWAY_DB:-ev_gateway}" -AtF '|' \
    -c "SELECT state, meter_start_wh, last_energy_wh FROM sessions WHERE session_ref = '$session_ref';")"
  IFS='|' read -r state meter_start_wh last_energy_wh <<<"$reading"
  if [[ "$state" == "active" && -n "${last_energy_wh:-}" && "$last_energy_wh" -gt "$meter_start_wh" && $((last_energy_wh - meter_start_wh)) -lt "$max_energy_wh" ]]; then
    break
  fi
  sleep 1
done
if [[ "$state" != "active" ]]; then
  echo "session was not charging before reconnect: ${reading:-no row}" >&2
  exit 1
fi

before="$(boot_at)"
echo "Dropping the charger mid-session ($before, delivered_wh=$((last_energy_wh - meter_start_wh)))"
curl --fail --silent --show-error -X POST "http://127.0.0.1:${SIMULATOR_HTTP_PORT:-8081}/control/reconnect"
echo

after=""
for _ in {1..20}; do
  after="$(boot_at)"
  if [[ "${after%% *}" == "connected" && "$after" != "$before" ]]; then
    break
  fi
  sleep 1
done
if [[ "${after%% *}" != "connected" || "$after" == "$before" ]]; then
  echo "charger did not boot again after reconnect: before=$before after=$after" >&2
  exit 1
fi
echo "Charger booted again: $after"

result=""
for _ in {1..30}; do
  result="$(docker compose exec -T gateway-db \
    psql -U "${POSTGRES_USER:-ev}" -d "${GATEWAY_DB:-ev_gateway}" -AtF '|' \
    -c "SELECT state, meter_start_wh, meter_stop_wh, stop_source FROM sessions WHERE session_ref = '$session_ref';")"
  if [[ "$result" == stopped\|* ]]; then
    break
  fi
  sleep 1
done
IFS='|' read -r state meter_start_wh meter_stop_wh stop_source <<<"$result"
delivered_wh=$((meter_stop_wh - meter_start_wh))
if [[ "$state" != "stopped" || "$stop_source" != "energy_limit" || "$delivered_wh" -ne "$max_energy_wh" ]]; then
  echo "session did not finish at the energy limit after reconnect: ${result:-no row}" >&2
  exit 1
fi
echo "Stopped after reconnect: delivered_wh=$delivered_wh"

published=0
for _ in {1..30}; do
  published="$(docker compose exec -T gateway-db \
    psql -U "${POSTGRES_USER:-ev}" -d "${GATEWAY_DB:-ev_gateway}" -At \
    -c "SELECT count(*) FROM outbox WHERE session_ref = '$session_ref' AND event_type = 'session.stopped' AND published_at IS NOT NULL;")"
  if [[ "$published" == "1" ]]; then
    break
  fi
  sleep 1
done
received="$(docker compose exec -T platform-db \
  psql -U "${POSTGRES_USER:-ev}" -d "${PLATFORM_DB:-ev_platform}" -At \
  -c "SELECT count(*) FROM processed_gateway_events WHERE session_ref = '$session_ref' AND event_type = 'session.stopped';")"
if [[ "$published" != "1" || "$received" != "1" ]]; then
  echo "session.stopped was not delivered: published=${published:-0} received=${received:-0}" >&2
  exit 1
fi
echo "Delivered session.stopped to the platform"
