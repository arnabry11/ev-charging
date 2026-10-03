#!/usr/bin/env bash
set -euo pipefail

mode="${1:-energy}"
case "$mode" in
  energy)
    max_energy_wh=240
    max_duration_s=3600
    expected_source=energy_limit
    expected_delivered_wh=240
    ;;
  duration)
    max_energy_wh=10000
    max_duration_s=60
    expected_source=duration_limit
    expected_delivered_wh=120
    ;;
  *)
    echo "usage: $0 [energy|duration]" >&2
    exit 2
    ;;
esac

random_uuid() {
  local value
  value="$(openssl rand -hex 16)"
  printf '%s-%s-%s-%s-%s' \
    "${value:0:8}" "${value:8:4}" "${value:12:4}" "${value:16:4}" "${value:20:12}"
}

command_id="$(random_uuid)"
session_ref="$(random_uuid)"
id_tag="S-${session_ref:0:8}"
body="$(printf '{"command_id":"%s","session_ref":"%s","charger_id":"%s","connector_id":1,"id_tag":"%s","limits":{"max_energy_wh":%d,"max_duration_s":%d},"expires_at":"2099-12-31T23:59:59Z"}' \
  "$command_id" \
  "$session_ref" \
  "${CHARGER_ID:-CHG-MUM-0001}" \
  "$id_tag" \
  "$max_energy_wh" \
  "$max_duration_s")"
timestamp="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
signature="$(printf '%s' "$timestamp.$body" |
  openssl dgst -sha256 -hmac "${PLATFORM_SIGNING_SECRET:-dev-platform-secret}" -hex |
  awk '{print $NF}')"

echo "Starting $mode-limited session $session_ref"
curl --fail --silent --show-error \
  -X POST http://127.0.0.1:"${GATEWAY_HTTP_PORT:-8080}"/internal/v1/commands/start-session \
  -H 'Content-Type: application/json' \
  -H "X-Timestamp: $timestamp" \
  -H "X-Signature: $signature" \
  --data "$body"
echo

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
if [[ "$state" != "stopped" || "$stop_source" != "$expected_source" ]]; then
  echo "session did not stop at $expected_source: ${result:-no row}" >&2
  exit 1
fi
delivered_wh=$((meter_stop_wh - meter_start_wh))
if [[ "$delivered_wh" -ne "$expected_delivered_wh" ]]; then
  echo "unexpected delivered energy: got $delivered_wh Wh, want $expected_delivered_wh Wh" >&2
  exit 1
fi
echo "Stopped: source=$stop_source delivered_wh=$delivered_wh meter_start_wh=$meter_start_wh meter_stop_wh=$meter_stop_wh"

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
