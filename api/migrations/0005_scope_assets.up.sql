-- scope_assets stores in-scope or out-of-scope items that aren't scannable
-- hosts: mobile apps, github repos, browser extensions, descriptive
-- labels, etc. The worker doesn't touch this table — it's reference data
-- for the operator.
CREATE TABLE scope_assets (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  in_scope BOOLEAN NOT NULL,
  kind TEXT NOT NULL DEFAULT 'other',
  value TEXT NOT NULL,
  source_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, in_scope, value)
);
CREATE INDEX idx_scope_assets_program ON scope_assets(program_id);
