-- name: ListByResponseID :many
SELECT * FROM answers
WHERE response_id = $1
ORDER BY created_at ASC;

-- name: GetIDByResponseIDAndQuestionID :one
SELECT id FROM answers
WHERE response_id = $1 AND question_id = $2;

-- name: GetByResponseIDAndQuestionID :one
SELECT * FROM answers
WHERE response_id = $1 AND question_id = $2;

-- name: Create :one
INSERT INTO answers (response_id, question_id, value)
VALUES ($1, $2, $3)
RETURNING *;

-- name: Upsert :one
INSERT INTO answers (response_id, question_id, value)
VALUES ($1, $2, $3)
    ON CONFLICT (response_id, question_id)
DO UPDATE
    SET
        value = excluded.value,
        updated_at = CASE
    WHEN answers.value IS DISTINCT FROM excluded.value
        THEN now()
        ELSE answers.updated_at
END
RETURNING *;

-- name: BatchUpsert :many
INSERT INTO answers (response_id, question_id, value)
SELECT
    unnest(@response_ids::uuid []),
    unnest(@question_ids::uuid []),
    unnest(@values::jsonb [])
ON CONFLICT (response_id, question_id)
DO UPDATE
    SET
        value = excluded.value,
        updated_at = CASE
    WHEN answers.value IS DISTINCT FROM excluded.value
        THEN now()
        ELSE answers.updated_at
END
RETURNING *;

-- name: Update :one
UPDATE answers
SET value = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteByResponseID :exec
DELETE FROM answers
WHERE response_id = $1;

-- name: Get :one
SELECT id, response_id, question_id, value, created_at, updated_at
FROM answers
WHERE id = $1;

-- name: ListAnswersForExport :many
SELECT a.id, a.response_id, a.question_id, a.value, a.created_at, a.updated_at, fr.submitted_at
FROM answers a
JOIN form_responses fr ON a.response_id = fr.id
WHERE fr.form_id = $1
  AND fr.progress = 'submitted'
  AND a.question_id = ANY($2::uuid[]);