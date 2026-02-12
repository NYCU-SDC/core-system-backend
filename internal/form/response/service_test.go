package response

import (
	"context"
	"encoding/json"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/form/workflow/node"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func mustJSONMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// mockWorkflowStore implements WorkflowStore for testing ListSections.
type mockWorkflowStore struct {
	mock.Mock
}

func (m *mockWorkflowStore) Get(ctx context.Context, formID uuid.UUID) (workflow.GetRow, error) {
	args := m.Called(ctx, formID)
	row, _ := args.Get(0).(workflow.GetRow)
	return row, args.Error(1)
}

// mockQuerier implements Querier for testing ListSections.
// Only the methods used by ListSections are configured in tests; others are no-ops.
type mockQuerier struct {
	mock.Mock
}

func (m *mockQuerier) Create(ctx context.Context, arg CreateParams) (FormResponse, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(FormResponse)
	return row, args.Error(1)
}

func (m *mockQuerier) Get(ctx context.Context, arg GetParams) (FormResponse, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(FormResponse)
	return row, args.Error(1)
}

func (m *mockQuerier) GetByFormIDAndSubmittedBy(ctx context.Context, arg GetByFormIDAndSubmittedByParams) (FormResponse, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(FormResponse)
	return row, args.Error(1)
}

func (m *mockQuerier) Exists(ctx context.Context, arg ExistsParams) (bool, error) {
	args := m.Called(ctx, arg)
	return args.Bool(0), args.Error(1)
}

func (m *mockQuerier) ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error) {
	args := m.Called(ctx, formID)
	rows, _ := args.Get(0).([]FormResponse)
	return rows, args.Error(1)
}

func (m *mockQuerier) Update(ctx context.Context, arg UpdateParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) CreateAnswer(ctx context.Context, arg CreateAnswerParams) (Answer, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(Answer)
	return row, args.Error(1)
}

func (m *mockQuerier) GetAnswersByQuestionID(ctx context.Context, arg GetAnswersByQuestionIDParams) (Answer, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(Answer)
	return row, args.Error(1)
}

func (m *mockQuerier) GetAnswersByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error) {
	args := m.Called(ctx, responseID)
	rows, _ := args.Get(0).([]Answer)
	return rows, args.Error(1)
}

func (m *mockQuerier) UpdateAnswer(ctx context.Context, arg UpdateAnswerParams) (Answer, error) {
	args := m.Called(ctx, arg)
	row, _ := args.Get(0).(Answer)
	return row, args.Error(1)
}

func (m *mockQuerier) AnswerExists(ctx context.Context, arg AnswerExistsParams) (bool, error) {
	args := m.Called(ctx, arg)
	return args.Bool(0), args.Error(1)
}

func (m *mockQuerier) CheckAnswerContent(ctx context.Context, arg CheckAnswerContentParams) (bool, error) {
	args := m.Called(ctx, arg)
	return args.Bool(0), args.Error(1)
}

func (m *mockQuerier) GetAnswerID(ctx context.Context, arg GetAnswerIDParams) (uuid.UUID, error) {
	args := m.Called(ctx, arg)
	id, _ := args.Get(0).(uuid.UUID)
	return id, args.Error(1)
}

func (m *mockQuerier) ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]FormResponse, error) {
	args := m.Called(ctx, submittedBy)
	rows, _ := args.Get(0).([]FormResponse)
	return rows, args.Error(1)
}

func (m *mockQuerier) GetFormIDByResponseID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, id)
	formID, _ := args.Get(0).(uuid.UUID)
	return formID, args.Error(1)
}

func (m *mockQuerier) GetSectionsByIDs(ctx context.Context, ids []uuid.UUID) ([]GetSectionsByIDsRow, error) {
	args := m.Called(ctx, ids)
	rows, _ := args.Get(0).([]GetSectionsByIDsRow)
	return rows, args.Error(1)
}

func (m *mockQuerier) GetRequiredQuestionsBySectionIDs(ctx context.Context, sectionIDs []uuid.UUID) ([]GetRequiredQuestionsBySectionIDsRow, error) {
	args := m.Called(ctx, sectionIDs)
	rows, _ := args.Get(0).([]GetRequiredQuestionsBySectionIDsRow)
	return rows, args.Error(1)
}

// newTestService creates a Service with mocked dependencies.
func newTestService(t *testing.T) (*Service, *mockQuerier, *mockWorkflowStore) {
	t.Helper()

	logger := zap.NewNop()
	tracer := noop.NewTracerProvider().Tracer("test")

	q := &mockQuerier{}
	ws := &mockWorkflowStore{}

	return &Service{
		logger:        logger,
		queries:       q,
		tracer:        tracer,
		workflowStore: ws,
	}, q, ws
}

func Test_traverseWorkflowSections(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		setup       func(t *testing.T) ([]byte, map[string]string)
		expectedIDs []uuid.UUID
		expectedErr bool
	}

	sectionLinearA := uuid.New()
	sectionLinearB := uuid.New()

	sectionTrue := uuid.New()
	sectionFalse := uuid.New()

	cases := []testCase{
		{
			name: "simple linear workflow",
			setup: func(t *testing.T) ([]byte, map[string]string) {
				startID := uuid.New()
				endID := uuid.New()
				nodes := createWorkflow_Linear(startID, endID, sectionLinearA, sectionLinearB)

				return buildWorkflowJSON(t, nodes), map[string]string{}
			},
			expectedIDs: []uuid.UUID{sectionLinearA, sectionLinearB},
			expectedErr: false,
		},
		{
			name: "condition true branch taken",
			setup: func(t *testing.T) ([]byte, map[string]string) {
				startID := uuid.New()
				condID := uuid.New()
				endID := uuid.New()

				questionID := uuid.New().String()

				nodes := createWorkflow_ConditionRule(startID, condID, endID, sectionTrue, sectionFalse, questionID)

				answers := map[string]string{
					questionID: "yes",
				}

				return buildWorkflowJSON(t, nodes), answers
			},
			expectedIDs: []uuid.UUID{sectionTrue},
			expectedErr: false,
		},
		{
			name: "condition false branch taken",
			setup: func(t *testing.T) ([]byte, map[string]string) {
				startID := uuid.New()
				condID := uuid.New()
				endID := uuid.New()

				questionID := uuid.New().String()

				nodes := createWorkflow_ConditionRule(startID, condID, endID, sectionTrue, sectionFalse, questionID)

				answers := map[string]string{
					questionID: "no",
				}

				return buildWorkflowJSON(t, nodes), answers
			},
			expectedIDs: []uuid.UUID{sectionFalse},
			expectedErr: false,
		},
		{
			name: "missing start node returns error",
			setup: func(t *testing.T) ([]byte, map[string]string) {
				// Only a section node, no start node.
				nodes := createWorkflow_MissingStart()
				return buildWorkflowJSON(t, nodes), map[string]string{}
			},
			expectedIDs: nil,
			expectedErr: true,
		},
		{
			name: "cycle in graph is guarded",
			setup: func(t *testing.T) ([]byte, map[string]string) {
				startID := uuid.New()
				sectionID := uuid.New()

				nodes := createWorkflow_Cyclic(startID, sectionID)

				return buildWorkflowJSON(t, nodes), map[string]string{}
			},
			expectedIDs: nil,
			expectedErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workflowJSON, answers := tc.setup(t)
			ids, err := traverseWorkflowSections(workflowJSON, answers)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.expectedIDs != nil {
				require.Equal(t, len(tc.expectedIDs), len(ids))
				for i, want := range tc.expectedIDs {
					require.Equal(t, want, ids[i])
				}
			} else {
				require.NotNil(t, ids)
				require.NotZero(t, len(ids))
			}
		})
	}
}

func TestService_ListSections(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		setup       func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID)
		expectedErr bool
		assertions  func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore)
	}

	testCases := []testCase{
		{
			name: "simple linear sections with progress normalization and ordering",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				sectionID1 := uuid.New()
				sectionID2 := uuid.New()

				questionID1 := uuid.New()
				questionID2 := uuid.New()

				formID := uuid.New()
				startID := uuid.New()

				nodes := createWorkflow_ServiceLinear(startID, sectionID1, sectionID2)
				workflowJSON := buildWorkflowJSON(t, nodes)

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()

				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   true,
					Workflow:   workflowJSON,
				}, nil).Once()

				// Only the second section has its required question answered.
				q.On("GetAnswersByResponseID", mock.Anything, responseID).Return([]Answer{
					{
						QuestionID: questionID2,
						Value:      mustJSONMarshal(t, "some answer"),
					},
				}, nil).Once()

				rows := []GetSectionsByIDsRow{
					{
						ID:    sectionID2,
						Title: pgtype.Text{String: "Section 2", Valid: true},
					},
					{
						ID:    sectionID1,
						Title: pgtype.Text{String: "Section 1", Valid: true},
					},
				}

				q.On("GetSectionsByIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 2 &&
						ids[0] == sectionID1 &&
						ids[1] == sectionID2
				})).Return(rows, nil).Once()

				q.On("GetRequiredQuestionsBySectionIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 2 &&
						ids[0] == sectionID1 &&
						ids[1] == sectionID2
				})).Return([]GetRequiredQuestionsBySectionIDsRow{
					{ID: questionID1, SectionID: sectionID1},
					{ID: questionID2, SectionID: sectionID2},
				}, nil).Once()
			},
			expectedErr: false,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				require.Len(t, got, 2)

				require.Equal(t, "Section 1", got[0].Title)
				require.Equal(t, SectionStatusDraft, got[0].Progress)

				require.Equal(t, "Section 2", got[1].Title)
				require.Equal(t, SectionStatusSubmitted, got[1].Progress)

				q.AssertExpectations(t)
				ws.AssertExpectations(t)
			},
		},
		{
			name: "conditional branching based on answers",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				formID := uuid.New()
				questionID := uuid.New()
				sectionTrueID := uuid.New()
				sectionFalseID := uuid.New()

				startID := uuid.New()
				condID := uuid.New()
				endID := uuid.New()

				nodes := createWorkflow_ConditionRule(startID, condID, endID, sectionTrueID, sectionFalseID, questionID.String())
				workflowJSON := buildWorkflowJSON(t, nodes)

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()

				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   true,
					Workflow:   workflowJSON,
				}, nil).Once()

				answerValue, err := json.Marshal("yes")
				require.NoError(t, err)

				q.On("GetAnswersByResponseID", mock.Anything, responseID).Return([]Answer{
					{
						QuestionID: questionID,
						Value:      answerValue,
					},
				}, nil).Once()

				q.On("GetSectionsByIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 1 && ids[0] == sectionTrueID
				})).Return([]GetSectionsByIDsRow{
					{
						ID:    sectionTrueID,
						Title: pgtype.Text{String: "True Section", Valid: true},
					},
				}, nil).Once()

				q.On("GetRequiredQuestionsBySectionIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 1 && ids[0] == sectionTrueID
				})).Return([]GetRequiredQuestionsBySectionIDsRow{
					{ID: questionID, SectionID: sectionTrueID},
				}, nil).Once()
			},
			expectedErr: false,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				require.Len(t, got, 1)
				require.Equal(t, "True Section", got[0].Title)

				require.Equal(t, SectionStatusSubmitted, got[0].Progress)

				q.AssertExpectations(t)
				ws.AssertExpectations(t)
			},
		},
		{
			name: "no section nodes returns empty slice",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				formID := uuid.New()

				startID := uuid.New()
				endID := uuid.New()

				nodes := createWorkflow_NoSection(startID, endID)
				workflowJSON := buildWorkflowJSON(t, nodes)

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()
				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   true,
					Workflow:   workflowJSON,
				}, nil).Once()
				q.On("GetAnswersByResponseID", mock.Anything, responseID).Return([]Answer{}, nil).Once()
			},
			expectedErr: false,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				require.NotNil(t, got)
				require.Len(t, got, 0)

				q.AssertExpectations(t)
				ws.AssertExpectations(t)
			},
		},
		{
			name: "error from GetFormIDByResponseID",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				expectedErr := context.DeadlineExceeded
				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(uuid.UUID{}, expectedErr).Once()
			},
			expectedErr: true,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				q.AssertExpectations(t)
				ws.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
			},
		},
		{
			name: "error from WorkflowStore.Get",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				formID := uuid.New()

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()

				expectedErr := context.Canceled
				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{}, expectedErr).Once()
			},
			expectedErr: true,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				q.AssertExpectations(t)
				ws.AssertExpectations(t)

				q.AssertNotCalled(t, "GetAnswersByResponseID", mock.Anything, mock.Anything)
				q.AssertNotCalled(t, "GetSectionsByIDs", mock.Anything, mock.Anything)
			},
		},
		{
			name: "error from GetAnswersByResponseID",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				formID := uuid.New()
				startID := uuid.New()

				nodes := createWorkflow_StartOnly(startID)
				workflowJSON := buildWorkflowJSON(t, nodes)

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()
				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   true,
					Workflow:   workflowJSON,
				}, nil).Once()

				expectedErr := context.DeadlineExceeded
				q.On("GetAnswersByResponseID", mock.Anything, responseID).Return(nil, expectedErr).Once()
			},
			expectedErr: true,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				q.AssertExpectations(t)
				ws.AssertExpectations(t)
				q.AssertNotCalled(t, "GetSectionsByIDs", mock.Anything, mock.Anything)
			},
		},
		{
			name: "error from GetSectionsByIDs",
			setup: func(t *testing.T, s *Service, q *mockQuerier, ws *mockWorkflowStore, responseID uuid.UUID) {
				formID := uuid.New()
				sectionID := uuid.New()
				startID := uuid.New()

				nodes := createWorkflow_StartAndSection(startID, sectionID)
				workflowJSON := buildWorkflowJSON(t, nodes)

				q.On("GetFormIDByResponseID", mock.Anything, responseID).Return(formID, nil).Once()
				ws.On("Get", mock.Anything, formID).Return(workflow.GetRow{
					ID:         uuid.New(),
					FormID:     formID,
					LastEditor: uuid.New(),
					IsActive:   true,
					Workflow:   workflowJSON,
				}, nil).Once()

				q.On("GetAnswersByResponseID", mock.Anything, responseID).Return([]Answer{}, nil).Once()

				expectedErr := context.Canceled
				q.On("GetSectionsByIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
					return len(ids) == 1 && ids[0] == sectionID
				})).Return(nil, expectedErr).Once()
			},
			expectedErr: true,
			assertions: func(t *testing.T, got []SectionSummary, err error, q *mockQuerier, ws *mockWorkflowStore) {
				q.AssertExpectations(t)
				ws.AssertExpectations(t)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service, q, ws := newTestService(t)
			responseID := uuid.New()

			tc.setup(t, service, q, ws, responseID)

			sections, err := service.ListSections(context.Background(), responseID)

			if tc.expectedErr {
				require.Error(t, err)
				require.Nil(t, sections)
			} else {
				require.NoError(t, err)
			}

			if tc.assertions != nil {
				tc.assertions(t, sections, err, q, ws)
			}
		})
	}
}

// helper to create workflow JSON for tests.
func buildWorkflowJSON(t *testing.T, nodes []map[string]interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(nodes)
	require.NoError(t, err)
	return data
}

// helper to create a conditional workflow used in tests.
func createWorkflow_ConditionRule(
	startID, condID, endID, sectionTrue, sectionFalse uuid.UUID,
	questionID string,
) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  condID.String(),
		},
		{
			"id":        condID.String(),
			"type":      node.TypeCondition,
			"label":     "Condition",
			"nextTrue":  sectionTrue.String(),
			"nextFalse": sectionFalse.String(),
			"conditionRule": map[string]interface{}{
				"source":  "nonChoice",
				"key":     questionID,
				"pattern": "^yes$",
			},
		},
		{
			"id":    sectionTrue.String(),
			"type":  node.TypeSection,
			"label": "Section True",
			"next":  endID.String(),
		},
		{
			"id":    sectionFalse.String(),
			"type":  node.TypeSection,
			"label": "Section False",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  node.TypeEnd,
			"label": "End",
		},
	}
}

// helper to create a simple linear workflow used in traversal tests.
func createWorkflow_Linear(
	startID, endID, sectionA, sectionB uuid.UUID,
) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  sectionA.String(),
		},
		{
			"id":    sectionA.String(),
			"type":  node.TypeSection,
			"label": "Section A",
			"next":  sectionB.String(),
		},
		{
			"id":    sectionB.String(),
			"type":  node.TypeSection,
			"label": "Section B",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  node.TypeEnd,
			"label": "End",
		},
	}
}

// helper to create a workflow missing a start node.
func createWorkflow_MissingStart() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    uuid.New().String(),
			"type":  node.TypeSection,
			"label": "Section",
		},
	}
}

// helper to create a cyclic workflow graph.
func createWorkflow_Cyclic(startID, sectionID uuid.UUID) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  node.TypeSection,
			"label": "Section",
			"next":  startID.String(), // cycle back to start
		},
	}
}

// helper to create a simple linear workflow used in service tests.
func createWorkflow_ServiceLinear(startID, sectionID1, sectionID2 uuid.UUID) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  sectionID1.String(),
		},
		{
			"id":    sectionID1.String(),
			"type":  node.TypeSection,
			"label": "Section 1",
			"next":  sectionID2.String(),
		},
		{
			"id":    sectionID2.String(),
			"type":  node.TypeSection,
			"label": "Section 2",
			"next":  uuid.New().String(),
		},
	}
}

// helper to create a workflow with no section nodes (start -> end).
func createWorkflow_NoSection(startID, endID uuid.UUID) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  endID.String(),
		},
		{
			"id":    endID.String(),
			"type":  node.TypeEnd,
			"label": "End",
		},
	}
}

// helper to create a workflow containing only a start node.
func createWorkflow_StartOnly(startID uuid.UUID) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
		},
	}
}

// helper to create a workflow with a start node followed by a single section.
func createWorkflow_StartAndSection(startID, sectionID uuid.UUID) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":    startID.String(),
			"type":  node.TypeStart,
			"label": "Start",
			"next":  sectionID.String(),
		},
		{
			"id":    sectionID.String(),
			"type":  node.TypeSection,
			"label": "Section",
		},
	}
}
