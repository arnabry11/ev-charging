# Simulator

Virtual OCPP 1.6J chargers. Run from the repository root:

```bash
docker compose up --build
```

It runs `SIM_CHARGER_COUNT` chargers (default 1 in the binary, 10 in Compose).
The ids count up from `CHARGER_ID`, so `CHG-MUM-0001` with a count of 10 gives
`CHG-MUM-0001` to `CHG-MUM-0010`. Each charger connects to
`ws://gateway:9000/{id}` with Basic auth, sends `BootNotification`, reports
connector state, and emits heartbeats. Every id needs a matching `CHARGER_AUTH`
entry on the gateway.

The simulator accepts remote start and stop commands. A session goes
`Preparing → Charging → Finishing → Available`, reports cumulative
`Energy.Active.Import.Register` meter values, and draws constant power.

`SIM_POWER_W` is the power of the first charger. The others run at 50%, 150%,
300% and 75% of it, repeating, so their live values differ. Each one-second tick
advances a charger's clock by `SIM_SECONDS_PER_TICK` seconds (10 by default).
Change `SIM_POWER_W`, `SIM_TICK_INTERVAL_MS` and `SIM_SECONDS_PER_TICK` in `.env`
when you want to exercise a specific limit.

`POST /control/reconnect?charger_id=CHG-MUM-0003` drops that charger's socket and
boots it again. Without the parameter it acts on the first charger.
