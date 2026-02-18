-- Create ENUMs for sections and workflow nodes

-- Node type enum for workflow nodes
CREATE TYPE node_type AS ENUM(
    'section',
    'end',
    'start',
    'condition'
);

-- Create sections table
CREATE TABLE IF NOT EXISTS sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for querying sections by form_id
CREATE INDEX idx_sections_form_id ON sections(form_id);

-- Create workflow_versions table
CREATE TABLE IF NOT EXISTS workflow_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id),
    is_active BOOLEAN NOT NULL DEFAULT false,
    workflow JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for querying active workflows by form_id
CREATE INDEX idx_workflow_versions_is_active ON workflow_versions(form_id, is_active) WHERE is_active = true;

-- Index for querying latest workflow by form_id (sorted by updated_at DESC)
CREATE INDEX idx_workflow_versions_latest ON workflow_versions(form_id, updated_at DESC);
