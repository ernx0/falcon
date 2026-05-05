-- Recreate the flat assets table and rebuild it from the host-centric tables.

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

INSERT INTO assets(program_id, target_id, kind, value, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT program_id, target_id, 'subdomain', value, meta, out_of_scope, first_seen_at, last_seen_at FROM hosts
ON CONFLICT DO NOTHING;

INSERT INTO assets(program_id, kind, value, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT program_id, 'port',
       host(value) || ':' || (
         SELECT s.port FROM services s WHERE s.ip_id = i.id LIMIT 1
       ),
       meta, out_of_scope, first_seen_at, last_seen_at
FROM ips i
WHERE EXISTS (SELECT 1 FROM services s WHERE s.ip_id = i.id)
ON CONFLICT DO NOTHING;

INSERT INTO assets(program_id, kind, value, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT s.program_id, 'http',
       COALESCE(NULLIF(s.scheme,''), 'http') || '://' || h.value ||
         CASE WHEN (s.scheme='https' AND s.port=443) OR (s.scheme='http' AND s.port=80) THEN ''
              ELSE ':' || s.port::text END,
       s.meta, s.out_of_scope, s.first_seen_at, s.last_seen_at
FROM services s
JOIN hosts h ON h.id = s.host_id
ON CONFLICT DO NOTHING;

INSERT INTO assets(program_id, kind, value, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT program_id, 'js_endpoint', url, meta, out_of_scope, first_seen_at, last_seen_at FROM endpoints
ON CONFLICT DO NOTHING;

ALTER TABLE findings DROP COLUMN host_id;
ALTER TABLE findings DROP COLUMN service_id;
ALTER TABLE findings DROP COLUMN endpoint_id;
ALTER TABLE findings ADD COLUMN asset_id BIGINT REFERENCES assets(id) ON DELETE SET NULL;

DROP TABLE endpoints;
DROP TABLE services;
DROP TABLE host_ips;
DROP TABLE ips;
DROP TABLE hosts;
