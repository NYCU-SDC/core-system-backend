-- Rollback: remove 'archived' from forms.status enum

-- Recreate the original enum without 'archived'
CREATE TYPE status_old AS ENUM(
    'draft',
    'published'
);

-- Drop existing default before changing type back
ALTER TABLE forms
    ALTER COLUMN status DROP DEFAULT;

-- Migrate column back to the old enum type
ALTER TABLE forms 
    ALTER COLUMN status TYPE status_old USING status::text::status_old;

-- Drop the current enum and rename the old one back
DROP TYPE IF EXISTS status;
ALTER TYPE status_old RENAME TO status;

-- Restore default value on the original enum type
ALTER TABLE forms
    ALTER COLUMN status SET DEFAULT 'draft';
