# EV Charging Platform (POC)

A portfolio project for the hard core of an EV charging stack: a Go OCPP 1.6J gateway that talks to chargers, and a Rails + Sidekiq platform that owns prepaid sessions, refunds, and GST invoices. Everything runs on a laptop with a charger simulator and a mock UPI provider. No real hardware, no real payments, no real tax authority.

This repository is a **thin vertical slice**. The goal is to prove the risky parts (OCPP sessions, the signed Go-to-Rails contract, and idempotent money), then stop and decide whether to continue.

**You only need Docker.** Do not install Go, Ruby, or Postgres on the host.

## Run

```bash
cp .env.example .env
docker compose up
```

That builds the gateway and platform images, starts two Postgres databases and Redis, and waits until `/health` is up on both apps.

- Gateway: http://127.0.0.1:8080/health
- Platform: http://127.0.0.1:3000/health

Later, `docker compose --profile demo up` will run a full prepaid session on fake data.

## Ownership rule

**Go owns live device state. Rails owns everything involving money.**

The gateway never calculates prices. It only enforces numeric limits (`max_energy_wh`, `max_duration_s`) that the platform computed from the prepaid amount. Final price always comes from `meter_start_wh` and `meter_stop_wh` on `session.stopped`, never from summing live meter batches.

## Architecture

```
Driver / CLI ──► Platform (Rails + Sidekiq) ──► Mock UPI
                      │
                      │ signed HTTP commands (command_id)
                      ▼
                 Gateway (Go) ── OCPP 1.6J WS ──► Simulator
                      │
                      │ signed outbox events
                      ▼
                 Platform event receiver ──► SettleSessionJob
```

Two Postgres databases stay separate. The services talk only through a signed HTTP contract.

## What this POC will demonstrate

- OCPP 1.6J session flow: boot, heartbeat, start, meter values, stop, remote start/stop
- Limit enforcement on the gateway
- Outbox events with retries, per-session sequence numbers, and HMAC signatures
- Prepaid mock UPI, webhook idempotency, GST-snapshotted invoice, refund of unused prepaid
- One-command demo and a small set of E2E scenarios in CI

## What is not built (yet)

- Multi-tenancy UI and RBAC (columns exist; one tenant runs)
- Time-of-use, per-minute, and idle fees
- Virtual charger panel UI (CLI / scenarios first)
- Reconciliation job, `needs_review` queue, exotic offline-buffer faults
- Load, soak, and chaos tests
- Notifications, audit log viewer, PDF invoices, metrics dashboards
- Official OCPP JSON schema validation (basic validation first)
- CGST/SGST vs IGST variations (one GST split with the odd-paisa rule)
- Real payment providers, real GST e-invoicing, OCPP 2.0.1, OCPI roaming

Tax handling is a **simulation**, not tax advice. GST rate is configurable and defaults to 18%.

## Layout

| Path | Role |
|---|---|
| `gateway/` | Go OCPP gateway |
| `platform/` | Rails + Sidekiq business platform |
| `simulator/` | Virtual charger (later) |
| `mock-upi/` | Mock payment provider (later) |
| `contracts/` | Command and event schemas |
| `docs/` | ADRs and design notes |

## Money

All money columns are integer **paise**. Never floats. `prepaid = invoice_total + refund` must hold for every settled session.
