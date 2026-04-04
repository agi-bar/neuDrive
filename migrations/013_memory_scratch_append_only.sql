ALTER TABLE memory_scratch
    DROP CONSTRAINT IF EXISTS memory_scratch_user_id_date_source_key;

CREATE INDEX IF NOT EXISTS idx_memory_scratch_user_date_source
    ON memory_scratch(user_id, date, source);

CREATE INDEX IF NOT EXISTS idx_memory_scratch_user_created_at
    ON memory_scratch(user_id, created_at DESC);
