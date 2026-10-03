# Contracts

JSON schemas for the signed HTTP contract between the Go gateway and the Rails platform live here.

Planned contents (not in this PR):

- Platform → gateway commands (`start-session`, `stop-session`, charger upsert)
- Gateway → platform event envelopes (`session.started`, `session.meter_values`, `session.stopped`, `command.result`)
- Shared examples used by producer and consumer tests

Both sides must validate against the same files. Delivery is at-least-once; every consumer is idempotent on `command_id` / `event_id`.
