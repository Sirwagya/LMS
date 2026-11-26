ALTER TABLE outbox_events
    ADD COLUMN relay_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN next_relay_at timestamptz NOT NULL DEFAULT '-infinity',
    ADD COLUMN relay_failed_at timestamptz,
    ADD COLUMN relay_error text;

DROP INDEX idx_outbox_unpublished;
CREATE INDEX idx_outbox_unpublished ON outbox_events(next_relay_at, created_at, id)
    WHERE published_at IS NULL AND relay_failed_at IS NULL;
CREATE INDEX idx_submission_asset_ref ON submissions(asset_id) WHERE asset_id IS NOT NULL;
CREATE INDEX idx_submission_staged_assets ON assets(created_at, id)
    WHERE storage_key LIKE 'submissions/%';
