# Simulator

Virtual OCPP 1.6J charger. Run from the repository root:

```bash
docker compose up --build
```

It connects to `ws://gateway:9000/{CHARGER_ID}` with Basic auth, sends
`BootNotification`, reports connector state, and emits heartbeats.

The simulator accepts remote start/stop commands. A session follows
`Preparing → Charging → Finishing → Available`, reports cumulative
`Energy.Active.Import.Register` meter values, and uses a constant-power model.
The defaults model 7.2 kW and advance the charger clock by 60 seconds on every
one-second tick. Override `SIM_POWER_W`, `SIM_TICK_INTERVAL_MS`, and
`SIM_SECONDS_PER_TICK` in `.env` when exercising specific limits.
