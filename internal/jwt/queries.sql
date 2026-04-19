-- name: GetUserIDByTokenID :one
SELECT user_id FROM refresh_tokens WHERE id = $1 AND is_active = TRUE AND expiration_date > NOW();

-- name: Create :one
INSERT INTO refresh_tokens (user_id, expiration_date) VALUES ($1, $2) RETURNING *;

-- name: Inactivate :execrows
UPDATE refresh_tokens SET is_active = FALSE WHERE id = $1 RETURNING *;

-- name: Delete :execrows
DELETE FROM refresh_tokens WHERE expiration_date < NOW() OR is_active = FALSE;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE id = $1;