-- name: CreateOrg :one
INSERT INTO organizations (name, owner_id, description, metadata, slug)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateUnit :one
INSERT INTO units (name, org_id, description, metadata)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateDefaultUnit :one
INSERT INTO units (id, name, org_id, description, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUnitByID :one
SELECT * FROM units WHERE id = $1;

-- name: GetOrgByID :one
SELECT * FROM organizations WHERE id = $1;

-- name: GetAllOrganizations :many
SELECT * FROM organizations;

-- name: GetOrgIDBySlug :one
SELECT id FROM organizations WHERE slug = $1;

-- name: UpdateOrg :one
UPDATE organizations
SET slug = $2, name = $3, description = $4, metadata = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUnit :one
UPDATE units
SET name = $2, description = $3, metadata = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUnit :exec
DELETE FROM units WHERE id = $1;

-- name: DeleteOrg :exec
DELETE FROM organizations WHERE id = $1;

-- name: AddParentChild :one
INSERT INTO parent_child (parent_id, child_id, org_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListSubUnits :many
SELECT u.* FROM units u
JOIN parent_child pc ON u.id = pc.child_id
WHERE pc.parent_id = $1;

-- name: ListOrgSubUnits :many
SELECT u.* FROM units u
JOIN parent_child pc ON u.id = pc.child_id
WHERE pc.parent_id IS NULL AND pc.org_id = $1;

-- name: ListSubUnitIDs :many
SELECT child_id FROM parent_child WHERE parent_id = $1;

-- name: ListOrgSubUnitIDs :many
SELECT child_id FROM parent_child WHERE parent_id IS NULL AND org_id = $1;

-- name: RemoveParentChild :exec
DELETE FROM parent_child WHERE parent_id = $1 AND child_id = $2;

-- name: AddOrgMember :one
INSERT INTO org_members (org_id, member_id)
VALUES ($1, $2)
RETURNING *;

-- name: ListOrgMembers :many
SELECT member_id FROM org_members WHERE org_id = $1;

-- name: RemoveOrgMember :exec
DELETE FROM org_members WHERE org_id = $1 AND member_id = $2;

-- name: AddUnitMember :one
INSERT INTO unit_members (unit_id, member_id)
VALUES ($1, $2)
RETURNING *;

-- name: ListUnitMembers :many
SELECT member_id FROM unit_members WHERE unit_id = $1;