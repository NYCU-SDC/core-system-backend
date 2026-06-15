DO $$
DECLARE
    id_column_existed boolean;
    id_in_primary_key boolean;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'auth'
          AND column_name = 'id'
    )
    INTO id_column_existed;

    IF NOT id_column_existed THEN
        ALTER TABLE auth ADD COLUMN id UUID DEFAULT gen_random_uuid();
        COMMENT ON COLUMN auth.id IS 'migration_20_add_auth_id';
    END IF;

    UPDATE auth SET id = gen_random_uuid() WHERE id IS NULL;

    ALTER TABLE auth ALTER COLUMN id SET NOT NULL;

    SELECT EXISTS (
        SELECT 1
        FROM pg_constraint c
                 JOIN pg_attribute a
                      ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
        WHERE c.conrelid = 'public.auth'::regclass
          AND c.contype = 'p'
          AND a.attname = 'id'
    )
    INTO id_in_primary_key;

    IF NOT id_in_primary_key THEN
        ALTER TABLE auth DROP CONSTRAINT IF EXISTS auth_pkey;
        ALTER TABLE auth ADD PRIMARY KEY (id);
        COMMENT ON TABLE auth IS 'migration_20_add_auth_id:pk_on_id';
    END IF;
END $$;
