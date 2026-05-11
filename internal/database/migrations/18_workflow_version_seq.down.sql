DROP INDEX IF EXISTS idx_workflow_versions_seq;

ALTER TABLE workflow_versions
    DROP COLUMN IF EXISTS seq;

