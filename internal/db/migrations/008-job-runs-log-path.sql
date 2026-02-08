-- Add log_path column to job_runs for storing execution logs

ALTER TABLE job_runs ADD COLUMN log_path TEXT NOT NULL DEFAULT '';

-- Record execution of this migration
INSERT OR IGNORE INTO migrations (migration_number, migration_name)
VALUES (008, '008-job-runs-log-path');
