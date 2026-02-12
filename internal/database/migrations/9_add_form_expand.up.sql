CREATE TYPE visibility AS ENUM (
  'public',
  'private'
);

ALTER TABLE forms
  ADD COLUMN message_after_submission TEXT NOT NULL DEFAULT '',
  ADD COLUMN visibility visibility NOT NULL DEFAULT 'private',
  ADD COLUMN google_sheet_url TEXT,
  ADD COLUMN publish_time TIMESTAMPTZ,
  ADD COLUMN cover_image_url TEXT,
  ADD COLUMN dressing_color TEXT,
  ADD COLUMN dressing_header_font TEXT,
  ADD COLUMN dressing_question_font TEXT,
  ADD COLUMN dressing_text_font TEXT;

CREATE TABLE IF NOT EXISTS form_covers (
    form_id UUID PRIMARY KEY REFERENCES forms(id) ON DELETE CASCADE,
    image_data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);