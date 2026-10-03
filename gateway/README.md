# Gateway

Go OCPP gateway. Run it from the repository root with Docker:

```bash
docker compose up --build
```

Health: http://127.0.0.1:8080/health

OCPP 1.6J WebSocket base URL: `ws://127.0.0.1:9000`. Chargers connect to `ws://127.0.0.1:9000/{charger_id}` with HTTP Basic auth matching `CHARGER_AUTH`.

