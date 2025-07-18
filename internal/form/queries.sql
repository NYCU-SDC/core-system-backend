-- name: Create :one
INSERT INTO forms (title, description, last_edit)
VALUES ($1, $2, $3) 
RETURNING *;

-- name: Update :one
UPDATE forms
SET title = $2, description = $3, last_edit = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: Delete :exec
DELETE FROM forms WHERE id = $1;

-- name: GetByID :one
SELECT * FROM forms WHERE id = $1;

-- name: List :many
SELECT * FROM forms ORDER BY updated_at DESC;
