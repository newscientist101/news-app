-- name: GetJob :one
SELECT * FROM jobs WHERE id = ? AND user_id = ?;

-- name: ListJobsByUser :many
SELECT * FROM jobs WHERE user_id = ? ORDER BY created_at DESC;

-- name: ListActiveJobs :many
SELECT * FROM jobs WHERE is_active = 1 AND (is_one_time = 0 OR status = 'pending');

-- name: CreateJob :one
INSERT INTO jobs (user_id, name, prompt, keywords, sources, region, frequency, is_one_time, is_active, status, next_run_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 'pending', ?)
RETURNING *;

-- name: UpdateJob :exec
UPDATE jobs
SET name = ?, prompt = ?, keywords = ?, sources = ?, region = ?, frequency = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = ?, last_run_at = ?, next_run_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = ? AND user_id = ?;

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = ?;

-- name: UpdateJobConversation :exec
UPDATE jobs SET current_conversation_id = ? WHERE id = ?;

-- name: DeactivateJob :exec
UPDATE jobs SET is_active = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?;
