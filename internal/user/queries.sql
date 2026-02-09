-- name: Create :one
INSERT INTO users (name, username, avatar_url, role, is_onboarded)
VALUES ($1, $2, $3, $4, $5) 
RETURNING *;

-- name: ExistsByID :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1);

-- name: GetByID :one
SELECT id, name, username, avatar_url, role, is_onboarded, created_at, updated_at, emails
FROM users_with_emails
WHERE id = $1;

-- name: Update :one
UPDATE users
SET name = $2, username = $3, avatar_url = $4, is_onboarded = $5,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateAuth :one
INSERT INTO auth (user_id, provider, provider_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetIDByAuth :one
SELECT user_id FROM auth WHERE provider = $1 AND provider_id = $2;

-- name: ExistsByAuth :one
SELECT EXISTS(SELECT 1 FROM auth WHERE provider = $1 AND provider_id = $2);

-- name: CreateEmail :exec
INSERT INTO user_emails (user_id, value)
VALUES ($1, $2) 
ON CONFLICT (user_id, value) DO NOTHING;

-- name: GetEmailsByID :many
SELECT user_emails.value as email FROM user_emails WHERE user_id = $1;