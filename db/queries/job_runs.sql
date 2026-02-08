-- name: CreateJobRun :one
INSERT INTO job_runs (job_id, status, started_at)
VALUES (?, 'running', CURRENT_TIMESTAMP)
RETURNING *;

-- name: CompleteJobRun :exec
UPDATE job_runs
SET status = ?, error_message = ?, articles_saved = ?, duplicates_skipped = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateJobRunLogPath :exec
UPDATE job_runs SET log_path = ? WHERE id = ?;

-- name: GetJobRunLogPath :one
SELECT log_path FROM job_runs jr
JOIN jobs j ON jr.job_id = j.id
WHERE jr.id = ? AND j.user_id = ?;

-- name: ListJobRunsByJob :many
SELECT * FROM job_runs WHERE job_id = ? ORDER BY started_at DESC LIMIT 10;

-- name: ListRunningJobRuns :many
SELECT jr.*, j.name as job_name, j.user_id as job_user_id
FROM job_runs jr
JOIN jobs j ON jr.job_id = j.id
WHERE jr.status = 'running' AND j.user_id = ?
ORDER BY jr.started_at DESC;

-- name: ListRecentJobRuns :many
SELECT jr.*, j.name as job_name, j.user_id as job_user_id
FROM job_runs jr
JOIN jobs j ON jr.job_id = j.id
WHERE j.user_id = ?
ORDER BY jr.started_at DESC
LIMIT ?;

-- name: GetJobRun :one
SELECT jr.*, j.name as job_name, j.user_id as job_user_id
FROM job_runs jr
JOIN jobs j ON jr.job_id = j.id
WHERE jr.id = ? AND j.user_id = ?;

-- name: CancelJobRun :exec
UPDATE job_runs
SET status = 'cancelled', error_message = 'Cancelled by user', completed_at = CURRENT_TIMESTAMP
WHERE id = ?;
