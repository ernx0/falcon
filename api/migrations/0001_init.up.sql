CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  email TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE programs (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT UNIQUE NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE targets (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  value TEXT NOT NULL,
  kind TEXT NOT NULL,
  schedule_cron TEXT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, value)
);
CREATE INDEX idx_targets_next_run ON targets(next_run_at) WHERE enabled;
CREATE INDEX idx_targets_program ON targets(program_id);

CREATE TABLE out_of_scopes (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  pattern TEXT NOT NULL,
  kind TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, pattern)
);
CREATE INDEX idx_oos_program ON out_of_scopes(program_id);

CREATE TABLE runs (
  id BIGSERIAL PRIMARY KEY,
  target_id BIGINT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  trigger TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_runs_target ON runs(target_id, created_at DESC);
CREATE INDEX idx_runs_program ON runs(program_id, created_at DESC);
CREATE INDEX idx_runs_status ON runs(status) WHERE status IN ('queued','running');

CREATE TABLE run_steps (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  tool TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  artifact_path TEXT NOT NULL DEFAULT '',
  stats JSONB NOT NULL DEFAULT '{}'::jsonb,
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_run_steps_run ON run_steps(run_id);

CREATE TABLE assets (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  target_id BIGINT REFERENCES targets(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,
  value TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  out_of_scope BOOLEAN NOT NULL DEFAULT FALSE,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, kind, value)
);
CREATE INDEX idx_assets_program_kind ON assets(program_id, kind) WHERE NOT out_of_scope;
CREATE INDEX idx_assets_value_trgm ON assets USING GIN (value gin_trgm_ops);
CREATE INDEX idx_assets_meta ON assets USING GIN (meta jsonb_path_ops);
CREATE INDEX idx_assets_target ON assets(target_id) WHERE target_id IS NOT NULL;

CREATE TABLE findings (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  asset_id BIGINT REFERENCES assets(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  severity TEXT NOT NULL,
  status TEXT NOT NULL,
  description_md TEXT NOT NULL DEFAULT '',
  poc TEXT NOT NULL DEFAULT '',
  cvss TEXT NOT NULL DEFAULT '',
  external_url TEXT NOT NULL DEFAULT '',
  reward_amount NUMERIC(12,2),
  reward_currency TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_findings_program ON findings(program_id, status);
CREATE INDEX idx_findings_search_trgm ON findings USING GIN ((title || ' ' || COALESCE(description_md,'')) gin_trgm_ops);
