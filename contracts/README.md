# Contracts

JSON schemas for the signed HTTP contract between the Go gateway and the Rails platform live here.

Current contracts:

- Platform → gateway commands: `commands/start-session.schema.json` and `commands/stop-session.schema.json`

Planned contents:

- Charger upsert command
- Gateway → platform event envelopes (`session.started`, `session.meter_values`, `session.stopped`, `command.result`)
- Shared examples used by producer and consumer tests

Both sides must validate against the same files. Delivery is at-least-once; every consumer is idempotent on `command_id` / `event_id`.
