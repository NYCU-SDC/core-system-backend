-- name: Create :one
INSERT INTO form_responses (form_id, submitted_by)
VALUES ($1, $2)
RETURNING *;

-- name: Get :one
SELECT * FROM form_responses
WHERE id = $1;

-- name: GetFormIDByID :one
SELECT form_id FROM form_responses
WHERE id = $1;

-- name: ListByFormID :many
SELECT * FROM form_responses
WHERE form_id = $1
ORDER BY created_at ASC;

-- name: ListBySubmittedBy :many
SELECT * FROM form_responses
WHERE submitted_by = $1
ORDER BY submitted_at DESC NULLS LAST;

-- name: Update :exec
UPDATE form_responses
SET updated_at = now(), progress = $2
WHERE id = $1;

-- name: UpdateSubmitted :one
UPDATE form_responses
SET submitted_at = now(), progress = 'submitted'
WHERE id = $1
RETURNING *;

-- name: Delete :exec
DELETE FROM form_responses
WHERE id = $1;

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM form_responses WHERE form_id = $1 AND submitted_by = $2);