# ADR 001: Ownership split between gateway and platform

## Status

Accepted for the POC.

## Context

An EV charging system has two different shapes of work. Chargers hold long-lived WebSocket connections and emit a stream of OCPP messages. Billing needs prepaid payments, GST invoices, refunds, and exactly-once money outcomes.

Putting both in one process mixes connection lifetime with money correctness. Splitting without a clear owner for each concern leads to double pricing, lost events, or the gateway inventing tariffs.

## Decision

- The **Go gateway** owns live device state: charger connections, connector status, session meter readings, and numeric limit enforcement.
- The **Rails platform** owns money: tariffs, prepaid payments, refunds, GST, invoices, and the session business state machine.
- The two databases are separate. Services communicate only through a signed HTTP contract (commands in, events out).
- The gateway never calculates a price. The platform never speaks OCPP.

## Consequences

- Limit calculation happens on the platform from the prepaid amount; the gateway only stops the charger when those numbers are hit.
- Final settlement uses `meter_start_wh` / `meter_stop_wh` from `session.stopped`.
- An outbox on the gateway is required so a crash cannot drop events after a state change.
- The platform must treat events as at-least-once and dedupe on `event_id`.
