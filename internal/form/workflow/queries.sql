-- name: Get :one
SELECT workflow, id, form_id, last_editor, is_active, created_at, updated_at
FROM workflow_versions
WHERE form_id = $1
ORDER BY updated_at DESC
LIMIT 1;

-- name: Update :one
WITH latest_workflow AS (
    SELECT wv.id, wv.is_active, wv.form_id
    FROM workflow_versions AS wv
    WHERE wv.form_id = $1
    ORDER BY wv.updated_at DESC
    LIMIT 1
    FOR UPDATE
),
updated AS (
    UPDATE workflow_versions AS wv
    SET workflow = $3, last_editor = $2, updated_at = now()
    FROM latest_workflow AS lw
    WHERE wv.id = lw.id 
      AND lw.is_active = false
    RETURNING wv.workflow, wv.id, wv.form_id, wv.last_editor, wv.is_active, wv.created_at, wv.updated_at
),
created AS (
    INSERT INTO workflow_versions (form_id, last_editor, workflow)
    SELECT $1, $2, $3
    FROM latest_workflow AS lw
    WHERE lw.is_active = true
    RETURNING workflow, id, form_id, last_editor, is_active, created_at, updated_at
)
SELECT * FROM updated
UNION ALL
SELECT * FROM created;

-- name: CreateNode :one
WITH latest_workflow AS (
    SELECT wv.id, wv.is_active, wv.form_id, wv.workflow
    FROM workflow_versions AS wv
    WHERE wv.form_id = $1
    ORDER BY wv.updated_at DESC
    LIMIT 1
    FOR UPDATE
),
new_section AS (
    INSERT INTO sections (form_id, title, progress)
    SELECT lw.form_id, 'New Section', 'draft'
    FROM latest_workflow AS lw
    WHERE @type::node_type = 'section'
    RETURNING id
),
node_id AS (
    SELECT COALESCE((SELECT id FROM new_section), gen_random_uuid()) AS id
),
new_node AS (
    SELECT jsonb_build_object(
        'id', node_id.id,
        'label', 'New ' || initcap(@type::text),
        'type', @type::node_type
    ) AS node
    FROM node_id
),
updated AS (
    UPDATE workflow_versions AS wv
    SET workflow = lw.workflow || jsonb_build_array(new_node.node),
        last_editor = $2,
        updated_at = now()
    FROM latest_workflow AS lw, new_node
    WHERE wv.id = lw.id 
      AND lw.is_active = false
    RETURNING wv.id, wv.form_id, wv.last_editor, wv.is_active, wv.workflow, wv.created_at, wv.updated_at
),
created AS (
    INSERT INTO workflow_versions (form_id, last_editor, workflow)
    SELECT $1, $2, lw.workflow || jsonb_build_array(new_node.node)
    FROM latest_workflow AS lw, new_node
    WHERE lw.is_active = true
    RETURNING id, form_id, last_editor, is_active, workflow, created_at, updated_at
)
SELECT 
    (SELECT node->>'id' FROM new_node)::uuid AS node_id,
    (SELECT node->>'type' FROM new_node)::node_type AS node_type,
    (SELECT node->>'label' FROM new_node) AS node_label,
    u.workflow AS workflow
FROM updated AS u, new_node
UNION ALL
SELECT 
    (SELECT node->>'id' FROM new_node)::uuid AS node_id,
    (SELECT node->>'type' FROM new_node)::node_type AS node_type,
    (SELECT node->>'label' FROM new_node) AS node_label,
    c.workflow AS workflow
FROM created AS c, new_node;

-- name: DeleteNode :one
-- Deletes a node from the workflow and nullifies all references to it in other nodes
WITH latest_workflow AS (
    SELECT wv.id, wv.is_active, wv.form_id, wv.workflow
    FROM workflow_versions AS wv
    WHERE wv.form_id = $1
    ORDER BY wv.updated_at DESC
    LIMIT 1
    FOR UPDATE
),
-- Convert deleted node ID to text format for JSONB comparison
deleted_node_id AS (
    SELECT @node_id::text AS deleted_id
),
-- Extract information about the node to be deleted
node_to_delete AS (
    SELECT 
        node->>'id' AS node_id,
        node->>'type' AS node_type
    FROM latest_workflow AS lw,
    LATERAL jsonb_array_elements(COALESCE(lw.workflow, '[]'::jsonb)) AS node
    WHERE node->>'id' = (SELECT deleted_id FROM deleted_node_id)
    LIMIT 1
),
-- Delete associated section if the node is a section type
deleted_section AS (
    DELETE FROM sections
    WHERE id = (SELECT node_id::uuid FROM node_to_delete)
      AND EXISTS (SELECT 1 FROM node_to_delete WHERE node_type = 'section')
    RETURNING id
),
-- Get all nodes except the one being deleted
remaining_nodes AS (
    SELECT node
    FROM latest_workflow AS lw,
    LATERAL jsonb_array_elements(COALESCE(lw.workflow, '[]'::jsonb)) AS node
    WHERE node->>'id' != (SELECT deleted_id FROM deleted_node_id)
),
-- Expand each node into individual key-value pairs
/* 
Example of node_fields_expanded: 
node_id   | field_key  | cleaned_value
----------|------------|------------
"node-a"  | "id"       | "node-a"
"node-a"  | "type"     | "start"
"node-a"  | "label"    | "Start"
"node-a"  | "next"     | null  ← NULLIFIED!
"node-c"  | "id"       | "node-c"
"node-c"  | "type"     | "condition"
"node-c"  | "label"    | "Check"
"node-c"  | "nextTrue" | null  ← NULLIFIED!
"node-c"  | "nextFalse"| "node-d"
"node-d"  | "id"       | "node-d"
"node-d"  | "type"     | "end"
"node-d"  | "label"    | "End"
*/
node_fields_expanded AS (
    SELECT 
        node->>'id' AS node_id,
        field_key,
        field_value
    FROM remaining_nodes,
    LATERAL jsonb_each(COALESCE(node, '{}'::jsonb)) AS node_fields(field_key, field_value)
),
-- Clean each field: remove reference fields that point to the deleted node (omit them entirely)
cleaned_node_fields AS (
    SELECT 
        node_id,
        field_key,
        field_value
    FROM node_fields_expanded
    WHERE NOT (
        -- Remove (omit) reference fields that point to the deleted node
        field_key IN ('next', 'nextTrue', 'nextFalse') 
        AND jsonb_typeof(field_value) = 'string'
        AND trim(both '"' from field_value::text) = (SELECT deleted_id FROM deleted_node_id)
    )
),
-- Rebuild nodes from cleaned fields
cleaned_nodes AS (
    SELECT 
        jsonb_object_agg(field_key, field_value ORDER BY field_key) AS cleaned_node
    FROM cleaned_node_fields
    GROUP BY node_id
),
-- Rebuild the workflow array from cleaned nodes
cleaned_workflow AS (
    SELECT COALESCE(jsonb_agg(cleaned_node), '[]'::jsonb) AS workflow
    FROM cleaned_nodes
),
-- Update draft workflow version in place
updated AS (
    UPDATE workflow_versions AS wv
    SET 
        workflow = COALESCE(cw.workflow, '[]'::jsonb),
        last_editor = $2,
        updated_at = now()
    FROM latest_workflow AS lw, cleaned_workflow AS cw
    WHERE wv.id = lw.id 
      AND lw.is_active = false
    RETURNING wv.workflow
),
-- Create new draft version if current version is active
created AS (
    INSERT INTO workflow_versions (form_id, last_editor, workflow)
    SELECT $1, $2, COALESCE(cw.workflow, '[]'::jsonb)
    FROM latest_workflow AS lw, cleaned_workflow AS cw
    WHERE lw.is_active = true
    RETURNING workflow
)
-- Return workflow as JSONB
SELECT workflow FROM updated
UNION ALL
SELECT workflow FROM created;

-- name: Activate :one
WITH form_lock AS (
    -- Lock the form row to serialize activations per form
    SELECT 1
    FROM forms AS f
    WHERE f.id = @form_id
    FOR UPDATE
),
current_active AS (
    -- Get the currently active workflow version (if any) for comparison
    SELECT wv.id, wv.workflow
    FROM workflow_versions AS wv
    WHERE wv.form_id = @form_id
      AND wv.is_active = true
    ORDER BY wv.updated_at DESC
    LIMIT 1
),
latest AS (
    -- Get the latest workflow version (regardless of active status)
    -- Lock the row to prevent concurrent modifications
    SELECT wv.id, wv.is_active, wv.workflow
    FROM workflow_versions AS wv
    WHERE wv.form_id = @form_id
    ORDER BY wv.updated_at DESC
    LIMIT 1
    FOR UPDATE
),
request_workflow AS (
    -- Use the workflow from the request body
    SELECT @workflow::jsonb AS workflow
),
should_activate AS (
    /*
    Determine if activation should proceed and what action to take:
    
    Decision logic:
    1. If no workflow version exists -> create first version (already active)
    2. Else if request_workflow == current_active AND latest is active -> skip (unchanged)
    3. Else if latest is not active -> update latest with request_workflow and activate
    4. Else (latest is active but request != current_active) -> create new active version
    
    Fields:
    - latest_id: ID of the latest workflow version (by updated_at), NULL if none exists
    - latest_is_active: Whether the latest version is currently active
    - can_activate: Whether we should proceed with activation
    - should_update_latest: Whether to update the latest version (vs creating new one)
    */
    SELECT 
        l.id AS latest_id,
        l.is_active AS latest_is_active,
        CASE 
            -- No latest version exists - can activate (will create first version)
            WHEN l.id IS NULL THEN true
            -- Request matches current active AND latest is active - skip activation
            WHEN ca.id IS NOT NULL AND rw.workflow IS NOT DISTINCT FROM ca.workflow AND l.is_active = true THEN false
            -- Latest version is inactive - can activate (will update it)
            WHEN l.is_active = false THEN true
            -- Latest is active but request differs from current active - can activate (will create new version)
            ELSE true
        END AS can_activate,
        CASE 
            -- Update latest version if it's inactive
            WHEN l.is_active = false THEN true
            -- Do not update if latest is active or doesn't exist (will create a new version)
            ELSE false
        END AS should_update_latest
    FROM request_workflow AS rw
    LEFT JOIN latest AS l ON true
    LEFT JOIN current_active AS ca ON true
),
deactivated AS (
    -- Deactivate all currently active workflow versions FIRST
    -- This runs before the new version is created/updated
    UPDATE workflow_versions AS wv
    SET is_active = false
    FROM should_activate AS sa
    WHERE wv.form_id = @form_id
      AND wv.is_active = true
      AND sa.can_activate = true
    RETURNING wv.id
),
activated_from_update AS (
    -- Update the latest (inactive) version with new workflow AND activate it in ONE operation
    UPDATE workflow_versions AS wv
    SET workflow = rw.workflow,
        last_editor = @last_editor,
        is_active = true,
        updated_at = now()
    FROM should_activate AS sa, request_workflow AS rw
    WHERE wv.id = sa.latest_id
      AND wv.form_id = @form_id
      AND sa.should_update_latest = true
      AND sa.can_activate = true
    RETURNING wv.id, wv.form_id, wv.last_editor, wv.is_active, wv.workflow, wv.created_at, wv.updated_at
),
activated_from_insert AS (
    -- Create a new workflow version that is ALREADY active
    INSERT INTO workflow_versions (form_id, last_editor, workflow, is_active)
    SELECT @form_id, @last_editor, rw.workflow, true
    FROM request_workflow AS rw, should_activate AS sa
    WHERE sa.can_activate = true
      AND sa.should_update_latest = false
    RETURNING id, form_id, last_editor, is_active, workflow, created_at, updated_at
),
activated AS (
    -- Combine both activation results
    SELECT * FROM activated_from_update
    UNION ALL
    SELECT * FROM activated_from_insert
),
unchanged AS (
    -- Return the current active workflow when activation is skipped
    -- (request_workflow == current_active AND latest is active)
    SELECT wv.id, wv.form_id, wv.last_editor, wv.is_active, wv.workflow, wv.created_at, wv.updated_at
    FROM workflow_versions AS wv
    WHERE wv.id IN (SELECT id FROM current_active WHERE id IS NOT NULL)
      AND EXISTS (
        SELECT 1 FROM should_activate AS sa 
        WHERE sa.can_activate = false
      )
)
-- Return the activated version or unchanged version
SELECT * FROM activated
UNION ALL
SELECT * FROM unchanged;