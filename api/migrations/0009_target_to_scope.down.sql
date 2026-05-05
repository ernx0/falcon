ALTER TABLE scope RENAME TO targets;
ALTER INDEX IF EXISTS scope_pkey            RENAME TO targets_pkey;
ALTER INDEX IF EXISTS idx_scope_program     RENAME TO idx_targets_program;
ALTER INDEX IF EXISTS idx_scope_next_run    RENAME TO idx_targets_next_run;
ALTER INDEX IF EXISTS scope_program_id_value_key RENAME TO targets_program_id_value_key;

ALTER TABLE runs RENAME COLUMN scope_id TO target_id;
ALTER INDEX IF EXISTS idx_runs_scope RENAME TO idx_runs_target;

ALTER TABLE hosts RENAME COLUMN scope_id TO target_id;
ALTER INDEX IF EXISTS idx_hosts_scope RENAME TO idx_hosts_target;
