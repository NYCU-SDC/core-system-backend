-- Add 'archived' status to forms.status enum in a transactional-safe way

-- Create new enum type including the new value
CREATE TYPE new_status AS ENUM(
    'draft',
    'published',
    'archived'
);

-- Drop existing default to avoid cast issues during type change
ALTER TABLE forms
    ALTER COLUMN status DROP DEFAULT;

-- Migrate existing column to use the new enum
ALTER TABLE forms 
    ALTER COLUMN status TYPE new_status USING status::text::new_status;

-- Drop the old enum type and rename the new one
DROP TYPE status;
ALTER TYPE new_status RENAME TO status;

-- Restore the default value on the updated enum type
ALTER TABLE forms
    ALTER COLUMN status SET DEFAULT 'draft';