-- name: GetArticle :one
SELECT * FROM articles WHERE id = ? AND user_id = ?;

-- name: ListArticlesByUser :many
SELECT * FROM articles WHERE user_id = ? ORDER BY retrieved_at DESC LIMIT ? OFFSET ?;

-- name: ListArticlesByJob :many
SELECT * FROM articles WHERE job_id = ? ORDER BY retrieved_at DESC;

-- name: CreateArticle :one
INSERT INTO articles (job_id, user_id, title, url, summary, content_path, retrieved_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
RETURNING *;

-- name: DeleteArticle :exec
DELETE FROM articles WHERE id = ? AND user_id = ?;

-- name: CountArticlesByUser :one
SELECT COUNT(*) as count FROM articles WHERE user_id = ?;

-- name: ListArticlesByUserSince :many
SELECT * FROM articles WHERE user_id = ? AND retrieved_at >= ? ORDER BY retrieved_at DESC LIMIT ? OFFSET ?;

-- name: CountArticlesByUserSince :one
SELECT COUNT(*) as count FROM articles WHERE user_id = ? AND retrieved_at >= ?;

-- name: ListArticlesByUserDateRange :many
SELECT * FROM articles WHERE user_id = ? AND retrieved_at >= ? AND retrieved_at <= ? ORDER BY retrieved_at DESC LIMIT ? OFFSET ?;

-- name: CountArticlesByUserDateRange :one
SELECT COUNT(*) as count FROM articles WHERE user_id = ? AND retrieved_at >= ? AND retrieved_at <= ?;

-- name: SearchArticlesByUser :many
SELECT * FROM articles 
WHERE user_id = ? AND (title LIKE ? OR summary LIKE ?)
ORDER BY retrieved_at DESC LIMIT ? OFFSET ?;

-- name: CountSearchArticlesByUser :one
SELECT COUNT(*) FROM articles 
WHERE user_id = ? AND (title LIKE ? OR summary LIKE ?);


-- name: ListArticlesByJobPaginated :many
SELECT * FROM articles WHERE job_id = ? AND user_id = ? ORDER BY retrieved_at DESC LIMIT ? OFFSET ?;

-- name: CountArticlesByJob :one
SELECT COUNT(*) FROM articles WHERE job_id = ? AND user_id = ?;
