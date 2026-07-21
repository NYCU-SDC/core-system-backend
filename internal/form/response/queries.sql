-- name: Create :one
INSERT INTO form_responses (form_id, submitted_by)
VALUES ($1, $2)
RETURNING *;

-- name: Get :one
SELECT * FROM form_responses
WHERE id = $1 AND form_id = $2;

-- name: GetFormID :one
SELECT form_id FROM form_responses
WHERE id = $1;

-- name: ListByFormIDAndSubmittedBy :many
SELECT * FROM form_responses
WHERE form_id = $1 AND submitted_by = $2
ORDER BY submitted_at DESC NULLS LAST;

-- name: ListByFormID :many
SELECT * FROM form_responses
WHERE form_id = $1
ORDER BY submitted_at DESC NULLS LAST;

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
SELECT EXISTS(SELECT 1 FROM form_responses WHERE id = $1);

-- name: ExistsByFormIDAndSubmittedBy :one
SELECT EXISTS(SELECT 1 FROM form_responses WHERE form_id = $1 AND submitted_by = $2);

-- name: ListSubmittedByFormID :many
SELECT * FROM form_responses
WHERE form_id = $1
  AND progress = 'SUBMITTED'
ORDER BY submitted_at ASC, id ASC;

-- name: GetEditInfo :one
SELECT
    r.progress,
    f.allow_edit_response
FROM form_responses r
         JOIN forms f ON f.id = r.form_id
WHERE r.id = $1;

-- name: GetFilterResponsesByQuestionsAndOptions :many
SELECT DISTINCT a.response_id
From answers a
         JOIN form_responses fr ON fr.id = a.response_id
WHERE fr.form_id = $1
  AND fr.progress = 'submitted'
  AND a.question_id = $2
  AND (
    (a.value::jsonb ->> 'choiceId')::uuid = ANY(@option_ids::uuid[])
        OR EXISTS (
          SELECT 1 FROM jsonb_array_elements(a.value::jsonb -> 'choices') AS c
          WHERE (c ->> 'choiceId')::uuid = ANY(@option_ids::uuid[])
        )
    );