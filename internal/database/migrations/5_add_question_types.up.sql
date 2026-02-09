-- This migration recreates the question_type enum with new values
-- This approach works within a transaction, unlike ALTER TYPE ... ADD VALUE

-- Create new enum type with all values (old + new)
CREATE TYPE question_type_new AS ENUM(
    'short_text',
    'long_text',
    'single_choice',
    'multiple_choice',
    'date',
    'dropdown',
    'detailed_multiple_choice',
    'linear_scale',
    'rating',
    'upload_file',
    'oauth_connect',
    'ranking',
    'hyperlink'
);

-- Update all columns to use the new type
ALTER TABLE questions 
    ALTER COLUMN type TYPE question_type_new USING type::text::question_type_new;
    
ALTER TABLE answers 
    ALTER COLUMN type TYPE question_type_new USING type::text::question_type_new;

-- Drop the old type and rename the new one
DROP TYPE question_type;
ALTER TYPE question_type_new RENAME TO question_type;
