CREATE TABLE IF NOT EXISTS scope_assets (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  in_scope BOOLEAN NOT NULL,
  kind TEXT NOT NULL DEFAULT 'other',
  value TEXT NOT NULL,
  source_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, in_scope, value)
);

ALTER TABLE targets       DROP COLUMN IF EXISTS source_url;
ALTER TABLE out_of_scopes DROP COLUMN IF EXISTS source_url;
