ALTER TABLE findings RENAME TO reports;
ALTER INDEX IF EXISTS findings_pkey RENAME TO reports_pkey;
