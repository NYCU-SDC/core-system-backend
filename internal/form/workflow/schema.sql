-- Node type enum for workflow nodes
CREATE TYPE node_type AS ENUM(
    'section',
    'end',
    'start',
    'condition'
);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id),
    is_active BOOLEAN NOT NULL DEFAULT false,
    workflow JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workflow_versions_is_active ON workflow_versions(form_id, is_active) WHERE is_active = true;

CREATE INDEX idx_workflow_versions_latest ON workflow_versions(form_id, updated_at DESC);
