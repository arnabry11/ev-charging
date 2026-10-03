DROP INDEX IF EXISTS outbox_dead;
DROP INDEX IF EXISTS outbox_unpublished;
CREATE INDEX outbox_unpublished
    ON outbox (session_ref, sequence)
    WHERE published_at IS NULL;

ALTER TABLE outbox
    DROP CONSTRAINT IF EXISTS outbox_dead_is_unpublished,
    DROP COLUMN IF EXISTS dead_at,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS rejections;
