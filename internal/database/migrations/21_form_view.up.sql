CREATE TABLE IF NOT EXISTS views (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id    UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    locked     BOOLEAN NOT NULL DEFAULT FALSE,
    "order"    INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_views_form_id ON views(form_id);
CREATE UNIQUE INDEX idx_views_form_id_title ON views(form_id, title);
