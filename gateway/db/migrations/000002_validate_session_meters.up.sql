ALTER TABLE sessions
    ADD CONSTRAINT sessions_stop_meter_not_before_start
    CHECK (state <> 'stopped' OR meter_stop_wh >= COALESCE(last_energy_wh, meter_start_wh));
