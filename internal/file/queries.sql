-- name: Create :one
INSERT INTO files (original_filename, content_type, size, data, uploaded_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetByID :one
SELECT * FROM files WHERE id = $1;

-- name: GetMetadataByID :one
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files WHERE id = $1;

-- name: GetByUploadedBy :many
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files WHERE uploaded_by = $1 ORDER BY created_at DESC;

-- name: Delete :exec
DELETE FROM files WHERE id = $1;

-- name: ExistsByID :one
SELECT EXISTS(SELECT 1 FROM files WHERE id = $1);

-- name: GetAll :many
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: Count :one
SELECT COUNT(*) FROM files;
