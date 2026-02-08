-- name: GetPreferences :one
SELECT * FROM preferences WHERE user_id = ?;

-- name: CreatePreferences :one
INSERT INTO preferences (user_id, system_prompt, discord_webhook, notify_success, notify_failure)
VALUES (?, '', '', 0, 0)
RETURNING *;

-- name: UpdatePreferences :exec
UPDATE preferences 
SET system_prompt = ?, discord_webhook = ?, notify_success = ?, notify_failure = ?, updated_at = CURRENT_TIMESTAMP
WHERE user_id = ?;
