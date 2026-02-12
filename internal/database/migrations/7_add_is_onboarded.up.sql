ALTER TABLE users 
    ADD COLUMN is_onboarded BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT users_username_unique UNIQUE (username);

DROP VIEW IF EXISTS users_with_emails;

CREATE OR REPLACE VIEW users_with_emails AS
SELECT 
    u.id,
    u.name,
    u.username,
    u.avatar_url,
    u.role,
    u.is_onboarded,
    u.created_at,
    u.updated_at,
    COALESCE(array_agg(e.value) FILTER (WHERE e.value IS NOT NULL), ARRAY[]::text[]) as emails
FROM users u
LEFT JOIN user_emails e ON u.id = e.user_id
GROUP BY u.id, u.name, u.username, u.avatar_url, u.role, u.is_onboarded, u.created_at, u.updated_at;