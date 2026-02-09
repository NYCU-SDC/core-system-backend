ALTER TABLE users 
    DROP CONSTRAINT IF EXISTS users_username_unique,
    DROP COLUMN IF EXISTS is_onboarded;

DROP VIEW IF EXISTS users_with_emails;

CREATE VIEW users_with_emails AS
SELECT 
    u.id,
    u.name,
    u.username,
    u.avatar_url,
    u.role,
    u.created_at,
    u.updated_at,
    COALESCE(array_agg(e.value) FILTER (WHERE e.value IS NOT NULL), ARRAY[]::text[]) as emails
FROM users u
LEFT JOIN user_emails e ON u.id = e.user_id
GROUP BY u.id, u.name, u.username, u.avatar_url, u.role, u.created_at, u.updated_at;