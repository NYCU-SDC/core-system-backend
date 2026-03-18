package workflow

import (
	"context"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/test/integration"
	"NYCU-SDC/core-system-backend/test/testdata/dbbuilder"
	workflowbuilder "NYCU-SDC/core-system-backend/test/testdata/dbbuilder/workflow"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestWorkflowService_GetValidationInfo tests the GetValidationInfo method
// which returns ValidationInfo array with nodeId and message for each validation error.
func TestWorkflowService_GetValidationInfo(t *testing.T) {
	type Params struct {
		formID       uuid.UUID
		workflowJSON []byte
	}

	type testCase struct {
		name         string
		params       Params
		setup        func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context
		expectedInfo bool
	}

	testCases := []testCase{
		{
			name:   "valid workflow - no errors",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-valid-org", "info-valid-unit")
				params.formID = data.FormRow.ID
				workflowJSON, _, _ := builder.CreateStartEndWorkflow()
				params.workflowJSON = workflowJSON
				return context.Background()
			},
			expectedInfo: false,
		},
		{
			name:   "invalid JSON format",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-invalid-json-org", "info-invalid-json-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = []byte(`{invalid json}`)
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "empty workflow",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-empty-org", "info-empty-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = []byte(`[]`)
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "missing start node",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-missing-start-org", "info-missing-start-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowMissingStartNode()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "missing end node",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-missing-end-org", "info-missing-end-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowMissingEndNode()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "multiple start nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-multiple-start-org", "info-multiple-start-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithMultipleStarts()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "multiple end nodes",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-multiple-end-org", "info-multiple-end-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithMultipleEnds()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "duplicate node IDs",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-duplicate-ids-org", "info-duplicate-ids-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithDuplicateIDs()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "invalid node ID format",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-invalid-id-org", "info-invalid-id-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithInvalidID()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "missing required fields - label",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-missing-label-org", "info-missing-label-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowMissingLabel()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "unreachable node",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-unreachable-org", "info-unreachable-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithUnreachableNode()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "invalid node reference",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-invalid-ref-org", "info-invalid-ref-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithInvalidReference()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "invalid node type",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-invalid-type-org", "info-invalid-type-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateWorkflowWithInvalidType()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "start node missing next field",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-start-no-next-org", "info-start-no-next-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateStartNodeMissingNext()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "section node missing next field",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-section-no-next-org", "info-section-no-next-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateSectionNodeMissingNext()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "condition node missing nextTrue",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-cond-no-next-true-org", "info-cond-no-next-true-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateConditionNodeMissingNextTrue()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "condition node missing nextFalse",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-cond-no-next-false-org", "info-cond-no-next-false-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateConditionNodeMissingNextFalse()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "condition node missing conditionRule",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-cond-no-rule-org", "info-cond-no-rule-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateConditionNodeMissingRule()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "condition node invalid conditionRule source",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-cond-bad-source-org", "info-cond-bad-source-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateConditionNodeInvalidSource()
				return context.Background()
			},
			expectedInfo: true,
		},
		{
			name:   "condition node invalid regex pattern",
			params: Params{},
			setup: func(t *testing.T, params *Params, db dbbuilder.DBTX) context.Context {
				builder := workflowbuilder.New(t, db)
				data := builder.SetupTestData("info-cond-bad-regex-org", "info-cond-bad-regex-unit")
				params.formID = data.FormRow.ID
				params.workflowJSON = builder.CreateConditionNodeInvalidRegex()
				return context.Background()
			},
			expectedInfo: true,
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

			// Call GetValidationInfo which returns ValidationInfo array
			validationInfos, err := workflowService.GetValidationInfo(ctx, params.formID, params.workflowJSON)

			require.NoError(t, err, "GetValidationInfo should not return error")
			require.NotNil(t, validationInfos)

			if !tc.expectedInfo {
				require.Len(t, validationInfos, 0, "expected no validation info")
			} else {
				require.Greater(t, len(validationInfos), 0, "expected at least one validation info")

				for _, info := range validationInfos {
					require.NotEmpty(t, info.Message)
				}
			}
		})
	}
}
