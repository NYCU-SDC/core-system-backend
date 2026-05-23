DROP VIEW IF EXISTS user_login_profile;

ALTER TABLE user_emails DROP CONSTRAINT IF EXISTS user_emails_pkey;

ALTER TABLE user_emails DROP COLUMN IF EXISTS id;
