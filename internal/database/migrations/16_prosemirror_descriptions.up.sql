ALTER TABLE forms
    ADD COLUMN IF NOT EXISTS description_json JSONB,
    ADD COLUMN IF NOT EXISTS description_html TEXT;

UPDATE forms
SET description_json = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN '{"type":"doc","content":[]}'::jsonb
        ELSE jsonb_build_object(
            'type', 'doc',
            'content', jsonb_build_array(
                jsonb_build_object(
                    'type', 'paragraph',
                    'content', jsonb_build_array(
                        jsonb_build_object('type', 'text', 'text', description)
                    )
                )
            )
        )
    END,
    description_html = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN ''
        ELSE
            '<p>'
                || replace(replace(replace(replace(btrim(description), '&', '&amp;'), '<', '&lt;'), '>', '&gt;'), '"', '&quot;')
                || '</p>'
    END;

ALTER TABLE forms DROP COLUMN IF EXISTS description;

ALTER TABLE forms
    ALTER COLUMN description_json SET NOT NULL,
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[]}'::jsonb,
    ALTER COLUMN description_html SET NOT NULL,
    ALTER COLUMN description_html SET DEFAULT '';

ALTER TABLE sections
    ADD COLUMN IF NOT EXISTS description_json JSONB,
    ADD COLUMN IF NOT EXISTS description_html TEXT;

UPDATE sections
SET description_json = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN '{"type":"doc","content":[]}'::jsonb
        ELSE jsonb_build_object(
            'type', 'doc',
            'content', jsonb_build_array(
                jsonb_build_object(
                    'type', 'paragraph',
                    'content', jsonb_build_array(
                        jsonb_build_object('type', 'text', 'text', description)
                    )
                )
            )
        )
    END,
    description_html = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN ''
        ELSE
            '<p>'
                || replace(replace(replace(replace(btrim(description), '&', '&amp;'), '<', '&lt;'), '>', '&gt;'), '"', '&quot;')
                || '</p>'
    END;

ALTER TABLE sections DROP COLUMN IF EXISTS description;

ALTER TABLE sections
    ALTER COLUMN description_json SET NOT NULL,
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[]}'::jsonb,
    ALTER COLUMN description_html SET NOT NULL,
    ALTER COLUMN description_html SET DEFAULT '';

ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS description_json JSONB,
    ADD COLUMN IF NOT EXISTS description_html TEXT;

UPDATE questions
SET description_json = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN '{"type":"doc","content":[]}'::jsonb
        ELSE jsonb_build_object(
            'type', 'doc',
            'content', jsonb_build_array(
                jsonb_build_object(
                    'type', 'paragraph',
                    'content', jsonb_build_array(
                        jsonb_build_object('type', 'text', 'text', description)
                    )
                )
            )
        )
    END,
    description_html = CASE
        WHEN description IS NULL OR btrim(description) = '' THEN ''
        ELSE
            '<p>'
                || replace(replace(replace(replace(btrim(description), '&', '&amp;'), '<', '&lt;'), '>', '&gt;'), '"', '&quot;')
                || '</p>'
    END;

ALTER TABLE questions DROP COLUMN IF EXISTS description;

ALTER TABLE questions
    ALTER COLUMN description_json SET NOT NULL,
    ALTER COLUMN description_json SET DEFAULT '{"type":"doc","content":[]}'::jsonb,
    ALTER COLUMN description_html SET NOT NULL,
    ALTER COLUMN description_html SET DEFAULT '';
