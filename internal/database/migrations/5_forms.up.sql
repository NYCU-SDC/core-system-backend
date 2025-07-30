CREATE TYPE question_type AS ENUM(
    'short_text',
    'long_text',
    'single_choice',
    'multiple_choice',
    'date'
);

CREATE TABLE IF NOT EXISTS forms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    unit_id UUID REFERENCES units(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS questions(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    required BOOLEAN NOT NULL,
    type question_type NOT NULL,
    title TEXT,
    description TEXT,
    metadata JSONB DEFAULT '{}'::JSONB,
    "order" INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
