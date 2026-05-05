-- Unify scope into targets / out_of_scopes. Non-host items (mobile apps,
-- github repos, descriptive labels) live in the same tables now and are
-- distinguished by kind. The worker only scans scannable kinds.
ALTER TABLE targets       ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE out_of_scopes ADD COLUMN source_url TEXT NOT NULL DEFAULT '';

-- Migrate any existing scope_assets rows back into targets / out_of_scopes
-- so we don't lose data when the table goes away.
INSERT INTO targets(program_id, value, kind, enabled, source_url)
SELECT program_id, value, kind, false, source_url
FROM scope_assets WHERE in_scope
ON CONFLICT (program_id, value) DO NOTHING;

INSERT INTO out_of_scopes(program_id, pattern, kind, source_url)
SELECT program_id, value, kind, source_url
FROM scope_assets WHERE NOT in_scope
ON CONFLICT (program_id, pattern) DO NOTHING;

DROP TABLE IF EXISTS scope_assets;
