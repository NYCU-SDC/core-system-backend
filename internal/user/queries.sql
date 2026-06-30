-- name: Create :one
INSERT INTO users (name, username, avatar_url, role, is_onboarded)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateWithID :one
INSERT INTO users (id, name, username, avatar_url, role, is_onboarded)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWithEarliestProviderByEmail :one
SELECT u.id, u.name, a.provider, a.provider_id
FROM user_emails e
         JOIN users u ON e.user_id = u.id
         LEFT JOIN auth a ON a.user_id = u.id
WHERE e.value = @email
ORDER BY a.created_at ASC
    LIMIT 1;

-- name: Get :one
SELECT id, name, username, avatar_url, role, is_onboarded, created_at, updated_at, emails
FROM users_with_emails
WHERE id = @id;

-- name: Update :one
UPDATE users
SET name = @name, username = @username, avatar_url = @avatar_url, is_onboarded = @is_onboarded,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: CreateAuth :one
INSERT INTO auth (user_id, provider, provider_id)
VALUES (@user_id, @provider, @provider_id)
RETURNING *;

-- name: GetIDByAuth :one
SELECT user_id FROM auth WHERE provider = @provider AND provider_id = @provider_id;

-- name: UpsertEmail :exec
INSERT INTO user_emails (user_id, value)
VALUES (@user_id, @email)
ON CONFLICT (value) DO UPDATE SET updated_at = now();

-- name: GetIDByEmailForUpdate :one
SELECT user_id FROM user_emails WHERE value = @email FOR UPDATE;

-- name: GetEmails :many
SELECT user_emails.value as email FROM user_emails WHERE user_id = @user_id ORDER BY value;

-- name: GetIDByEmail :one
SELECT user_id FROM user_emails WHERE value = @email;
