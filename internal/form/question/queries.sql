-- name: Create :one
WITH inserted AS (
    INSERT INTO questions (section_id, required, type, title, description, metadata, "order", source_id)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    RETURNING *
)
SELECT 
    i.id,
    i.section_id,
    i.required,
    i.type,
    i.title,
    i.description,
    i.metadata,
    i."order",
    i.source_id,
    i.created_at,
    i.updated_at,
    s.form_id
FROM inserted i
JOIN sections s ON i.section_id = s.id;

-- name: Update :one
WITH updated AS (
    UPDATE questions
    SET required = $3, type = $4, title = $5, description = $6, metadata = $7, source_id = $8, updated_at = now()
    WHERE questions.section_id = $1 AND questions.id = $2
    RETURNING *
)
SELECT 
    u.id,
    u.section_id,
    u.required,
    u.type,
    u.title,
    u.description,
    u.metadata,
    u."order",
    u.source_id,
    u.created_at,
    u.updated_at,
    s.form_id
FROM updated u
JOIN sections s ON u.section_id = s.id;

-- name: UpdateOrder :one
WITH shifted AS (
    UPDATE questions
    SET "order" = CASE
        -- Moving down: shift questions between old and new position up
        WHEN $3 > (SELECT q2."order" FROM questions q2 WHERE q2.id = $2 AND q2.section_id = $1) 
             AND questions."order" > (SELECT q2."order" FROM questions q2 WHERE q2.id = $2 AND q2.section_id = $1) 
             AND questions."order" <= $3
        THEN questions."order" - 1
        -- Moving up: shift questions between new and old position down
        WHEN $3 < (SELECT q2."order" FROM questions q2 WHERE q2.id = $2 AND q2.section_id = $1)
             AND questions."order" >= $3
             AND questions."order" < (SELECT q2."order" FROM questions q2 WHERE q2.id = $2 AND q2.section_id = $1)
        THEN questions."order" + 1
        ELSE questions."order"
    END,
    updated_at = now()
    WHERE questions.section_id = $1 AND questions.id != $2
    RETURNING questions.id
),
updated AS (
    UPDATE questions q
    SET "order" = $3, updated_at = now()
    WHERE q.id = $2 AND q.section_id = $1
    RETURNING q.*
)
SELECT 
    u.id,
    u.section_id,
    u.required,
    u.type,
    u.title,
    u.description,
    u.metadata,
    u."order",
    u.source_id,
    u.created_at,
    u.updated_at,
    s.form_id
FROM updated u
JOIN sections s ON u.section_id = s.id;

-- name: DeleteAndReorder :exec
WITH deleted_row AS (
    DELETE FROM questions q
    WHERE q.section_id = $1 AND q.id = $2
    RETURNING "order" as old_order, section_id
)
UPDATE questions q
SET "order" = q."order" - 1
FROM deleted_row
WHERE q.section_id = deleted_row.section_id
  AND "order" > deleted_row.old_order;

-- name: ListSectionsByFormID :many
SELECT *
FROM sections
WHERE form_id = $1;

-- name: ListSectionsWithAnswersByFormID :many
SELECT
    s.id as section_id,
    s.form_id,
    s.title,
    s.description,
    s.created_at,
    s.updated_at,
    q.id,
    q.required,
    q.type,
    q.title as question_title,
    q.description as question_description,
    q.metadata,
    q."order",
    q.source_id,
    q.created_at as question_created_at,
    q.updated_at as question_updated_at
FROM sections s
LEFT JOIN questions q ON s.id = q.section_id
WHERE s.form_id = $1
ORDER BY
    s.id ASC,
    q."order" ASC;

-- name: GetByID :one
SELECT 
    q.id,
    q.section_id,
    q.required,
    q.type,
    q.title,
    q.description,
    q.metadata,
    q."order",
    q.source_id,
    q.created_at,
    q.updated_at,
    s.form_id
FROM questions q
JOIN sections s ON q.section_id = s.id
WHERE q.id = $1;