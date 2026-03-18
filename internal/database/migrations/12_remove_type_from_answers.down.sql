ALTER TABLE answers ADD COLUMN type question_type NOT NULL DEFAULT 'short_text'::question_type;
