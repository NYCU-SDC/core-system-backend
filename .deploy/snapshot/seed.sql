-- Seed data for snapshot QA
-- Creates a stable QA user and links to the default SDC org.

DO $$
DECLARE
    user_id UUID := '4eb1b1e2-7bd5-4b08-b3b5-2d3e0459516e';
    org_id  UUID := 'cfc4e7f4-629f-420e-a79d-a58849cfd236';
BEGIN
    INSERT INTO users (id, name, username, avatar_url, role)
    VALUES (user_id, 'John Doe', 'qa_user', '', ARRAY['admin','user'])
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO user_emails (user_id, value)
    VALUES (user_id, 'qa@example.com')
    ON CONFLICT (user_id, value) DO NOTHING;

    INSERT INTO unit_members (unit_id, member_id, role)
    VALUES (org_id, user_id, 'admin, user')
    ON CONFLICT (unit_id, member_id) DO NOTHING;
END $$;
