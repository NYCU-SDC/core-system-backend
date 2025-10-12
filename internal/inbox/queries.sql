-- name: CreateMessage :one
INSERT INTO inbox_message (posted_by, type, content_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateUserInboxBulk :many
INSERT INTO user_inbox_messages (user_id, message_id)
SELECT unnest($1::uuid[]), $2::uuid
RETURNING *;

-- name: GetByID :one
SELECT 
    uim.*,
    im.*,
    CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) END AS preview_message,
    CASE WHEN im.type = 'form' THEN f.title END AS title,
    CASE WHEN im.type = 'form' THEN COALESCE(o.name, u.name) END AS org_name,
    CASE WHEN im.type = 'form' AND u.type = 'unit' THEN u.name END AS unit_name
FROM user_inbox_messages uim
JOIN inbox_message im ON uim.message_id = im.id
LEFT JOIN forms f ON im.type = 'form' AND im.content_id = f.id
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
WHERE uim.id = $1 AND uim.user_id = $2;

-- name: List :many
SELECT 
    uim.*,
    im.*,
    CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) END AS preview_message,
    CASE WHEN im.type = 'form' THEN f.title END AS title,
    CASE WHEN im.type = 'form' THEN COALESCE(o.name, u.name) END AS org_name,
    CASE WHEN im.type = 'form' AND u.type = 'unit' THEN u.name END AS unit_name
FROM user_inbox_messages uim
JOIN inbox_message im ON uim.message_id = im.id
LEFT JOIN forms f ON im.type = 'form' AND im.content_id = f.id
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
WHERE uim.user_id = $1 AND uim.is_archived = false;

-- name: UpdateByID :one
UPDATE user_inbox_messages AS uim
SET is_read = $3, is_starred = $4, is_archived = $5
FROM inbox_message AS im
LEFT JOIN forms f ON im.type = 'form' AND im.content_id = f.id
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
WHERE uim.message_id = im.id AND uim.id = $1 AND uim.user_id = $2
RETURNING uim.*, im.*,
CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) END AS preview_message,
CASE WHEN im.type = 'form' THEN f.title END AS title,
CASE WHEN im.type = 'form' THEN COALESCE(o.name, u.name) END AS org_name,
CASE WHEN im.type = 'form' AND u.type = 'unit' THEN u.name END AS unit_name;
