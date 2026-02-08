-- Add duplicates_skipped column to job_runs table

ALTER TABLE job_runs ADD COLUMN duplicates_skipped INTEGER DEFAULT 0;

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (006, '006-duplicates-tracking');
