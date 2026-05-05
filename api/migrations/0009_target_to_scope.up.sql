-- Final rename: drop "target" terminology everywhere on the DB level.
-- The `targets` table becomes `scope`; `target_id` foreign keys become
-- `scope_id`. Indexes are renamed too for grep-ability.

ALTER TABLE targets RENAME TO scope;
ALTER INDEX IF EXISTS targets_pkey            RENAME TO scope_pkey;
ALTER INDEX IF EXISTS idx_targets_program     RENAME TO idx_scope_program;
ALTER INDEX IF EXISTS idx_targets_next_run    RENAME TO idx_scope_next_run;
ALTER INDEX IF EXISTS targets_program_id_value_key RENAME TO scope_program_id_value_key;

-- runs.target_id → runs.scope_id
ALTER TABLE runs RENAME COLUMN target_id TO scope_id;
ALTER INDEX IF EXISTS idx_runs_target RENAME TO idx_runs_scope;

-- hosts.target_id → hosts.scope_id
ALTER TABLE hosts RENAME COLUMN target_id TO scope_id;
ALTER INDEX IF EXISTS idx_hosts_target RENAME TO idx_hosts_scope;
