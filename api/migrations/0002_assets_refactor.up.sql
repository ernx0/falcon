-- Refactor flat `assets` table into a host-centric model:
--   hosts       — canonical FQDN
--   ips         — IP addresses
--   host_ips    — many-to-many DNS resolution
--   services    — open port + protocol on a host or IP
--   endpoints   — discovered URL/path
-- Existing data is backfilled below; the old `assets` table is dropped at the end.

CREATE TABLE hosts (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  target_id BIGINT REFERENCES targets(id) ON DELETE SET NULL,
  value TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  out_of_scope BOOLEAN NOT NULL DEFAULT FALSE,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, value)
);
CREATE INDEX idx_hosts_program ON hosts(program_id) WHERE NOT out_of_scope;
CREATE INDEX idx_hosts_value_trgm ON hosts USING GIN (value gin_trgm_ops);
CREATE INDEX idx_hosts_target ON hosts(target_id) WHERE target_id IS NOT NULL;

CREATE TABLE ips (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  value INET NOT NULL,
  version SMALLINT NOT NULL DEFAULT 4,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  out_of_scope BOOLEAN NOT NULL DEFAULT FALSE,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, value)
);
CREATE INDEX idx_ips_program ON ips(program_id) WHERE NOT out_of_scope;

CREATE TABLE host_ips (
  host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  ip_id BIGINT NOT NULL REFERENCES ips(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT '',
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY(host_id, ip_id)
);
CREATE INDEX idx_host_ips_ip ON host_ips(ip_id);

CREATE TABLE services (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  host_id BIGINT REFERENCES hosts(id) ON DELETE CASCADE,
  ip_id BIGINT REFERENCES ips(id) ON DELETE CASCADE,
  port INT NOT NULL,
  proto TEXT NOT NULL DEFAULT 'tcp',
  scheme TEXT NOT NULL DEFAULT '',
  status_code INT,
  title TEXT NOT NULL DEFAULT '',
  tech TEXT[] NOT NULL DEFAULT '{}',
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  out_of_scope BOOLEAN NOT NULL DEFAULT FALSE,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK (host_id IS NOT NULL OR ip_id IS NOT NULL)
);
-- Distinct service per (host, port, proto) and per (ip, port, proto). Use partial
-- unique indexes so NULLs in the unused side don't collapse rows.
CREATE UNIQUE INDEX uq_services_host ON services(program_id, host_id, port, proto)
  WHERE host_id IS NOT NULL;
CREATE UNIQUE INDEX uq_services_ip ON services(program_id, ip_id, port, proto)
  WHERE host_id IS NULL AND ip_id IS NOT NULL;
CREATE INDEX idx_services_program ON services(program_id) WHERE NOT out_of_scope;
CREATE INDEX idx_services_host ON services(host_id) WHERE host_id IS NOT NULL;
CREATE INDEX idx_services_ip ON services(ip_id) WHERE ip_id IS NOT NULL;

CREATE TABLE endpoints (
  id BIGSERIAL PRIMARY KEY,
  program_id BIGINT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  host_id BIGINT REFERENCES hosts(id) ON DELETE CASCADE,
  service_id BIGINT REFERENCES services(id) ON DELETE SET NULL,
  url TEXT NOT NULL,
  method TEXT NOT NULL DEFAULT '',
  status_code INT,
  source TEXT NOT NULL DEFAULT '',
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  out_of_scope BOOLEAN NOT NULL DEFAULT FALSE,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(program_id, url)
);
CREATE INDEX idx_endpoints_program ON endpoints(program_id) WHERE NOT out_of_scope;
CREATE INDEX idx_endpoints_host ON endpoints(host_id) WHERE host_id IS NOT NULL;
CREATE INDEX idx_endpoints_service ON endpoints(service_id) WHERE service_id IS NOT NULL;
CREATE INDEX idx_endpoints_url_trgm ON endpoints USING GIN (url gin_trgm_ops);

-- ---- Backfill ----------------------------------------------------------------

-- 1) hosts from subdomain assets
INSERT INTO hosts(program_id, target_id, value, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT program_id, target_id, lower(value), meta, out_of_scope, first_seen_at, last_seen_at
FROM assets
WHERE kind = 'subdomain'
ON CONFLICT (program_id, value) DO NOTHING;

-- 2) hosts from http URLs (extract hostname)
INSERT INTO hosts(program_id, target_id, value, out_of_scope, first_seen_at, last_seen_at)
SELECT a.program_id, a.target_id,
       lower(split_part(split_part(regexp_replace(a.value, '^https?://', ''), '/', 1), ':', 1)),
       a.out_of_scope, a.first_seen_at, a.last_seen_at
FROM assets a
WHERE a.kind = 'http'
  AND length(regexp_replace(a.value, '^https?://', '')) > 0
ON CONFLICT (program_id, value) DO NOTHING;

-- 3) ips from `port` asset values like "1.2.3.4:443" (extract IP part)
INSERT INTO ips(program_id, value, out_of_scope, first_seen_at, last_seen_at)
SELECT a.program_id,
       split_part(a.value, ':', 1)::inet,
       a.out_of_scope, a.first_seen_at, a.last_seen_at
FROM assets a
WHERE a.kind = 'port'
  AND split_part(a.value, ':', 1) ~ '^\d+\.\d+\.\d+\.\d+$'
ON CONFLICT (program_id, value) DO NOTHING;

-- 4) services from http (host_id) and from port (ip_id)
WITH http_parsed AS (
  SELECT a.program_id, a.target_id, a.meta, a.out_of_scope,
         a.first_seen_at, a.last_seen_at,
         CASE WHEN a.value LIKE 'https://%' THEN 'https'
              WHEN a.value LIKE 'http://%'  THEN 'http'
              ELSE '' END AS scheme,
         lower(split_part(split_part(regexp_replace(a.value, '^https?://', ''), '/', 1), ':', 1)) AS host,
         CASE
           WHEN split_part(split_part(regexp_replace(a.value, '^https?://', ''), '/', 1), ':', 2) ~ '^\d+$'
             THEN split_part(split_part(regexp_replace(a.value, '^https?://', ''), '/', 1), ':', 2)::int
           WHEN a.value LIKE 'https://%' THEN 443
           WHEN a.value LIKE 'http://%'  THEN 80
           ELSE 0
         END AS port
  FROM assets a
  WHERE a.kind = 'http'
)
INSERT INTO services(program_id, host_id, port, proto, scheme, status_code, title, tech, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT p.program_id, h.id, p.port, 'tcp', p.scheme,
       NULLIF((p.meta->>'status')::int, 0),
       COALESCE(p.meta->>'title', ''),
       CASE
         WHEN jsonb_typeof(p.meta->'tech') = 'array'
           THEN ARRAY(SELECT jsonb_array_elements_text(p.meta->'tech'))
         ELSE '{}'::text[]
       END,
       p.meta, p.out_of_scope, p.first_seen_at, p.last_seen_at
FROM http_parsed p
JOIN hosts h ON h.program_id = p.program_id AND h.value = p.host
WHERE p.port > 0
ON CONFLICT (program_id, host_id, port, proto) WHERE host_id IS NOT NULL DO NOTHING;

WITH port_parsed AS (
  SELECT a.program_id, a.meta, a.out_of_scope, a.first_seen_at, a.last_seen_at,
         split_part(a.value, ':', 1) AS ip,
         CASE WHEN split_part(a.value, ':', 2) ~ '^\d+$'
              THEN split_part(a.value, ':', 2)::int ELSE 0 END AS port
  FROM assets a
  WHERE a.kind = 'port'
)
INSERT INTO services(program_id, ip_id, port, proto, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT p.program_id, i.id, p.port, 'tcp', p.meta, p.out_of_scope, p.first_seen_at, p.last_seen_at
FROM port_parsed p
JOIN ips i ON i.program_id = p.program_id AND i.value::text = p.ip
WHERE p.port > 0
ON CONFLICT (program_id, ip_id, port, proto) WHERE host_id IS NULL AND ip_id IS NOT NULL DO NOTHING;

-- 5) endpoints from js_endpoint
INSERT INTO endpoints(program_id, host_id, url, source, meta, out_of_scope, first_seen_at, last_seen_at)
SELECT a.program_id,
       h.id,
       a.value,
       COALESCE(a.meta->>'source', ''),
       a.meta, a.out_of_scope, a.first_seen_at, a.last_seen_at
FROM assets a
LEFT JOIN hosts h ON h.program_id = a.program_id
  AND h.value = lower(split_part(split_part(regexp_replace(a.value, '^https?://', ''), '/', 1), ':', 1))
WHERE a.kind = 'js_endpoint'
ON CONFLICT (program_id, url) DO NOTHING;

-- 6) findings.asset_id is unused (verified during migration); drop the column.
ALTER TABLE findings DROP COLUMN asset_id;
ALTER TABLE findings
  ADD COLUMN host_id BIGINT REFERENCES hosts(id) ON DELETE SET NULL,
  ADD COLUMN service_id BIGINT REFERENCES services(id) ON DELETE SET NULL,
  ADD COLUMN endpoint_id BIGINT REFERENCES endpoints(id) ON DELETE SET NULL;
CREATE INDEX idx_findings_host ON findings(host_id) WHERE host_id IS NOT NULL;

-- 7) drop old assets table
DROP TABLE assets;
