-- name: Create :one
INSERT INTO questions (form_id, required, type, label, description, "order")
VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING *;

-- name: Update :one
UPDATE questions
SET required = $3, type = $4, label = $5, description = $6, "order" = $7, updated_at = now()
WHERE form_id = $1 AND id = $2
    RETURNING *;

-- name: Delete :exec
DELETE FROM questions WHERE form_id = $1 AND id = $2;

-- name: ListByFormID :many
SELECT * FROM questions WHERE form_id = $1 ORDER BY "order";

-- name: GetByID :one
SELECT * FROM questions WHERE id = $1;