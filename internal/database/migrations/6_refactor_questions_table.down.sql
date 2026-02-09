-- Drop section-related columns
ALTER TABLE questions
    DROP COLUMN IF EXISTS section_id;
ALTER TABLE questions
    DROP COLUMN IF EXISTS source_id;

-- Add form_id column back
ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE;
