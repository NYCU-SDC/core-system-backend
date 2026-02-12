-- Remove section progress tracking from sections.
-- The section_progress enum and sections.progress column are unused.

ALTER TABLE sections
    DROP COLUMN IF EXISTS progress;

DROP TYPE IF EXISTS section_progress;
