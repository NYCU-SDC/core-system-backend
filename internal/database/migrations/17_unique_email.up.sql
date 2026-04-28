Alter TABLE user_emails
ADD CONSTRAINT user_emails_value_unique UNIQUE(value);