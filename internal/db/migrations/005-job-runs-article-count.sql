-- Add articles_saved column to track how many articles were saved during the run
ALTER TABLE job_runs ADD COLUMN articles_saved INTEGER DEFAULT 0;
