CREATE TYPE resource_type AS ENUM(
    'form_answer'
);

CREATE TABLE IF NOT EXISTS file_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    resource_type resource_type NOT NULL,
    resource_id UUID NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(file_id, resource_type, resource_id)
    );

CREATE INDEX IF NOT EXISTS idx_file_attachments_file_id ON file_attachments(file_id);
CREATE INDEX IF NOT EXISTS idx_file_attachments_resource ON file_attachments(resource_type, resource_id);