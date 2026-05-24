CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255),
    username VARCHAR(255) UNIQUE,
    avatar_url VARCHAR(512),
    role VARCHAR(255)[] NOT NULL DEFAULT '{"user"}',
    is_onboarded BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_email_id UUID REFERENCES user_emails(id) ON DELETE CASCADE,
    provider VARCHAR(255) NOT NULL,
    provider_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_id)
);

CREATE TABLE IF NOT EXISTS user_emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(value)
);

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

CREATE OR REPLACE VIEW user_login_profile AS
SELECT
    u.id AS user_id,
    COALESCE(
        (
            SELECT json_agg(
                json_build_object(
                    'email', e.value,
                    'authProviders', COALESCE(
                        (
                            SELECT json_agg(a.provider ORDER BY a.provider)
                            FROM auth AS a
                            WHERE a.user_email_id = e.id
                        ),
                        '[]'::json
                    )
                )
                ORDER BY e.value
            )
            FROM user_emails AS e
            WHERE e.user_id = u.id
        ),
        '[]'::json
    )::jsonb AS emails_and_auths
FROM users AS u;
