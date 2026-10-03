# Simulator

Virtual OCPP 1.6J charger. Run from the repository root:

```bash
docker compose up --build
```

It connects to `ws://gateway:9000/{CHARGER_ID}` with Basic auth, sends `BootNotification`, then heartbeats.
