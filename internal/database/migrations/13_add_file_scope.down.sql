-- Remove scope and resource tracking columns from files table
DROP INDEX IF EXISTS idx_files_resource;
DROP INDEX IF EXISTS idx_files_scope;

ALTER TABLE files DROP COLUMN IF EXISTS resource_id;
ALTER TABLE files DROP COLUMN IF EXISTS resource_type;
ALTER TABLE files DROP COLUMN IF EXISTS scope;

-- Revert uploaded_by to allow NULL
ALTER TABLE files ALTER COLUMN uploaded_by DROP NOT NULL;
