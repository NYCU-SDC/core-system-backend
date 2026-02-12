-- Convert answers.value from TEXT to JSONB for better data structure support
-- This migration handles the conversion of existing semicolon-separated values to JSON arrays

-- First, create a temporary function to convert existing TEXT values to JSONB
CREATE OR REPLACE FUNCTION convert_answer_value_to_jsonb(text_value TEXT, question_type question_type)
RETURNS JSONB AS $$
BEGIN
    -- For multiple choice questions, convert semicolon-separated UUIDs to JSON array
    IF question_type IN ('multiple_choice', 'detailed_multiple_choice') THEN
        IF text_value = '' OR text_value IS NULL THEN
            RETURN '[]'::jsonb;
        ELSE
            -- Split by semicolon and create JSON array
            RETURN (
                SELECT jsonb_agg(trim(elem))
                FROM unnest(string_to_array(text_value, ';')) AS elem
                WHERE trim(elem) != ''
            );
        END IF;
    ELSE
        -- For single value questions (single_choice, text, etc.), wrap in JSON string
        IF text_value IS NULL THEN
            RETURN 'null'::jsonb;
        ELSE
            RETURN to_jsonb(text_value);
        END IF;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Add new JSONB column
ALTER TABLE answers ADD COLUMN value_jsonb JSONB;

-- Migrate existing data
UPDATE answers 
SET value_jsonb = convert_answer_value_to_jsonb(value, type);

-- Make the new column NOT NULL
ALTER TABLE answers ALTER COLUMN value_jsonb SET NOT NULL;

-- Drop old TEXT column
ALTER TABLE answers DROP COLUMN value;

-- Rename new column to original name
ALTER TABLE answers RENAME COLUMN value_jsonb TO value;

-- Drop the temporary function
DROP FUNCTION convert_answer_value_to_jsonb(TEXT, question_type);

-- Add unique constraint to ensure one answer per question per response
ALTER TABLE answers ADD CONSTRAINT answers_response_question_unique UNIQUE (response_id, question_id);


CREATE TYPE response_progress AS ENUM(
    'draft',
    'submitted'
    );
ALTER TABLE form_responses ADD COLUMN IF NOT EXISTS progress response_progress NOT NULL DEFAULT 'draft';