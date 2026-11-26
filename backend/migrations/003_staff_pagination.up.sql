CREATE INDEX idx_submissions_staff_order ON submissions(created_at DESC, id DESC);
CREATE INDEX idx_assets_storage_key ON assets(storage_key);
