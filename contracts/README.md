# Contracts

JSON schemas for the signed HTTP contract between the Go gateway and the Rails platform live here.

Current contracts:

- Platform → gateway commands: `commands/start-session.schema.json` and `commands/stop-session.schema.json`
- Gateway → platform events: `events/gateway-event.schema.json`

Planned contents:

- Charger upsert command
- Shared examples used by producer and consumer tests

Both sides must validate against the same files. Delivery is at-least-once; every consumer is idempotent on `command_id` / `event_id`.
