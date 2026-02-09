-- name: Create :one
INSERT INTO form_responses (form_id, submitted_by)
VALUES ($1, $2)
RETURNING *;

-- name: Get :one
SELECT * FROM form_responses
WHERE id = $1 AND form_id = $2;

-- name: GetByFormIDAndSubmittedBy :one
SELECT * FROM form_responses
WHERE form_id = $1 AND submitted_by = $2;

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

-- name: Delete :exec
DELETE FROM form_responses
WHERE id = $1;

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM form_responses WHERE form_id = $1 AND submitted_by = $2);

-- name: CreateAnswer :one
INSERT INTO answers (response_id, question_id, type, value)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAnswersByResponseID :many
SELECT * FROM answers 
WHERE response_id = $1
ORDER BY created_at ASC;

-- name: GetAnswersByQuestionID :one
SELECT * FROM answers
WHERE question_id = $1 AND response_id = $2
ORDER BY created_at ASC;

-- name: DeleteAnswersByResponseID :exec
DELETE FROM answers 
WHERE response_id = $1;

-- name: UpdateAnswer :one
UPDATE answers 
SET value = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CheckAnswerContent :one
SELECT EXISTS(SELECT 1 FROM answers WHERE response_id = $1 AND question_id = $2 AND value = $3);

-- name: AnswerExists :one
SELECT EXISTS(SELECT 1 FROM answers WHERE response_id = $1 AND question_id = $2);

-- name: GetAnswerID :one
SELECT id FROM answers WHERE response_id = $1 AND question_id = $2;