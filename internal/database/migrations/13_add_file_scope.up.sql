-- Add scope and resource tracking columns to files table
ALTER TABLE files ADD COLUMN scope VARCHAR(50) NOT NULL DEFAULT 'private';
ALTER TABLE files ADD COLUMN resource_type VARCHAR(50) NOT NULL DEFAULT 'unknown';
ALTER TABLE files ADD COLUMN resource_id UUID NOT NULL;

-- Create indexes for efficient querying
CREATE INDEX idx_files_scope ON files(scope);
CREATE INDEX idx_files_resource ON files(resource_type, resource_id);

-- Make uploaded_by NOT NULL for better data integrity
-- (Use a default system user UUID for existing NULL records if any)
UPDATE files SET uploaded_by = '00000000-0000-0000-0000-000000000000'::UUID WHERE uploaded_by IS NULL;
ALTER TABLE files ALTER COLUMN uploaded_by SET NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN files.scope IS 'File visibility scope: public, organization, private';
COMMENT ON COLUMN files.resource_type IS 'Type of resource this file belongs to: user_avatar, form_cover, form_response, etc';
COMMENT ON COLUMN files.resource_id IS 'ID of the resource this file belongs to';
