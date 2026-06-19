CREATE TYPE status_new AS ENUM (
    'draft',
    'published',
    'archived'
);

ALTER TABLE forms
ALTER COLUMN status TYPE status_new
USING (
    CASE
        WHEN status::text = 'close' THEN 'archived'
        ELSE status::text
    END
)::status_new;

DROP TYPE status;

ALTER TYPE status_new RENAME TO status;