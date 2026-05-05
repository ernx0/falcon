ALTER TABLE reports RENAME TO findings;
ALTER INDEX IF EXISTS reports_pkey RENAME TO findings_pkey;
