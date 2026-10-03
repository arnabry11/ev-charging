# Platform agent rules

Follow these Rails conventions (adapted from Smart Hub). This is a Docker-first app: generate gems, run specs, and run RuboCop inside the platform image.

## Controllers

Keep controllers thin. Actions wire params and HTTP only. Domain work lives in services and model scopes.

## Services

- Call stateful services with `.new(...).call`. Do not define `def self.call`.
- Simple create/update later can subclass a `PersistModelService`. Multi-step flows can use `ServiceSteps`.
- Return `ServiceResponse.success` / `ServiceResponse.error`.
- Read instance variables through `private attr_reader`, not `@ivar`.
- Admin-only services will live under `app/services/admin/<resource>/`.

## Jobs

Thin Sidekiq jobs: `include Sidekiq::Job`, load a record, early return, call a service.

```ruby
class SettleSessionJob
  include Sidekiq::Job
  sidekiq_options queue: "default", retry: 5

  def perform(session_id)
    Sessions::SettleService.new(session_id:).call
  end
end
```

## Money and idempotency

- Money is integer **paise** (`bigint`). Never decimals or floats.
- Idempotency uses unique indexes plus rescue `ActiveRecord::RecordNotUnique`.
- Keep `tenant_id` on tenant-owned tables. POC runs one tenant; always scope queries by tenant.

## Tests

- Request specs for HTTP, service specs for domain, job specs for jobs.
- Prefer `let_it_be` for database records once test-prof is added.
- Run RSpec and RuboCop in Docker before a PR is done:

```bash
docker compose run --rm platform bundle exec rspec
docker compose run --rm -e SKIP_DB_PREPARE=1 platform bundle exec rubocop
```

## Migrations

Generate with `docker compose run --rm platform bundle exec rails g migration Name`. Never hand-create migration files.
