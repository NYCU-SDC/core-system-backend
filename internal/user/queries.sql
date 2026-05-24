-- name: Create :one
INSERT INTO users (name, username, avatar_url, role, is_onboarded)
VALUES ($1, $2, $3, $4, $5) 
RETURNING *;

-- name: CreateWithID :one
INSERT INTO users (id, name, username, avatar_url, role, is_onboarded)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: Exists :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1);

-- name: GetWithEarliestProviderByEmail :one
SELECT u.id, u.name, a.provider, a.provider_id
FROM user_emails e
         JOIN users u ON e.user_id = u.id
         LEFT JOIN auth a ON a.user_email_id = e.id
WHERE e.value = $1
ORDER BY a.created_at ASC
    LIMIT 1;

-- name: Get :one
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
INSERT INTO auth (user_id, user_email_id, provider, provider_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetByAuth :one
SELECT user_id FROM auth WHERE provider = $1 AND provider_id = $2;

-- name: GetEmailIDByAuth :one
SELECT user_email_id FROM auth WHERE provider = $1 AND provider_id = $2;

-- name: ExistsByAuth :one
SELECT EXISTS(SELECT 1 FROM auth WHERE provider = $1 AND provider_id = $2);

-- name: UpsertEmail :one
INSERT INTO user_emails (user_id, value)
VALUES ($1, $2)
ON CONFLICT (value) DO UPDATE SET updated_at = now()
RETURNING id;

-- name: GetEmailForUpdate :one
SELECT id, user_id FROM user_emails WHERE value = $1 FOR UPDATE;

-- name: GetByEmailForUpdate :one
SELECT user_id FROM user_emails WHERE value = $1 FOR UPDATE;

-- name: GetEmails :many
SELECT user_emails.value as email FROM user_emails WHERE user_id = $1 ORDER BY value;

-- name: GetByEmail :one
SELECT user_id FROM user_emails WHERE value = $1;

-- name: GetLoginProfile :one
SELECT emails_and_auths FROM user_login_profile WHERE user_id = $1;
