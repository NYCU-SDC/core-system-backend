DROP TABLE IF EXISTS form_covers;

ALTER TABLE forms
  DROP COLUMN dressing_text_font,
  DROP COLUMN dressing_question_font,
  DROP COLUMN dressing_header_font,
  DROP COLUMN dressing_color,
  DROP COLUMN cover_image_url,
  DROP COLUMN publish_time,
  DROP COLUMN google_sheet_url,
  DROP COLUMN visibility,
  DROP COLUMN message_after_submission;

DROP TYPE IF EXISTS visibility;