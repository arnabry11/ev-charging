-- name: CreateSession :one
INSERT INTO sessions (
    session_ref,
    start_command_id,
    charger_id,
    connector_id,
    id_tag,
    state,
    limit_energy_wh,
    limit_duration_s
)
VALUES ($1, $2, $3, $4, $5, 'start_requested', $6, $7)
RETURNING *;

-- name: GetSession :one
SELECT *
FROM sessions
WHERE session_ref = $1;

-- name: GetSessionForStart :one
SELECT *
FROM sessions
WHERE charger_id = $1
  AND connector_id = $2
  AND id_tag = $3
  AND state IN ('start_requested', 'active')
ORDER BY created_at DESC
LIMIT 1;

-- name: ActivateSession :one
UPDATE sessions
SET state = 'active',
    ocpp_transaction_id = nextval('ocpp_transaction_ids'),
    meter_start_wh = $2,
    last_energy_wh = $2,
    started_at = $3,
    updated_at = now()
WHERE session_ref = $1
  AND state = 'start_requested'
RETURNING *;

-- name: MarkSessionStopping :one
UPDATE sessions
SET state = 'stopping',
    stop_source = $2,
    stop_command_id = $3,
    updated_at = now()
WHERE session_ref = $1
  AND state = 'active'
RETURNING *;

-- name: RestoreSessionActive :execrows
UPDATE sessions
SET state = 'active',
    stop_source = NULL,
    stop_command_id = NULL,
    updated_at = now()
WHERE session_ref = $1
  AND state = 'stopping';

-- name: FailSession :execrows
UPDATE sessions
SET state = 'failed',
    stop_reason = $2,
    updated_at = now()
WHERE session_ref = $1
  AND state = 'start_requested';

-- name: GetSessionByTransaction :one
SELECT *
FROM sessions
WHERE charger_id = $1
  AND ocpp_transaction_id = $2;

-- name: StopSession :one
UPDATE sessions
SET state = 'stopped',
    meter_stop_wh = $3,
    last_energy_wh = $3,
    stopped_at = $4,
    stop_reason = $5,
    updated_at = now()
WHERE charger_id = $1
  AND ocpp_transaction_id = $2
  AND state IN ('active', 'stopping')
RETURNING *;
