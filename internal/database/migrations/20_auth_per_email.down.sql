DROP VIEW IF EXISTS user_login_profile;

ALTER TABLE auth DROP COLUMN IF EXISTS user_email_id;

ALTER TABLE user_emails DROP CONSTRAINT IF EXISTS user_emails_pkey;

ALTER TABLE user_emails DROP COLUMN IF EXISTS id;
