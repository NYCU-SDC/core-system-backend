-- name: CreateMessage :one
INSERT INTO inbox_message (posted_by, type, content_id)
VALUES (@posted_by, @type, @content_id)
RETURNING *;

-- name: CreateUserInboxBulk :many
INSERT INTO user_inbox_messages (user_id, message_id)
SELECT unnest(@user_ids::uuid[]), @message_id::uuid
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
WHERE uim.id = @user_inbox_message_id AND uim.user_id = @user_id;

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
WHERE uim.user_id = @user_id
  AND (sqlc.narg(is_read)::boolean IS NULL OR uim.is_read = sqlc.narg(is_read))
  AND (sqlc.narg(is_starred)::boolean IS NULL OR uim.is_starred = sqlc.narg(is_starred))
  AND (uim.is_archived = COALESCE(sqlc.narg(is_archived)::boolean, false))
  AND (@search::text = '' OR @search::text IS NULL OR (
    CASE WHEN im.type = 'form' THEN f.title ELSE '' END ILIKE '%' || @search::text || '%'
    OR CASE WHEN im.type = 'form' THEN f.description ELSE '' END ILIKE '%' || @search::text || '%'
    OR CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) ELSE '' END ILIKE '%' || @search::text || '%'
  ))
LIMIT COALESCE(@page_limit::int, 10)
OFFSET COALESCE(@page_offset::int, 0);

-- name: ListCount :one
SELECT 
    COUNT(*) AS total
FROM user_inbox_messages uim
JOIN inbox_message im ON uim.message_id = im.id
LEFT JOIN forms f ON im.type = 'form' AND im.content_id = f.id
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
WHERE uim.user_id = @user_id
  AND (sqlc.narg(is_read)::boolean IS NULL OR uim.is_read = sqlc.narg(is_read))
  AND (sqlc.narg(is_starred)::boolean IS NULL OR uim.is_starred = sqlc.narg(is_starred))
  AND (uim.is_archived = COALESCE(sqlc.narg(is_archived)::boolean, false))
  AND (@search::text = '' OR @search::text IS NULL OR (
    CASE WHEN im.type = 'form' THEN f.title ELSE '' END ILIKE '%' || @search::text || '%'
    OR CASE WHEN im.type = 'form' THEN f.description ELSE '' END ILIKE '%' || @search::text || '%'
    OR CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) ELSE '' END ILIKE '%' || @search::text || '%'
  ));

-- name: UpdateByID :one
UPDATE user_inbox_messages AS uim
SET is_read = @is_read, is_starred = @is_starred, is_archived = @is_archived
FROM inbox_message AS im
LEFT JOIN forms f ON im.type = 'form' AND im.content_id = f.id
LEFT JOIN units u ON f.unit_id = u.id
LEFT JOIN units o ON u.org_id = o.id
WHERE uim.message_id = im.id AND uim.id = @id AND uim.user_id = @user_id
RETURNING uim.*, im.*,
CASE WHEN im.type = 'form' THEN COALESCE(f.preview_message, LEFT(f.description, 25)) END AS preview_message,
CASE WHEN im.type = 'form' THEN f.title END AS title,
CASE WHEN im.type = 'form' THEN COALESCE(o.name, u.name) END AS org_name,
CASE WHEN im.type = 'form' AND u.type = 'unit' THEN u.name END AS unit_name;
