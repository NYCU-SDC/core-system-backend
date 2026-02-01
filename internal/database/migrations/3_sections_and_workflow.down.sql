-- Drop workflow_versions table and its indexes
DROP TABLE IF EXISTS workflow_versions CASCADE;

-- Drop sections table and its indexes
DROP TABLE IF EXISTS sections CASCADE;

-- Drop ENUMs
DROP TYPE IF EXISTS section_progress;
DROP TYPE IF EXISTS node_type;
