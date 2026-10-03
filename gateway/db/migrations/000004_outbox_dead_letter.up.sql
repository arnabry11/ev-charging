ALTER TABLE outbox
    ADD COLUMN rejections integer NOT NULL DEFAULT 0 CHECK (rejections >= 0),
    ADD COLUMN last_error text,
    ADD COLUMN dead_at timestamptz,
    ADD CONSTRAINT outbox_dead_is_unpublished CHECK (dead_at IS NULL OR published_at IS NULL);

-- Dead events are out of the delivery queue, so the queue index leaves them out.
DROP INDEX outbox_unpublished;
CREATE INDEX outbox_unpublished
    ON outbox (session_ref, sequence)
    WHERE published_at IS NULL AND dead_at IS NULL;

CREATE INDEX outbox_dead
    ON outbox (session_ref, sequence)
    WHERE dead_at IS NOT NULL;
