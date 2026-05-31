DO $$
DECLARE
    id_column_added boolean;
    pk_moved_to_id boolean;
BEGIN
    SELECT col_description('public.auth'::regclass, (
        SELECT attnum
        FROM pg_attribute
        WHERE attrelid = 'public.auth'::regclass
          AND attname = 'id'
          AND NOT attisdropped
    )) = 'migration_20_add_auth_id'
    INTO id_column_added;

    SELECT obj_description('public.auth'::regclass, 'pg_class') = 'migration_20_add_auth_id:pk_on_id'
    INTO pk_moved_to_id;

    IF pk_moved_to_id THEN
        ALTER TABLE auth DROP CONSTRAINT IF EXISTS auth_pkey;
        ALTER TABLE auth ADD PRIMARY KEY (provider, provider_id);
        COMMENT ON TABLE auth IS NULL;
    END IF;

    ALTER TABLE auth ALTER COLUMN id DROP NOT NULL;

    IF id_column_added THEN
        ALTER TABLE auth DROP COLUMN id;
    END IF;
END $$;
