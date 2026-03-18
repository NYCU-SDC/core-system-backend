package workflow

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	workflowbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/workflow"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkflowService_ActivateValidation(t *testing.T) {
	type Params struct {
		formID       uuid.UUID
		userID       uuid.UUID
		workflowJSON []byte
	}

	type testCase struct {
		name        string
		params      Params
		setup       func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
		validate    func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error)
		expectedErr bool
	}

	testCases := []testCase{
		{
			name:   "valid workflow activates successfully",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-success-org", "validate-success-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				workflowJSON, _, _ := builder.CreateStartEndWorkflow()
				params.workflowJSON = workflowJSON
				return context.Background()
			},
			validate: func(t *testing.T, params Params, db dbbuilder.DBTX, result workflow.ActivateRow, err error) {
				require.NoError(t, err, "should not return error for valid workflow")
				require.NotEqual(t, uuid.Nil, result.ID, "workflow version ID should be set")
				require.Equal(t, params.formID, result.FormID, "form ID should match")
				require.Equal(t, params.userID, result.LastEditor, "last editor should match")
				require.True(t, result.IsActive, "workflow should be active")

				builder := workflowbuilder.New(t, db)
				// Verify workflow content
				workflowData := builder.ParseWorkflow(result.Workflow)
				require.True(t, builder.HasNodeType(workflowData, "start"), "workflow should have start node")
				require.True(t, builder.HasNodeType(workflowData, "end"), "workflow should have end node")
			},
			expectedErr: false,
		},
		{
			name:   "invalid JSON format",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-invalid-json-org", "validate-invalid-json-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = []byte(`{invalid json}`)
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "empty workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-empty-org", "validate-empty-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = []byte(`[]`)
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "missing start node",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-missing-start-org", "validate-missing-start-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowMissingStartNode()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "missing end node",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-missing-end-org", "validate-missing-end-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowMissingEndNode()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "multiple start nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-multiple-start-org", "validate-multiple-start-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithMultipleStarts()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "multiple end nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-multiple-end-org", "validate-multiple-end-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithMultipleEnds()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "duplicate node IDs",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-duplicate-ids-org", "validate-duplicate-ids-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithDuplicateIDs()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "invalid node ID format",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-invalid-id-org", "validate-invalid-id-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithInvalidID()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "missing required fields",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-missing-fields-org", "validate-missing-fields-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowMissingLabel()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "unreachable nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-unreachable-org", "validate-unreachable-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithUnreachableNode()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "invalid node references",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-invalid-ref-org", "validate-invalid-ref-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithInvalidReference()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "invalid node type",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-invalid-type-org", "validate-invalid-type-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateWorkflowWithInvalidType()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "start node missing next field",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-start-missing-next-org", "validate-start-missing-next-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateStartNodeMissingNext()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "section node missing next field",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-section-missing-next-org", "validate-section-missing-next-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateSectionNodeMissingNext()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "condition node missing conditionRule",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-condition-missing-rule-org", "validate-condition-missing-rule-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateConditionNodeMissingRule()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "condition node missing nextTrue",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-condition-missing-next-true-org", "validate-condition-missing-next-true-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateConditionNodeMissingNextTrue()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "condition node missing nextFalse",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-condition-missing-next-false-org", "validate-condition-missing-next-false-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateConditionNodeMissingNextFalse()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "condition node invalid conditionRule source",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-condition-invalid-source-org", "validate-condition-invalid-source-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateConditionNodeInvalidSource()
				return context.Background()
			},
			expectedErr: true,
		},
		{
			name:   "condition node invalid regex pattern",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("validate-condition-invalid-regex-org", "validate-condition-invalid-regex-unit")
				params.formID = data.FormRow.ID
				params.userID = data.User
				params.workflowJSON = builder.CreateConditionNodeInvalidRegex()
				return context.Background()
			},
			expectedErr: true,
		},
	}

	resourceManager, logger, err := integration.GetOrInitResource()
	if err != nil {
		t.Fatalf("failed to get resource manager: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, rollback, err := resourceManager.SetupPostgres()
			if err != nil {
				t.Fatalf("failed to setup postgres: %v", err)
			}
			defer rollback()

			ctx := context.Background()
			params := tc.params
			if tc.setup != nil {
				ctx = tc.setup(t, &params, db)
			}

			// Create question service to satisfy QuestionStore interface
			questionService := question.NewService(logger, db, nil)

			// Create workflow service with real dependencies
			workflowService := workflow.NewService(logger, db, questionService)

			// Call service.Activate which runs validation
			result, err := workflowService.Activate(ctx, params.formID, params.userID, params.workflowJSON)

			// Assert error expectations
			if tc.expectedErr {
				require.Error(t, err, "expected error but got nil")
				require.ErrorIs(t, err, internal.ErrWorkflowValidationFailed, "error should be ErrWorkflowValidationFailed")
				// When error is expected, result should be zero value
				require.Equal(t, uuid.Nil, result.ID, "result ID should be zero when error occurs")
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
				require.NotEqual(t, uuid.Nil, result.ID, "result ID should be set when no error")
			}

			// Run additional validation if provided
			if tc.validate != nil {
				tc.validate(t, params, db, result, err)
			}
		})
	}
}
