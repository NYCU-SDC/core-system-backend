-- Best-effort rollback: restore plain-text description from HTML (tags stripped).

CREATE OR REPLACE FUNCTION _migration_16_description_html_to_text(html text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $f$
	SELECT replace(
		replace(
			replace(
				replace(
					replace(
						replace(
							regexp_replace(
								regexp_replace(
									regexp_replace(
										html,
										'(?i)<br\\s*/?>',
										E'\n',
										'g'
									),
									'(?i)</(p|li|div)>',
									E'\n',
									'g'
								),
								'<[^>]*>',
								'',
								'g'
							),
							'&nbsp;',
							' '
						),
						'&amp;',
						'&'
					),
					'&lt;',
					'<'
				),
				'&gt;',
				'>'
			),
			'&quot;',
			'"'
		),
		'&#39;',
		''''
	);
$f$;

ALTER TABLE forms ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE forms
SET description = NULLIF(_migration_16_description_html_to_text(description_html), '');
ALTER TABLE forms DROP COLUMN IF EXISTS description_json;
ALTER TABLE forms DROP COLUMN IF EXISTS description_html;

ALTER TABLE sections ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE sections
SET description = NULLIF(_migration_16_description_html_to_text(description_html), '');
ALTER TABLE sections DROP COLUMN IF EXISTS description_json;
ALTER TABLE sections DROP COLUMN IF EXISTS description_html;

ALTER TABLE questions ADD COLUMN IF NOT EXISTS description TEXT;
UPDATE questions
SET description = NULLIF(_migration_16_description_html_to_text(description_html), '');
ALTER TABLE questions DROP COLUMN IF EXISTS description_json;
ALTER TABLE questions DROP COLUMN IF EXISTS description_html;

DROP FUNCTION IF EXISTS _migration_16_description_html_to_text(text);
