# Gateway

Go OCPP gateway. Run it from the repository root with Docker:

```bash
docker compose up --build
```

Health: http://localhost:8080/health

OCPP 1.6J WebSocket base URL: `ws://localhost:9000`. Chargers connect to `ws://localhost:9000/{charger_id}` with HTTP Basic auth matching `CHARGER_AUTH`.

## Internal commands

- `POST /internal/v1/commands/start-session`
- `POST /internal/v1/commands/stop-session`

Commands require `X-Timestamp` (RFC3339) and
`X-Signature = hex(HMAC_SHA256(PLATFORM_SIGNING_SECRET, timestamp + "." + raw_body))`.
Each `command_id` is persisted and returns the original result when retried.

During an active transaction, the gateway stores cumulative Wh readings and
requests `RemoteStopTransaction` when either the command's energy or duration
limit is reached. Limits are numeric inputs from the platform; the gateway does
not calculate prices.

Session transitions and command completions append `session.started`,
`session.meter_values`, `session.stopped`, and `command.result` rows to the
outbox in the same database transaction. Each session has a gap-free sequence.
When `PLATFORM_EVENTS_URL` is set, a publisher delivers those rows in sequence
with `X-Signature = hex(HMAC_SHA256(GATEWAY_SIGNING_SECRET, timestamp + "." + body))`
and retries after a non-2xx response.

