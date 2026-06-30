CREATE TYPE status_new AS ENUM (
    'draft',
    'published',
    'archived'
);

ALTER TABLE forms
ALTER COLUMN status DROP DEFAULT;

ALTER TABLE forms
ALTER COLUMN status TYPE status_new
USING (
    CASE
        WHEN status::text = 'closed' THEN 'archived'
        ELSE status::text
    END
)::status_new;

DROP TYPE status;

ALTER TYPE status_new RENAME TO status;

ALTER TABLE forms
ALTER COLUMN status SET DEFAULT 'draft';