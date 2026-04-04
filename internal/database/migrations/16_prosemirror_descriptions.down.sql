-- Best-effort rollback: restore plain-text description from HTML (tags stripped).

ALTER TABLE forms ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE forms SET description = regexp_replace(regexp_replace(description_html, '<[^>]*>', '', 'g'), '&[^;]+;', '', 'g');
ALTER TABLE forms DROP COLUMN IF EXISTS description_json;
ALTER TABLE forms DROP COLUMN IF EXISTS description_html;

ALTER TABLE sections ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE sections SET description = NULLIF(regexp_replace(regexp_replace(description_html, '<[^>]*>', '', 'g'), '&[^;]+;', '', 'g'), '');
ALTER TABLE sections DROP COLUMN IF EXISTS description_json;
ALTER TABLE sections DROP COLUMN IF EXISTS description_html;

ALTER TABLE questions ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE questions SET description = regexp_replace(regexp_replace(description_html, '<[^>]*>', '', 'g'), '&[^;]+;', '', 'g');
ALTER TABLE questions DROP COLUMN IF EXISTS description_json;
ALTER TABLE questions DROP COLUMN IF EXISTS description_html;
