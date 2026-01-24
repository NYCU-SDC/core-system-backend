CREATE TYPE status AS ENUM(
    'draft',
    'published'
);

CREATE TYPE visibility AS ENUM(
    'public',
    'private'
);

CREATE TABLE IF NOT EXISTS forms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    preview_message TEXT DEFAULT NULL,
    message_after_submission TEXT NOT NULL,
    status status NOT NULL DEFAULT 'draft',
    unit_id UUID REFERENCES units(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deadline TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    visibility visibility NOT NULL DEFAULT 'private',
    google_sheet_url TEXT,
    publish_time TIMESTAMPTZ,
    cover_image_url TEXT,
    dressing_color TEXT,
    dressing_header_font TEXT,
    dressing_question_font TEXT,
    dressing_text_font TEXT
);

