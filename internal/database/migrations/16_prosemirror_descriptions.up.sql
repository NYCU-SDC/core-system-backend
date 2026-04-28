CREATE OR REPLACE FUNCTION _migration_16_text_to_doc_json(t text)
RETURNS jsonb
LANGUAGE sql
IMMUTABLE
AS $f$
	SELECT jsonb_build_object(
		'type', 'doc',
		'content', jsonb_build_array(
			jsonb_build_object(
				'type', 'paragraph',
				'content', jsonb_build_array(
					jsonb_build_object('type', 'text', 'text', t)
				)
			)
		)
	);
$f$;

CREATE OR REPLACE FUNCTION _migration_16_escape_html(t text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $f$
	SELECT replace(
		replace(
			replace(
				replace(
					replace(btrim(t), '&', '&amp;'),
					'<',
					'&lt;'
				),
				'>',
				'&gt;'
			),
			'"',
			'&quot;'
		),
		'''',
		'&#39;'
	);
$f$;

ALTER TABLE forms
    ADD COLUMN IF NOT EXISTS description_json JSONB NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb,
    ADD COLUMN IF NOT EXISTS description_html TEXT NOT NULL DEFAULT '';

UPDATE forms
SET
    description_json = _migration_16_text_to_doc_json(description),
    description_html =
        '<p>'
            || _migration_16_escape_html(description)
            || '</p>'
WHERE description IS NOT NULL AND btrim(description) <> '';

ALTER TABLE forms DROP COLUMN IF EXISTS description;

ALTER TABLE sections
    ADD COLUMN IF NOT EXISTS description_json JSONB NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb,
    ADD COLUMN IF NOT EXISTS description_html TEXT NOT NULL DEFAULT '';

UPDATE sections
SET
    description_json = _migration_16_text_to_doc_json(description),
    description_html =
        '<p>'
            || _migration_16_escape_html(description)
            || '</p>'
WHERE description IS NOT NULL AND btrim(description) <> '';

ALTER TABLE sections DROP COLUMN IF EXISTS description;

ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS description_json JSONB NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb,
    ADD COLUMN IF NOT EXISTS description_html TEXT NOT NULL DEFAULT '';

UPDATE questions
SET
    description_json = _migration_16_text_to_doc_json(description),
    description_html =
        '<p>'
            || _migration_16_escape_html(description)
            || '</p>'
WHERE description IS NOT NULL AND btrim(description) <> '';

ALTER TABLE questions DROP COLUMN IF EXISTS description;

-- Align any legacy empty-doc shape (content:[]) with doc "block+" / markdown.EmptyDocumentJSON.
UPDATE forms
SET description_json = '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb
WHERE description_json = '{"type":"doc","content":[]}'::jsonb;

UPDATE sections
SET description_json = '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb
WHERE description_json = '{"type":"doc","content":[]}'::jsonb;

UPDATE questions
SET description_json = '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb
WHERE description_json = '{"type":"doc","content":[]}'::jsonb;

ALTER TABLE forms
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb;

ALTER TABLE sections
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb;

ALTER TABLE questions
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}'::jsonb;

DROP FUNCTION IF EXISTS _migration_16_text_to_doc_json(text);
DROP FUNCTION IF EXISTS _migration_16_escape_html(text);
