# Gateway

Go OCPP gateway. Run it from the repository root with Docker:

```bash
docker compose up --build
```

Health: http://127.0.0.1:8080/health

OCPP 1.6J WebSocket base URL: `ws://127.0.0.1:9000`. Chargers connect to `ws://127.0.0.1:9000/{charger_id}` with HTTP Basic auth matching `CHARGER_AUTH`.

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

