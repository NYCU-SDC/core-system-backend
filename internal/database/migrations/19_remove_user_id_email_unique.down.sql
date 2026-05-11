ALTER TABLE user_emails
ADD CONSTRAINT user_emails_user_id_value_key UNIQUE(user_id, value);
