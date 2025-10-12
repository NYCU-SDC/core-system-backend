CREATE TYPE status AS ENUM(
    'draft',
    'published'
);

CREATE TABLE IF NOT EXISTS forms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    preview_message TEXT DEFAULT NULL,
    status status NOT NULL DEFAULT 'draft',
    unit_id UUID REFERENCES units(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deadline TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

