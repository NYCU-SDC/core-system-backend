-- name: ListByResponseID :many
SELECT * FROM answers
WHERE response_id = $1
ORDER BY created_at ASC;

-- name: ListByQuestionIDAndResponseID :one
SELECT * FROM answers
WHERE question_id = $1 AND response_id = $2
ORDER BY created_at ASC;

-- name: Get :one
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
WITH upsert AS (
INSERT INTO answers (response_id, question_id, value)
VALUES ($1, $2, $3)
ON CONFLICT (response_id, question_id)
    DO UPDATE SET
           value = excluded.value,
           updated_at = now()
       WHERE answers.value IS DISTINCT FROM excluded.value
           RETURNING *
           )
SELECT * FROM upsert
UNION ALL
SELECT * FROM answers
WHERE response_id = $1 AND question_id = $2
  AND NOT EXISTS (SELECT 1 FROM upsert);

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
        updated_at = now()
    WHERE answers.value IS DISTINCT FROM excluded.value
RETURNING *;

-- name: Update :one
UPDATE answers
SET value = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteByResponseID :exec
DELETE FROM answers
WHERE response_id = $1;
