-- Seed data for snapshot QA
-- Creates a stable QA user and links to the default SDC org.

DO $$
DECLARE
    v_slug TEXT := 'SDC';
    v_user_id UUID := '4eb1b1e2-7bd5-4b08-b3b5-2d3e0459516e';
    v_org_id  UUID;
BEGIN
    SELECT org_id
    INTO v_org_id
    FROM slug_history
    WHERE slug = v_slug
    ORDER BY created_at DESC, id DESC;
    LIMIT 1;

    IF v_org_id IS NULL THEN
        RAISE EXCEPTION '% organization not found in slug_history', v_slug;
    END IF;

    INSERT INTO users (id, name, username, avatar_url, role)
    VALUES (v_user_id, 'John Doe', 'qa_user', '', ARRAY['admin','user'])
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO user_emails (user_id, value)
    VALUES (v_user_id, 'qa@example.com')
    ON CONFLICT (user_id, value) DO NOTHING;

    INSERT INTO unit_members (unit_id, member_id, role)
    VALUES (v_org_id, v_user_id, 'admin')
    ON CONFLICT (unit_id, member_id) DO NOTHING;
END $$;
