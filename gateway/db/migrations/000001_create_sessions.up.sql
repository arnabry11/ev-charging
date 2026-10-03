CREATE TABLE command_inbox (
    command_id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    command_type text NOT NULL,
    request jsonb NOT NULL,
    state text NOT NULL DEFAULT 'received' CHECK (state IN ('received', 'dispatching', 'completed')),
    result jsonb,
    http_status integer,
    locked_until timestamptz,
    claim_token uuid,
    dispatched_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CHECK (
        (result IS NULL AND http_status IS NULL AND completed_at IS NULL)
        OR
        (result IS NOT NULL AND http_status IS NOT NULL AND completed_at IS NOT NULL)
    ),
    CHECK (result IS NULL OR locked_until IS NULL),
    CHECK (state <> 'completed' OR result IS NOT NULL),
    CHECK (state <> 'dispatching' OR (dispatched_at IS NOT NULL AND claim_token IS NOT NULL))
);

CREATE TABLE sessions (
    session_ref uuid PRIMARY KEY,
    tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    start_command_id uuid NOT NULL UNIQUE REFERENCES command_inbox(command_id),
    stop_command_id uuid UNIQUE REFERENCES command_inbox(command_id),
    charger_id text NOT NULL,
    connector_id integer NOT NULL CHECK (connector_id > 0),
    id_tag text NOT NULL,
    state text NOT NULL CHECK (state IN (
        'start_requested',
        'active',
        'stopping',
        'stopped',
        'failed'
    )),
    ocpp_transaction_id integer UNIQUE,
    limit_energy_wh bigint NOT NULL CHECK (limit_energy_wh > 0),
    limit_duration_s integer NOT NULL CHECK (limit_duration_s > 0),
    meter_start_wh bigint,
    meter_stop_wh bigint,
    last_energy_wh bigint,
    started_at timestamptz,
    stopped_at timestamptz,
    stop_reason text,
    stop_source text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        state IN ('start_requested', 'failed')
        OR
        (ocpp_transaction_id IS NOT NULL AND meter_start_wh IS NOT NULL AND started_at IS NOT NULL)
    ),
    CHECK (
        state <> 'stopped'
        OR
        (meter_stop_wh IS NOT NULL AND stopped_at IS NOT NULL)
    ),
    CHECK (stop_command_id IS NULL OR state IN ('stopping', 'stopped'))
);

CREATE UNIQUE INDEX sessions_one_live_connector
    ON sessions (charger_id, connector_id)
    WHERE state IN ('start_requested', 'active', 'stopping');

CREATE UNIQUE INDEX sessions_pending_id_tag
    ON sessions (charger_id, id_tag)
    WHERE state = 'start_requested';

CREATE SEQUENCE ocpp_transaction_ids AS integer START WITH 1;
