ALTER TABLE user_emails ADD COLUMN IF NOT EXISTS id UUID DEFAULT gen_random_uuid();

UPDATE user_emails SET id = gen_random_uuid() WHERE id IS NULL;

ALTER TABLE user_emails ALTER COLUMN id SET NOT NULL;

ALTER TABLE user_emails ADD PRIMARY KEY (id);

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
                            WHERE a.user_id = u.id
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
