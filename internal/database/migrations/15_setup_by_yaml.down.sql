DO $$
DECLARE
org_uuid UUID := 'cfc4e7f4-629f-420e-a79d-a58849cfd236';
BEGIN

INSERT INTO units (id, org_id, parent_id, type, name, description, metadata)
VALUES (
           org_uuid,
           NULL,
           NULL,
           'organization',
           'Default Organization',
           'System default organization',
           '{}'::jsonb
       );

INSERT INTO tenants (id, db_strategy, owner_id)
VALUES (
           org_uuid,
           'shared',
           NULL
       );

INSERT INTO slug_history (slug, org_id)
VALUES ('SDC', org_uuid);
END $$;
