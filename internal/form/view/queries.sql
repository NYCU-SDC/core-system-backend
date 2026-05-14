-- name: Get :one
SELECT * FROM views
WHERE id = $1 AND form_id = $2;

-- name: ListByFormID :many
SELECT * FROM views
WHERE form_id = $1
ORDER BY "order" ASC;

-- name: ListTitlesByFormID :many
SELECT title FROM views
WHERE form_id = $1;

-- name: MaxOrder :one
SELECT COALESCE(MAX("order"), -1)::integer FROM views
WHERE form_id = $1;

-- name: Create :one
INSERT INTO views (form_id, title, "order")
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateTitle :one
UPDATE views
SET title = $3, updated_at = now()
WHERE id = $1 AND form_id = $2
RETURNING *;

-- name: UpdateOrder :one
WITH shifted AS (
    UPDATE views
    SET "order" = CASE
        -- Moving down: shift views between old and new position up by 1
        WHEN $3 > (SELECT v2."order" FROM views v2 WHERE v2.id = $2 AND v2.form_id = $1)
             AND views."order" > (SELECT v2."order" FROM views v2 WHERE v2.id = $2 AND v2.form_id = $1)
             AND views."order" <= $3
        THEN views."order" - 1
        -- Moving up: shift views between new and old position down by 1
        WHEN $3 < (SELECT v2."order" FROM views v2 WHERE v2.id = $2 AND v2.form_id = $1)
             AND views."order" >= $3
             AND views."order" < (SELECT v2."order" FROM views v2 WHERE v2.id = $2 AND v2.form_id = $1)
        THEN views."order" + 1
        ELSE views."order"
    END,
    updated_at = now()
    WHERE views.form_id = $1 AND views.id != $2
    RETURNING views.id
),
updated AS (
    UPDATE views
    SET "order" = $3, updated_at = now()
    WHERE id = $2 AND form_id = $1
    RETURNING *
)
SELECT * FROM updated;

-- name: UpdateLocked :one
UPDATE views
SET locked = $3, updated_at = now()
WHERE id = $1 AND form_id = $2
RETURNING *;

-- name: ShiftOrders :exec
UPDATE views
SET "order" = "order" + $3, updated_at = now()
WHERE form_id = $1 AND "order" > $2;

-- name: DeleteIfUnlocked :one
DELETE FROM views
WHERE id = $1 AND form_id = $2 AND locked = FALSE
RETURNING id, "order";

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM views WHERE id = $1 AND form_id = $2);

-- name: FormExists :one
SELECT EXISTS(SELECT 1 FROM forms WHERE id = $1);
