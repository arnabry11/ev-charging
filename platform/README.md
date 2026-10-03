# Platform

Rails + Sidekiq business app. Run it from the repository root with Docker:

```bash
docker compose up --build
```

See the root README. Do not install Ruby on the host.

Internal registry:

- `GET` and `POST /internal/v1/chargers`
- `GET` and `POST /internal/v1/drivers`

Charger ids are stored uppercase. Driver phones are stored as 10-digit Indian mobile numbers. Both are unique per tenant. The POC tenant id matches the gateway default, `00000000-0000-0000-0000-000000000001`.

`GET` and `PUT /internal/v1/tariff` keep one active flat tariff. Prices are integer paise: the seeded tariff is 1800 paise per kWh plus a 1000 paise session fee. Development Compose loads `db/seeds.rb` after preparing the database.
