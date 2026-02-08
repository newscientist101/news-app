-- name: GetUserByExeID :one
SELECT * FROM users WHERE exe_user_id = ?;

-- name: CreateUser :one
INSERT INTO users (exe_user_id, email, created_at, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: UpdateUserEmail :exec
UPDATE users SET email = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;
