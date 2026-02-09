CREATE TYPE question_type AS ENUM(
    'short_text',
    'long_text',
    'single_choice',
    'multiple_choice',
    'date',
    'dropdown',
    'detailed_multiple_choice',
    'upload_file',
    'linear_scale',
    'rating',
    'ranking',
    'oauth_connect',
    'hyperlink'
);

CREATE TABLE IF NOT EXISTS questions(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    section_id UUID NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
    required BOOLEAN NOT NULL,
    type question_type NOT NULL,
    title TEXT,
    description TEXT,
    metadata JSONB DEFAULT '{}'::JSONB,
    "order" INTEGER NOT NULL,
    source_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);