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

-- name: DeleteByFormID :execrows
DELETE FROM form_highlights
WHERE form_id = $1;

-- name: UpdateDisplayTitleByFormID :one
UPDATE form_highlights
SET display_title = $2,
    updated_at = now()
WHERE form_id = $1
RETURNING *;

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
SELECT a.value
FROM answers a
JOIN form_responses fr ON fr.id = a.response_id
WHERE a.question_id = $1
  AND fr.progress = 'submitted';
