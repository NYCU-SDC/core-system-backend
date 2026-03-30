DO $$
DECLARE
org_uuid UUID;
BEGIN

SELECT org_id
INTO org_uuid
FROM slug_history
WHERE slug = 'SDC'
ORDER BY id DESC
    LIMIT 1;

IF org_uuid IS NOT NULL THEN
DELETE FROM units WHERE id = org_uuid;
END IF;
END $$;
