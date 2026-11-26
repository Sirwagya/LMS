DROP INDEX idx_submission_staged_assets;
DROP INDEX idx_submission_asset_ref;
DROP INDEX idx_outbox_unpublished;
CREATE INDEX idx_outbox_unpublished ON outbox_events(created_at, id) WHERE published_at IS NULL;
ALTER TABLE outbox_events
    DROP COLUMN relay_attempts,
    DROP COLUMN next_relay_at,
    DROP COLUMN relay_failed_at,
    DROP COLUMN relay_error;
