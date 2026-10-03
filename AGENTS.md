# EV Charging agent rules

## Scope and workflow

- This is a Go + Rails portfolio POC. Build one feature at a time in small, focused PRs. Stop after presenting a completed feature's PR stack for review. Do not start the next stack until the current one is reviewed and merged.
- Use `main` as the default branch. Stacked PRs may target the preceding branch. Before opening a PR, inspect `git log <base>..HEAD` to confirm its scope.
- Write PR descriptions with **What**, **Why**, and **How**. Keep commits imperative and focused. Never add assistant attribution trailers.
- Use the `github.com-personal` SSH alias for Git operations and verify the remote belongs to `arnabry11`. Use the personal Git author identity configured in this repository (`Arnab` / `rarnab021@gmail.com`).
- Keep secrets, databases, caches, and local environment files out of Git.

## Docker first

- Docker Compose is the supported way to run, test, and demo. Do not add host-machine setup (Go, Ruby, Postgres, mise) as a requirement in the README.
- Generate and verify code inside containers when a toolchain is needed (`docker compose run`, `docker run --rm golang:1.25`, `docker run --rm ruby:3.4`).
- Every service that a reviewer should start must have a Dockerfile and a Compose service with a healthcheck.
- Keep `docker compose up` working at each merged stack.

## Ownership and money

- Go owns live device state (connections, connectors, sessions, meter readings, limit enforcement). Rails owns money (tariffs, payments, refunds, GST invoices).
- The gateway never calculates prices. It only enforces numeric limits the platform supplied.
- Final price is computed from `meter_start_wh` and `meter_stop_wh` on `session.stopped`, never by summing meter-value batches.
- All money columns are integer **paise** (`bigint`). Never floats or decimals for money.
- Keep `tenant_id` on tenant-owned tables. POC runs a single seeded tenant; do not build RBAC until asked.
- Idempotency is required on commands (`command_id`), events (`event_id`), payments, refunds, and invoices. Prefer unique indexes plus `RecordNotUnique` / conflict handling over "check then insert".

## Go patterns

- OCPP 1.6J via `lorenzodonini/ocpp-go`. Do not hand-roll framing.
- HTTP with `chi`. Structured logs with `log/slog`. Database later with `pgx` + `sqlc` and a migrate tool.
- Handle each charger sequentially (one goroutine owns that connection). Write outbox events in the same database transaction as the state change.
- Keep packages small. Comments only for non-obvious why, upstream constraints, or deliberate exceptions.

## Rails patterns

- Follow [`platform/AGENTS.md`](platform/AGENTS.md) once that file exists. Until then: thin controllers, services called with `.new(...).call`, thin Sidekiq jobs that call services.
- Generate schema migrations with `bundle exec rails g migration` inside the platform container. Never hand-create migration files.

## Verification

- Prefer Compose: `docker compose config`, healthchecks, and service tests run in their images.
- Gateway: `go test -race ./...` inside the gateway image. Lint with `golangci-lint` when the config exists.
- Platform: RSpec + RuboCop inside the platform image.
- CI must not require real chargers, real UPI, or network payment providers.
- Keep each commit bootable. Update `.env.example` when adding settings.
