-- name: Create :one
INSERT INTO responses (form_id, submitted_by)
VALUES ($1, $2)
RETURNING *;

-- name: Get :one
SELECT * FROM responses 
WHERE id = $1 AND form_id = $2;

-- name: GetByFormIDAndSubmittedBy :one
SELECT * FROM responses 
WHERE form_id = $1 AND submitted_by = $2;

-- name: ListByFormID :many
SELECT * FROM responses 
WHERE form_id = $1
ORDER BY created_at ASC;

-- name: Update :exec
UPDATE responses 
SET updated_at = now()
WHERE id = $1;

-- name: Delete :exec
DELETE FROM responses 
WHERE id = $1;

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM responses WHERE form_id = $1 AND submitted_by = $2);

-- name: CreateAnswer :one
INSERT INTO answers (response_id, question_id, type, value)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAnswersByResponseID :many
SELECT * FROM answers 
WHERE response_id = $1
ORDER BY created_at ASC;

-- name: GetAnswersByQuestionID :many
SELECT a.*, r.form_id, r.submitted_by FROM answers a
JOIN responses r ON a.response_id = r.id
WHERE a.question_id = $1 AND r.form_id = $2
ORDER BY a.created_at ASC;

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