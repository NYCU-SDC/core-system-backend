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

-- name: Update :one
WITH updated AS (
    UPDATE forms
    SET
        title = $2,
        description = $3,
        preview_message = $4,
        last_editor = $5,
        deadline = $6,
        publish_time = $7,
        message_after_submission = $8,
        google_sheet_url = $9,
        visibility = $10,
        dressing_color = $11,
        dressing_header_font = $12,
        dressing_question_font = $13,
        dressing_text_font = $14,
        updated_at = now()
    WHERE forms.id = $1
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
WHERE f.status <> 'archived'
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
  AND f.status <> 'archived'
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