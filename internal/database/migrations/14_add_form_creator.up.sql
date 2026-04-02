ALTER TABLE forms
    ADD COLUMN created_by UUID;

UPDATE forms
SET created_by = last_editor
WHERE created_by IS NULL;

ALTER TABLE forms
    ALTER COLUMN created_by SET NOT NULL;

ALTER TABLE forms
    ADD CONSTRAINT forms_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id);