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

## When the platform refuses an event

If the platform is down or unreachable, or returns a 5xx, a timeout or a 401, the
gateway keeps the event and retries it with a delay that doubles from 1 second up
to a minute. It never gives up on those, because they say nothing about the event
itself (a wrong signing secret, for example, would otherwise throw the whole
backlog away).

If the platform looks at an event and refuses it (HTTP 400, 409, 413 or 422), the
gateway counts a rejection. After `OUTBOX_MAX_REJECTIONS` of them (10 by default,
about five minutes of backoff) it gives up: the event gets a `dead_at` timestamp
and the reason in `last_error`, and one `gave up on gateway event` line is
logged at ERROR. Events queued behind it in the same session are marked dead too,
because the platform would only refuse them as out of order. Other sessions keep
publishing.

Nothing is deleted. To see what was given up on:

```sql
SELECT session_ref, sequence, event_type, rejections, last_error, dead_at
FROM outbox WHERE dead_at IS NOT NULL ORDER BY session_ref, sequence;
```

Once the cause is fixed, put everything back in the queue and the gateway will
deliver it in order:

```sql
UPDATE outbox
SET dead_at = NULL, rejections = 0, last_error = NULL, locked_until = NULL
WHERE dead_at IS NOT NULL;
```

Run these against the gateway database, for example with
`docker compose exec gateway-db psql -U ev -d ev_gateway`.
