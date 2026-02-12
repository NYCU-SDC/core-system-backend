-- Re-introduce section progress tracking on rollback.

CREATE TYPE section_progress AS ENUM(
    'draft',
    'submitted'
);

ALTER TABLE sections
    ADD COLUMN IF NOT EXISTS progress section_progress NOT NULL DEFAULT 'draft';
