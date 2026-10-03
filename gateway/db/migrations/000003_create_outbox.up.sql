CREATE TABLE session_event_sequences (
    session_ref uuid PRIMARY KEY,
    tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    last_sequence bigint NOT NULL CHECK (last_sequence > 0)
);

CREATE TABLE outbox (
    event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    session_ref uuid NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    event_type text NOT NULL CHECK (event_type IN (
        'session.started',
        'session.meter_values',
        'session.stopped',
        'command.result'
    )),
    payload jsonb NOT NULL,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    locked_until timestamptz,
    UNIQUE (session_ref, sequence),
    CHECK (published_at IS NULL OR locked_until IS NULL)
);

CREATE INDEX outbox_unpublished
    ON outbox (session_ref, sequence)
    WHERE published_at IS NULL;
