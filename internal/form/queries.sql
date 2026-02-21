-- name: Create :one
WITH created AS (
    INSERT INTO forms (
                       title,
                       description,
                       preview_message,
                       unit_id,
                       last_editor,
                       deadline,
                       publish_time,
                       message_after_submission,
                       google_sheet_url,
                       visibility,
                       dressing_color,
                       dressing_header_font,
                       dressing_question_font,
                       dressing_text_font
                       )
    VALUES (
        $1, $2, $3, $4, $5,
        $6, $7, $8, $9, $10,
        $11, $12, $13, $14
    )
    RETURNING *
),
workflow_created AS (
    INSERT INTO workflow_versions (form_id, last_editor, workflow)
    SELECT 
        id, 
        last_editor,
        jsonb_build_array(
            jsonb_build_object(
                'id', start_node_id,
                'label', '開始表單',
                'type', 'start',
                'next', end_node_id
            ),
            jsonb_build_object(
                'id', end_node_id,
                'label', '確認/送出',
                'type', 'end'
            )
        )
    FROM created, LATERAL (
        SELECT gen_random_uuid() AS start_node_id, gen_random_uuid() AS end_node_id
    ) AS node_ids
)
SELECT 
    f.*,
    u.name as unit_name,
    o.name as org_name,
    usr.name as last_editor_name,
    usr.username as last_editor_username,
    usr.avatar_url as last_editor_avatar_url,
    usr.emails as last_editor_email
FROM created f
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
LEFT JOIN users_with_emails usr ON f.last_editor = usr.id;  

-- name: Patch :one
WITH updated AS (
    UPDATE forms
    SET
        title = COALESCE(sqlc.narg('title')::text, forms.title),
        description = COALESCE(sqlc.narg('description')::text, forms.description),
        preview_message = COALESCE(sqlc.narg('preview_message')::text, forms.preview_message),
        last_editor = sqlc.arg('last_editor'),
        deadline = COALESCE(sqlc.narg('deadline')::timestamptz, forms.deadline),
        publish_time = COALESCE(sqlc.narg('publish_time')::timestamptz, forms.publish_time),
        message_after_submission = COALESCE(sqlc.narg('message_after_submission')::text, forms.message_after_submission),
        google_sheet_url = COALESCE(sqlc.narg('google_sheet_url')::text, forms.google_sheet_url),
        visibility = COALESCE(sqlc.narg('visibility')::visibility, forms.visibility),
        dressing_color = COALESCE(sqlc.narg('dressing_color')::text, forms.dressing_color),
        dressing_header_font = COALESCE(sqlc.narg('dressing_header_font')::text, forms.dressing_header_font),
        dressing_question_font = COALESCE(sqlc.narg('dressing_question_font')::text, forms.dressing_question_font),
        dressing_text_font = COALESCE(sqlc.narg('dressing_text_font')::text, forms.dressing_text_font),
        updated_at = now()
    WHERE forms.id = sqlc.arg('id')
    RETURNING *
)
SELECT 
    f.*,
    u.name as unit_name,
    o.name as org_name,
    usr.name as last_editor_name,
    usr.username as last_editor_username,
    usr.avatar_url as last_editor_avatar_url,
    usr.emails as last_editor_email
FROM updated f
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
LEFT JOIN users_with_emails usr ON f.last_editor = usr.id;

-- name: Delete :exec
DELETE FROM forms WHERE id = $1;

-- name: GetByID :one
SELECT 
    f.*,
    u.name as unit_name,
    o.name as org_name,
    usr.name as last_editor_name,
    usr.username as last_editor_username,
    usr.avatar_url as last_editor_avatar_url,
    usr.emails as last_editor_email
FROM forms f
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
LEFT JOIN users_with_emails usr ON f.last_editor = usr.id
WHERE f.id = $1;

-- name: List :many
SELECT 
    f.*,
    u.name as unit_name,
    o.name as org_name,
    usr.name as last_editor_name,
    usr.username as last_editor_username,
    usr.avatar_url as last_editor_avatar_url,
    usr.emails as last_editor_email
FROM forms f
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
LEFT JOIN users_with_emails usr ON f.last_editor = usr.id
WHERE (f.status <> 'archived' OR sqlc.narg(include_archived)::boolean IS TRUE)
ORDER BY f.updated_at DESC;

-- name: ListByUnit :many
SELECT 
    f.*,
    u.name as unit_name,
    o.name as org_name,
    usr.name as last_editor_name,
    usr.username as last_editor_username,
    usr.avatar_url as last_editor_avatar_url,
    usr.emails as last_editor_email
FROM forms f
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
LEFT JOIN users_with_emails usr ON f.last_editor = usr.id
WHERE f.unit_id = $1
AND (f.status <> 'archived' OR sqlc.narg(include_archived)::boolean IS TRUE)
ORDER BY f.updated_at DESC;

-- name: SetStatus :one
UPDATE forms
SET status = $2, last_editor = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UploadCoverImage :one
WITH upsert AS (
    INSERT INTO form_covers (form_id, image_data)
    VALUES ($1, $2)
    ON CONFLICT (form_id) DO UPDATE
        SET image_data = EXCLUDED.image_data,
            updated_at = now()
    RETURNING form_id
)
UPDATE forms
SET cover_image_url = $3,
    updated_at = now()
WHERE id = (SELECT form_id FROM upsert)
RETURNING id;

-- name: GetCoverImage :one
SELECT image_data FROM form_covers WHERE form_id = $1;

-- name: GetUnitIDByFormID :one
SELECT unit_id
FROM forms
WHERE id = $1;

-- name: GetUnitIDBySectionID :one
SELECT f.unit_id
FROM sections s
JOIN forms f ON s.form_id = f.id
WHERE s.id = $1;