-- Rollback: restore enum to original values only

-- Create enum with only original values
CREATE TYPE question_type_old AS ENUM(
    'short_text',
    'long_text',
    'single_choice',
    'multiple_choice',
    'date'
);

-- Change columns to use the old enum
ALTER TABLE questions 
    ALTER COLUMN type TYPE question_type_old USING type::text::question_type_old;
ALTER TABLE answers 
    ALTER COLUMN type TYPE question_type_old USING type::text::question_type_old;

-- Drop the new enum
DROP TYPE IF EXISTS question_type;

-- Rename the old enum back
ALTER TYPE question_type_old RENAME TO question_type;
