package workflowbuilder

import (
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/unit"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	formbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/form"
	unitbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/unit"
	userbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/user"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type Builder struct {
	t  *testing.T
	db dbbuilder.DBTX
}

func New(t *testing.T, db dbbuilder.DBTX) *Builder {
	return &Builder{t: t, db: db}
}

func (b Builder) Queries() *workflow.Queries {
	return workflow.New(b.db)
}

// TestData contains common test data structures
type TestData struct {
	Org     unit.Unit
	UnitRow unit.Unit
	User    uuid.UUID
	FormRow form.CreateRow
	Queries *workflow.Queries
}

// SetupTestData creates common test data (org, unit, user, form)
func (b Builder) SetupTestData(orgName, unitName string) TestData {
	unitBuilder := unitbuilder.New(b.t, b.db)
	userBuilder := userbuilder.New(b.t, b.db)
	formBuilder := formbuilder.New(b.t, b.db)

	org := unitBuilder.Create(unit.UnitTypeOrganization, unitbuilder.WithName(orgName))
	unitRow := unitBuilder.Create(unit.UnitTypeUnit, unitbuilder.WithOrgID(org.ID), unitbuilder.WithName(unitName))
	user := userBuilder.Create()
	userBuilder.CreateEmail(user.ID, "user@example.com")

	formRow := formBuilder.Create(
		formbuilder.WithUnitID(unitRow.ID),
		formbuilder.WithLastEditor(user.ID),
	)

	return TestData{
		Org:     org,
		UnitRow: unitRow,
		User:    user.ID,
		FormRow: formRow,
		Queries: b.Queries(),
	}
}

// CreateStartEndWorkflow creates a simple workflow with start -> end
// Returns: workflowJSON, startID, endID
func (b Builder) CreateStartEndWorkflow() ([]byte, uuid.UUID, uuid.UUID) {
	startID := uuid.New()
	endID := uuid.New()

	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)

	return workflowJSON, startID, endID
}

// CreateStartSectionEndWorkflow creates a workflow with start -> section -> end
// Returns: workflowJSON, startID, sectionID, endID
func (b Builder) CreateStartSectionEndWorkflow() ([]byte, uuid.UUID, uuid.UUID, uuid.UUID) {
	startID := uuid.New()
	sectionID := uuid.New()
	endID := uuid.New()

	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)

	return workflowJSON, startID, sectionID, endID
}

// CreateStartConditionEndWorkflow creates a workflow with start -> condition -> end
// Returns: workflowJSON, startID, conditionID, endID
func (b Builder) CreateStartConditionEndWorkflow() ([]byte, uuid.UUID, uuid.UUID, uuid.UUID) {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()

	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)

	return workflowJSON, startID, conditionID, endID
}

// CreateDraftWorkflow creates a draft workflow version
func (b Builder) CreateDraftWorkflow(formID uuid.UUID, userID uuid.UUID, workflowJSON []byte) {
	queries := b.Queries()
	_, err := queries.Update(context.Background(), workflow.UpdateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflowJSON,
	})
	require.NoError(b.t, err)
}

// CreateActiveWorkflow creates an active workflow version
func (b Builder) CreateActiveWorkflow(formID uuid.UUID, userID uuid.UUID, workflowJSON []byte) {
	queries := b.Queries()
	_, err := queries.Activate(context.Background(), workflow.ActivateParams{
		FormID:     formID,
		LastEditor: userID,
		Workflow:   workflowJSON,
	})
	require.NoError(b.t, err)
}

// CreateSectionRecord creates a section record in the database
func (b Builder) CreateSectionRecord(sectionID uuid.UUID, formID uuid.UUID, title string) {
	_, err := b.db.Exec(context.Background(),
		"INSERT INTO sections (id, form_id, title, progress) VALUES ($1, $2, $3, $4)",
		sectionID, formID, title, "draft")
	require.NoError(b.t, err)
}

// SectionExists checks if a section record exists
func (b Builder) SectionExists(sectionID uuid.UUID) bool {
	var exists bool
	err := b.db.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM sections WHERE id = $1)", sectionID).Scan(&exists)
	require.NoError(b.t, err)
	return exists
}

// ParseWorkflow parses workflow JSON into a slice of node maps
func (b Builder) ParseWorkflow(workflowJSON []byte) []map[string]interface{} {
	var workflowData []map[string]interface{}
	err := json.Unmarshal(workflowJSON, &workflowData)
	require.NoError(b.t, err)
	return workflowData
}

// NodeExists checks if a node with the given ID exists in the workflow
func (b Builder) NodeExists(workflowData []map[string]interface{}, nodeID string) bool {
	for _, node := range workflowData {
		if node["id"] == nodeID {
			return true
		}
	}
	return false
}

// HasNodeType checks if the workflow contains a node of the given type
func (b Builder) HasNodeType(workflowData []map[string]interface{}, nodeType string) bool {
	for _, node := range workflowData {
		if nt, ok := node["type"].(string); ok && nt == nodeType {
			return true
		}
	}
	return false
}

// NodeReferencesDeletedNode checks if any node references the deleted node
func (b Builder) NodeReferencesDeletedNode(workflowData []map[string]interface{}, deletedNodeID string, referenceFields ...string) bool {
	for _, node := range workflowData {
		for _, field := range referenceFields {
			ref, exists := node[field]
			if exists {
				refStr, ok := ref.(string)
				if ok && refStr == deletedNodeID {
					return true
				}
			}
		}
	}
	return false
}

// CountActiveVersions counts the number of active workflow versions for a form
func (b Builder) CountActiveVersions(formID uuid.UUID) int {
	var count int
	err := b.db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM workflow_versions WHERE form_id = $1 AND is_active = true",
		formID).Scan(&count)
	require.NoError(b.t, err)
	return count
}

// GetActiveVersionID gets the ID of the active workflow version for a form
func (b Builder) GetActiveVersionID(formID uuid.UUID) uuid.UUID {
	var activeID uuid.UUID
	err := b.db.QueryRow(context.Background(),
		"SELECT id FROM workflow_versions WHERE form_id = $1 AND is_active = true ORDER BY updated_at DESC LIMIT 1",
		formID).Scan(&activeID)
	require.NoError(b.t, err)
	return activeID
}

// CreateWorkflowMissingStartNode creates a workflow without a start node
func (b Builder) CreateWorkflowMissingStartNode() []byte {
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowMissingEndNode creates a workflow without an end node
func (b Builder) CreateWorkflowMissingEndNode() []byte {
	startID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  uuid.New().String(),
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithMultipleStarts creates a workflow with multiple start nodes
func (b Builder) CreateWorkflowWithMultipleStarts() []byte {
	startID1 := uuid.New()
	startID2 := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID1.String(),
			"type":  "start",
			"label": "Start 1",
			"next":  endID.String(),
		},
		{
			"id":    startID2.String(),
			"type":  "start",
			"label": "Start 2",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithMultipleEnds creates a workflow with multiple end nodes
func (b Builder) CreateWorkflowWithMultipleEnds() []byte {
	startID := uuid.New()
	endID1 := uuid.New()
	endID2 := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  endID1.String(),
		},
		{
			"id":    endID1.String(),
			"type":  "end",
			"label": "End 1",
		},
		{
			"id":    endID2.String(),
			"type":  "end",
			"label": "End 2",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithDuplicateIDs creates a workflow with duplicate node IDs
func (b Builder) CreateWorkflowWithDuplicateIDs() []byte {
	duplicateID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    duplicateID.String(),
			"type":  "start",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    duplicateID.String(), // Duplicate ID
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithInvalidID creates a workflow with invalid UUID format for node ID
func (b Builder) CreateWorkflowWithInvalidID() []byte {
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    "not-a-valid-uuid",
			"type":  "start",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowMissingLabel creates a workflow with a node missing the label field
func (b Builder) CreateWorkflowMissingLabel() []byte {
	startID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			// Missing "label" field
			"next": endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithUnreachableNode creates a workflow with an unreachable node
func (b Builder) CreateWorkflowWithUnreachableNode() []byte {
	startID := uuid.New()
	endID := uuid.New()
	orphanID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
		{
			"id":    orphanID.String(),
			"type":  "section",
			"label": "Orphan",
			"next":  endID.String(),
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithInvalidReference creates a workflow with a node referencing a non-existent node
func (b Builder) CreateWorkflowWithInvalidReference() []byte {
	startID := uuid.New()
	endID := uuid.New()
	nonExistentID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  nonExistentID.String(), // References non-existent node
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateWorkflowWithInvalidType creates a workflow with an invalid node type
func (b Builder) CreateWorkflowWithInvalidType() []byte {
	startID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "invalid_type",
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateStartNodeMissingNext creates a workflow with a start node missing the next field
func (b Builder) CreateStartNodeMissingNext() []byte {
	startID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			// Missing "next" field
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateSectionNodeMissingNext creates a workflow with a section node missing the next field
func (b Builder) CreateSectionNodeMissingNext() []byte {
	startID := uuid.New()
	sectionID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			// Missing "next" field
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateConditionNodeMissingRule creates a workflow with a condition node missing conditionRule
func (b Builder) CreateConditionNodeMissingRule() []byte {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			// Missing "conditionRule" field
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateConditionNodeMissingNextTrue creates a workflow with a condition node missing nextTrue
func (b Builder) CreateConditionNodeMissingNextTrue() []byte {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			// Missing "nextTrue" field
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "yes",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateConditionNodeMissingNextFalse creates a workflow with a condition node missing nextFalse
func (b Builder) CreateConditionNodeMissingNextFalse() []byte {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":       conditionID.String(),
			"type":     "condition",
			"label":    "Condition",
			"nextTrue": endID.String(),
			// Missing "nextFalse" field
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "yes",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateConditionNodeInvalidSource creates a workflow with a condition node having invalid conditionRule.source
func (b Builder) CreateConditionNodeInvalidSource() []byte {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "invalid_source", // Invalid source
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "yes",
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}

// CreateConditionNodeInvalidRegex creates a workflow with a condition node having invalid regex pattern
func (b Builder) CreateConditionNodeInvalidRegex() []byte {
	startID := uuid.New()
	conditionID := uuid.New()
	endID := uuid.New()
	sectionID := uuid.New()
	workflowJSON, err := json.Marshal([]map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  "start",
			"label": "Start",
			"next":  conditionID.String(),
		},
		{
			"id":        conditionID.String(),
			"type":      "condition",
			"label":     "Condition",
			"nextTrue":  endID.String(),
			"nextFalse": endID.String(),
			"conditionRule": map[string]interface{}{
				"source":  "choice",
				"nodeId":  sectionID.String(),
				"key":     uuid.New().String(),
				"pattern": "[invalid regex", // Invalid regex pattern
			},
		},
		{
			"id":    sectionID.String(),
			"type":  "section",
			"label": "Section",
			"next":  conditionID.String(),
		},
		{
			"id":    endID.String(),
			"type":  "end",
			"label": "End",
		},
	})
	require.NoError(b.t, err)
	return workflowJSON
}
