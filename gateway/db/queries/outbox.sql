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
      AND dead_at IS NULL
      AND (locked_until IS NULL OR locked_until < now())
      AND NOT EXISTS (
          SELECT 1
          FROM outbox AS earlier
          WHERE earlier.session_ref = pending.session_ref
            AND earlier.sequence < pending.sequence
            AND earlier.published_at IS NULL
            AND earlier.dead_at IS NULL
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

-- A failure that says nothing about the event itself (platform down, timeout,
-- 5xx, bad signature): try again later, with a delay that doubles up to a minute.
-- name: ReleaseOutbox :exec
UPDATE outbox
SET locked_until = now() + make_interval(secs => LEAST(60::float8, power(2::float8, LEAST(GREATEST(attempts - 1, 0), 6)))),
    last_error = sqlc.arg(last_error)::text
WHERE event_id = $1
  AND published_at IS NULL;

-- The platform refused this event. Count it, back off, and mark it dead once it
-- has been refused max_rejections times. Returns whether the event is now dead.
-- name: RejectOutbox :one
UPDATE outbox
SET rejections = rejections + 1,
    last_error = sqlc.arg(last_error)::text,
    locked_until = now() + make_interval(secs => LEAST(60::float8, power(2::float8, LEAST(GREATEST(attempts - 1, 0), 6)))),
    dead_at = CASE WHEN rejections + 1 >= sqlc.arg(max_rejections)::int THEN now() END
WHERE event_id = $1
  AND published_at IS NULL
  AND dead_at IS NULL
RETURNING (dead_at IS NOT NULL)::boolean AS dead;

-- Events behind a dead event in the same session can never be accepted in order,
-- so they are dead too. Returns how many were swept.
-- name: DeadLetterBlockedOutbox :execrows
UPDATE outbox AS blocked
SET dead_at = now(),
    last_error = 'behind a dead event in the same session',
    locked_until = NULL
WHERE blocked.published_at IS NULL
  AND blocked.dead_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM outbox AS dead
      WHERE dead.session_ref = blocked.session_ref
        AND dead.sequence < blocked.sequence
        AND dead.dead_at IS NOT NULL
  );
