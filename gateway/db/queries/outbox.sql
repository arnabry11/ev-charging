-- name: AppendOutboxEvent :one
WITH seq AS (
    INSERT INTO session_event_sequences (session_ref, last_sequence)
    VALUES ($1, 1)
    ON CONFLICT (session_ref)
    DO UPDATE SET last_sequence = session_event_sequences.last_sequence + 1
    RETURNING last_sequence
)
INSERT INTO outbox (session_ref, sequence, event_type, payload, occurred_at)
SELECT $1, last_sequence, $2, $3, $4
FROM seq
RETURNING *;

-- name: ClaimOutboxBatch :many
WITH candidates AS (
    SELECT event_id, session_ref, sequence
    FROM outbox AS pending
    WHERE published_at IS NULL
      AND (locked_until IS NULL OR locked_until < now())
      AND NOT EXISTS (
          SELECT 1
          FROM outbox AS earlier
          WHERE earlier.session_ref = pending.session_ref
            AND earlier.sequence < pending.sequence
            AND earlier.published_at IS NULL
      )
    ORDER BY session_ref, sequence
    FOR UPDATE SKIP LOCKED
),
heads AS (
    SELECT DISTINCT ON (session_ref) event_id
    FROM candidates
    ORDER BY session_ref, sequence
    LIMIT $1
)
UPDATE outbox
SET locked_until = now() + interval '30 seconds',
    attempts = attempts + 1
WHERE event_id IN (SELECT event_id FROM heads)
RETURNING *;

-- name: MarkOutboxPublished :execrows
UPDATE outbox
SET published_at = now(),
    locked_until = NULL
WHERE event_id = $1
  AND published_at IS NULL;

-- name: ReleaseOutbox :exec
UPDATE outbox
SET locked_until = now() + interval '1 second'
WHERE event_id = $1
  AND published_at IS NULL;
