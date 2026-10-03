-- name: InsertCommand :execrows
INSERT INTO command_inbox (command_id, command_type, request)
VALUES ($1, $2, $3)
ON CONFLICT (command_id) DO NOTHING;

-- name: GetCommand :one
SELECT *
FROM command_inbox
WHERE command_id = $1;

-- name: ClaimCommand :one
UPDATE command_inbox
SET locked_until = now() + interval '15 seconds',
    claim_token = $2
WHERE command_id = $1
  AND state = 'received'
  AND result IS NULL
  AND (locked_until IS NULL OR locked_until < now())
RETURNING *;

-- name: MarkCommandDispatching :one
UPDATE command_inbox
SET state = 'dispatching',
    dispatched_at = now(),
    locked_until = NULL
WHERE command_id = $1
  AND state = 'received'
  AND claim_token = $2
RETURNING *;

-- name: CompleteCommand :execrows
UPDATE command_inbox
SET state = 'completed',
    result = $2,
    http_status = $3,
    locked_until = NULL,
    claim_token = NULL,
    completed_at = now()
WHERE command_id = $1
  AND claim_token = $4
  AND state IN ('received', 'dispatching');
