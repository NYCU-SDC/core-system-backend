-- name: GetByFormID :one
SELECT *
FROM form_highlights
WHERE form_id = $1;

-- name: UpsertByFormID :one
INSERT INTO form_highlights (form_id, question_id, display_title)
VALUES ($1, $2, $3)
ON CONFLICT (form_id)
DO UPDATE SET
    question_id = EXCLUDED.question_id,
    display_title = EXCLUDED.display_title,
    updated_at = now()
RETURNING *;

-- name: DeleteByFormID :exec
DELETE FROM form_highlights
WHERE form_id = $1;

-- name: GetQuestionByFormIDAndQuestionID :one
SELECT
    q.id,
    q.type,
    q.title,
    q.metadata
FROM questions q
JOIN sections s ON s.id = q.section_id
WHERE s.form_id = $1
  AND q.id = $2;

-- name: ListAnswerValuesByQuestionID :many
SELECT value
FROM answers
WHERE question_id = $1;
