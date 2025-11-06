-- name: Create :one
INSERT INTO units (name, org_id, description, metadata, type, parent_id)
VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING *;

-- name: GetByID :one
SELECT * FROM units WHERE id = $1;

-- name: GetAllOrganizations :many
SELECT u.*, sh.slug
FROM units u
LEFT JOIN slug_history sh ON sh.org_id = u.id
WHERE u.type = 'organization' AND sh.ended_at IS NULL;

-- name: ListOrganizationsOfUser :many
SELECT u.*, sh.slug
FROM unit_members um
JOIN units u ON um.unit_id = u.id
LEFT JOIN slug_history sh ON sh.org_id = u.id
WHERE u.type = 'organization'
    AND um.member_id = $1
    AND sh.ended_at IS NULL;

-- name: GetOrganizationByIDWithSlug :one
SELECT u.*, sh.slug
FROM units u
LEFT JOIN slug_history sh ON sh.org_id = u.id
WHERE u.id = $1 AND u.type = 'organization' AND  sh.ended_at IS NULL;

-- name: Update :one
UPDATE units
SET name = $2,
    description = $3,
    metadata = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateParent :one
UPDATE units
SET parent_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: Delete :exec
DELETE FROM units WHERE id = $1;

-- name: ListSubUnits :many
SELECT * FROM units WHERE parent_id = $1;

-- name: ListSubUnitIDs :many
SELECT id FROM units WHERE parent_id = $1;

-- name: AddMember :one
WITH inserted_member AS (
    INSERT INTO unit_members (unit_id, member_id)
    SELECT sqlc.arg(unit_id), user_emails.user_id
    FROM user_emails
        WHERE user_emails.value = sqlc.arg(member_email)
    ON CONFLICT (unit_id, member_id) DO UPDATE
        SET member_id = EXCLUDED.member_id
    RETURNING *
)
SELECT um.*, u.name, u.username, u.avatar_url
FROM inserted_member um
LEFT JOIN users u ON u.id = um.member_id;

-- name: ListMembers :many
SELECT m.member_id,
       u.name,
       u.username,
       u.avatar_url,
       u.emails
FROM unit_members m
JOIN users_with_emails u ON u.id = m.member_id
WHERE m.unit_id = $1;

-- name: ListUnitsMembers :many
SELECT m.unit_id,
       m.member_id,
       u.name,
       u.username,
       u.avatar_url
FROM unit_members m
JOIN users u ON u.id = m.member_id
WHERE m.unit_id = ANY($1::uuid[]);

-- name: RemoveMember :exec
DELETE FROM unit_members WHERE unit_id = $1 AND member_id = $2;
