-- Drop old form_id column and add section_id column
ALTER TABLE questions
    DROP COLUMN IF EXISTS form_id;
    
ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS section_id UUID NOT NULL REFERENCES sections(id) ON DELETE CASCADE;

-- Add source_id column (nullable)
ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS source_id UUID;
