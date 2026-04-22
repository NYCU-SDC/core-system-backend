-- name: Create :one
INSERT INTO files (original_filename, content_type, size, data, uploaded_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: Get :one
SELECT * FROM files WHERE id = $1;

-- name: GetMetadata :one
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files WHERE id = $1;

-- name: GetByUploadedBy :many
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files WHERE uploaded_by = $1 ORDER BY created_at DESC;

-- name: Delete :exec
DELETE FROM files WHERE id = $1;

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM files WHERE id = $1);

-- name: GetAll :many
SELECT id, original_filename, content_type, size, uploaded_by, created_at, updated_at 
FROM files ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: Count :one
SELECT COUNT(*) FROM files;

-- name: CreateAttachment :one
INSERT INTO file_attachments (file_id, resource_type, resource_id, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAttachmentsByFileID :many
SELECT * FROM file_attachments WHERE file_id = $1 ORDER BY created_at ASC;

-- name: ListAttachmentsByResource :many
SELECT * FROM file_attachments
WHERE resource_type = $1 AND resource_id = $2 ORDER BY created_at ASC;

-- name: DeleteAttachment :exec
DELETE FROM file_attachments WHERE id = $1;

-- name: GetAttachment :one
SELECT * FROM file_attachments WHERE id = $1;

-- name: ExistsAttachmentByFileAndResource :one
SELECT EXISTS(
    SELECT 1 FROM file_attachments 
    WHERE file_id = $1 AND resource_type = $2 AND resource_id = $3
);

-- name: GetAttachmentByFileAndResource :one
SELECT *
FROM file_attachments
WHERE file_id = $1
  AND resource_type = $2
  AND resource_id = $3;

-- name: DeleteAttachmentsByFileID :exec
DELETE FROM file_attachments
WHERE file_id = $1;

-- name: LockFile :one
SELECT id
FROM files
WHERE id = $1
FOR UPDATE;