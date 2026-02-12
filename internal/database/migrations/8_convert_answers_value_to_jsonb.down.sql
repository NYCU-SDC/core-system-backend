-- Rollback: Convert answers.value from JSONB back to TEXT

-- Drop unique constraint
ALTER TABLE answers DROP CONSTRAINT IF EXISTS answers_response_question_unique;

-- Create a temporary function to convert JSONB back to TEXT
CREATE OR REPLACE FUNCTION convert_answer_jsonb_to_text(jsonb_value JSONB, question_type question_type)
RETURNS TEXT AS $$
BEGIN
    -- For multiple choice questions, convert JSON array back to semicolon-separated string
    IF question_type IN ('multiple_choice', 'detailed_multiple_choice') THEN
        IF jsonb_value = '[]'::jsonb OR jsonb_value IS NULL THEN
            RETURN '';
        ELSIF jsonb_typeof(jsonb_value) = 'array' THEN
            -- Join array elements with semicolon
            RETURN (
                SELECT string_agg(elem::text, ';')
                FROM jsonb_array_elements_text(jsonb_value) AS elem
            );
        ELSE
            RETURN jsonb_value::text;
        END IF;
    ELSE
        -- For single value questions, extract the string value
        IF jsonb_value IS NULL OR jsonb_value = 'null'::jsonb THEN
            RETURN NULL;
        ELSIF jsonb_typeof(jsonb_value) = 'string' THEN
            RETURN jsonb_value #>> '{}';
        ELSE
            RETURN jsonb_value::text;
        END IF;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Add new TEXT column
ALTER TABLE answers ADD COLUMN value_text TEXT;

-- Migrate data back
UPDATE answers 
SET value_text = convert_answer_jsonb_to_text(value, type);

-- Make the new column NOT NULL
ALTER TABLE answers ALTER COLUMN value_text SET NOT NULL;

-- Drop old JSONB column
ALTER TABLE answers DROP COLUMN value;

-- Rename new column to original name
ALTER TABLE answers RENAME COLUMN value_text TO value;

-- Drop the temporary function
DROP FUNCTION convert_answer_jsonb_to_text(JSONB, question_type);

DROP TYPE response_progress;
ALTER TABLE form_responses DROP COLUMN submitted_at;