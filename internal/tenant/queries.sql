-- name: Create :one
INSERT INTO tenants (id, slug, db_strategy, owner_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: Get :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: Update :one
UPDATE tenants
SET db_strategy = $2
WHERE id = $1
RETURNING *;

-- name: Delete :exec
DELETE FROM tenants
WHERE id = $1;