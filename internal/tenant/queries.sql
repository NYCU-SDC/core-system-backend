-- name: ExistsBySlug :one
SELECT EXISTS(SELECT 1 FROM slug_history WHERE slug = $1 AND ended_at IS NULL);

-- name: Create :one
INSERT INTO tenants (id, db_strategy, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: Get :one
SELECT * FROM tenants WHERE id = $1;

-- name: Update :one
UPDATE tenants
SET db_strategy = $2
WHERE id = $1
RETURNING *;

-- name: Delete :exec
DELETE FROM tenants
WHERE id = $1;

-- name: GetSlugStatus :one
SELECT org_id
FROM slug_history
WHERE slug = $1
  AND ended_at IS NULL;

-- name: GetSlugHistory :many
SELECT s.*, u.name
FROM slug_history s
LEFT JOIN units u ON s.org_id = u.id
WHERE slug = $1
ORDER BY s.created_at DESC, s.id DESC;

-- name: CreateSlugHistory :one
INSERT INTO slug_history (slug, org_id)
VALUES ($1, $2)
RETURNING *;

-- name: UpsertSlugHistory :many
WITH
    -- Select the currently active slug for the org
    current_slug AS (
        SELECT slug
        FROM slug_history sh
        WHERE sh.org_id = $1 AND ended_at IS NULL
        FOR UPDATE
    ),

    -- End the old slug record if a new slug is assigned
    ended_history AS (
      UPDATE slug_history sh
      SET ended_at = now()
      WHERE org_id = $1
        AND ended_at IS NULL
        AND (SELECT sh.slug FROM current_slug) <> $2
      RETURNING org_id
    ),

    -- Insert the new slug record if an update occurred
    new_history AS (
      INSERT INTO slug_history (slug, org_id)
      SELECT
        $2, $1
      FROM ended_history
      RETURNING org_id
    )
SELECT * FROM new_history;