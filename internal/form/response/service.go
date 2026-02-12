package response

import (
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/form/workflow/node"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type WorkflowStore interface {
	Get(ctx context.Context, formID uuid.UUID) (workflow.GetRow, error)
}

type Querier interface {
	Create(ctx context.Context, arg CreateParams) (FormResponse, error)
	Get(ctx context.Context, arg GetParams) (FormResponse, error)
	GetByFormIDAndSubmittedBy(ctx context.Context, arg GetByFormIDAndSubmittedByParams) (FormResponse, error)
	Exists(ctx context.Context, arg ExistsParams) (bool, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Update(ctx context.Context, arg UpdateParams) error
	Delete(ctx context.Context, id uuid.UUID) error
	CreateAnswer(ctx context.Context, arg CreateAnswerParams) (Answer, error)
	GetAnswersByQuestionID(ctx context.Context, arg GetAnswersByQuestionIDParams) (Answer, error)
	GetAnswersByResponseID(ctx context.Context, responseID uuid.UUID) ([]Answer, error)
	UpdateAnswer(ctx context.Context, arg UpdateAnswerParams) (Answer, error)
	AnswerExists(ctx context.Context, arg AnswerExistsParams) (bool, error)
	CheckAnswerContent(ctx context.Context, arg CheckAnswerContentParams) (bool, error)
	GetAnswerID(ctx context.Context, arg GetAnswerIDParams) (uuid.UUID, error)
	ListBySubmittedBy(ctx context.Context, submittedBy uuid.UUID) ([]FormResponse, error)
	GetFormIDByResponseID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	GetSectionsByIDs(ctx context.Context, dollar_1 []uuid.UUID) ([]GetSectionsByIDsRow, error)
	GetRequiredQuestionsBySectionIDs(ctx context.Context, sectionIDs []uuid.UUID) ([]GetRequiredQuestionsBySectionIDsRow, error)
}

type Service struct {
	logger        *zap.Logger
	queries       Querier
	tracer        trace.Tracer
	workflowStore WorkflowStore
}

func NewService(logger *zap.Logger, db DBTX, workflowStore WorkflowStore) *Service {
	return &Service{
		logger:        logger,
		queries:       New(db),
		tracer:        otel.Tracer("response/service"),
		workflowStore: workflowStore,
	}
}

type conditionRule struct {
	Source  string `json:"source"`
	Key     string `json:"key"` // question ID
	Pattern string `json:"pattern"`
}

// traverseWorkflowSections walks the workflow from the start node using answers
// to evaluate condition nodes, and returns section IDs in traversal order.
func traverseWorkflowSections(workflowJSON []byte, answerByQuestionID map[string]string) ([]uuid.UUID, error) {
	var nodes []map[string]interface{}
	if err := json.Unmarshal(workflowJSON, &nodes); err != nil {
		return nil, fmt.Errorf("parse workflow json: %w", err)
	}

	workflowMap := make(map[string]map[string]interface{})
	var startNodeID string
	for _, n := range nodes {
		id, _ := n["id"].(string)
		if id == "" {
			continue
		}

		workflowMap[id] = n
		typ, _ := n["type"].(string)
		if typ == node.TypeStart {
			startNodeID = id
		}
	}

	if startNodeID == "" {
		return nil, fmt.Errorf("workflow has no start node")
	}

	var sectionIDs []uuid.UUID
	visited := make(map[string]struct{})
	currentID := startNodeID

	for currentID != "" {
		_, visitedNode := visited[currentID]
		if visitedNode {
			break // cycle guard
		}
		visited[currentID] = struct{}{}

		currentNode, ok := workflowMap[currentID]
		if !ok {
			break
		}

		nodeType, _ := currentNode["type"].(string)
		switch nodeType {
		case node.TypeSection:
			id, err := uuid.Parse(currentID)
			if err == nil {
				sectionIDs = append(sectionIDs, id)
			}
			next, _ := currentNode["next"].(string)
			currentID = next

		case node.TypeCondition:
			ruleRaw, ok := currentNode["conditionRule"]
			if !ok {
				currentID, _ = currentNode["nextFalse"].(string)
				continue
			}

			ruleBytes, _ := json.Marshal(ruleRaw)

			var rule conditionRule
			if json.Unmarshal(ruleBytes, &rule) != nil {
				currentID, _ = currentNode["nextFalse"].(string)
				continue
			}

			val := answerByQuestionID[rule.Key]
			matched, _ := regexp.MatchString(rule.Pattern, val)
			if matched {
				currentID, _ = currentNode["nextTrue"].(string)
			} else {
				currentID, _ = currentNode["nextFalse"].(string)
			}

		case node.TypeStart:
			next, _ := currentNode["next"].(string)
			currentID = next

		case node.TypeEnd:
			currentID = ""
		default:
			next, _ := currentNode["next"].(string)
			currentID = next
		}
	}

	return sectionIDs, nil
}

// ListSections lists all sections of a response by traversing the workflow
// using stored answers to evaluate condition nodes. Sections are returned
// in the order they appear on the user's path. Progress comes from the sections table.
func (s Service) ListSections(ctx context.Context, responseID uuid.UUID) ([]SectionSummary, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListSections")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	formID, err := s.queries.GetFormIDByResponseID(traceCtx, responseID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", responseID.String(), logger, "get form id by response id")
		span.RecordError(err)
		return nil, err
	}

	workflow, err := s.workflowStore.Get(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get workflow by form id")
		span.RecordError(err)
		return nil, err
	}

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, responseID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answers", "response_id", responseID.String(), logger, "get answers by response id")
		span.RecordError(err)
		return nil, err
	}
	// use answerByQuestionID to traverse the workflow
	answersByQuestionID := make(map[string]string, len(answers))
	// use answeredQuestionIDs to check if a question is answered
	answeredQuestionIDs := make(map[uuid.UUID]struct{}, len(answers))

	for _, a := range answers {
		// Values are stored as JSON; attempt to unmarshal to a string.
		var value string
		err := json.Unmarshal(a.Value, &value)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "unmarshal answer value")
			span.RecordError(err)
			return nil, err
		}
		answersByQuestionID[a.QuestionID.String()] = value
		answeredQuestionIDs[a.QuestionID] = struct{}{}
	}

	sectionIDs, err := traverseWorkflowSections(workflow.Workflow, answersByQuestionID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if len(sectionIDs) == 0 {
		return []SectionSummary{}, nil
	}

	sectionRows, err := s.queries.GetSectionsByIDs(traceCtx, sectionIDs)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get sections by ids")
		span.RecordError(err)
		return nil, err
	}

	rowByID := make(map[uuid.UUID]GetSectionsByIDsRow, len(sectionRows))
	for _, row := range sectionRows {
		rowByID[row.ID] = row
	}

	requiredRows, err := s.queries.GetRequiredQuestionsBySectionIDs(traceCtx, sectionIDs)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get required questions by section ids")
		span.RecordError(err)
		return nil, err
	}

	requiredBySection := make(map[uuid.UUID][]uuid.UUID, len(requiredRows))
	for _, row := range requiredRows {
		requiredBySection[row.SectionID] = append(requiredBySection[row.SectionID], row.ID)
	}

	out := make([]SectionSummary, 0, len(sectionIDs))
	for _, sectionID := range sectionIDs {
		row, ok := rowByID[sectionID]
		if !ok {
			continue
		}

		// A section is SUBMITTED if all its required questions
		// have answers for this response; otherwise it is DRAFT.
		requiredQuestions := requiredBySection[sectionID]

		progress := SectionStatusDraft
		if len(requiredQuestions) > 0 {
			isComplete := true
			for _, qID := range requiredQuestions {
				if _, ok := answeredQuestionIDs[qID]; !ok {
					isComplete = false
					break
				}
			}
			if isComplete {
				progress = SectionStatusSubmitted
			}
		}

		out = append(out, SectionSummary{
			ID:       sectionID.String(),
			Title:    row.Title.String,
			Progress: progress,
		})
	}

	return out, nil
}

func (s Service) CreateOrUpdate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateOrUpdate")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	if len(answers) != len(questionType) {
		err := fmt.Errorf("number of answers (%d) does not match number of question types (%d)", len(answers), len(questionType))
		logger.Error("Failed to create response", zap.Error(err), zap.String("formID", formID.String()), zap.String("userID", userID.String()))
		span.RecordError(err)
		return FormResponse{}, err
	}

	exists, err := s.queries.Exists(traceCtx, ExistsParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "check if response exists")
		span.RecordError(err)
		return FormResponse{}, err
	}

	if exists {
		return s.UpdateAnswer(traceCtx, formID, userID, answers, questionType)
	} else {
		return s.CreateAnswer(traceCtx, formID, userID, answers, questionType)
	}
}

// Create creates a new response for a given form and user
func (s Service) Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Create")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	newResponse, err := s.queries.Create(traceCtx, CreateParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create response")
		span.RecordError(err)
		return FormResponse{}, err
	}

	return newResponse, nil
}

// UpdateAnswer updates the answers of the response
func (s Service) UpdateAnswer(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "Update")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	currentResponse, err := s.queries.GetByFormIDAndSubmittedBy(traceCtx, GetByFormIDAndSubmittedByParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "get response by form id and submitted by")
		span.RecordError(err)
		return FormResponse{}, err
	}

	for i, answer := range answers {
		// check if answer exists
		questionID, err := internal.ParseUUID(answer.QuestionID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "parse question id")
			span.RecordError(err)
			return FormResponse{}, err
		}
		answerExists, err := s.queries.AnswerExists(traceCtx, AnswerExistsParams{
			ResponseID: currentResponse.ID,
			QuestionID: questionID,
		})
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "check if answer exists")
			span.RecordError(err)
			return FormResponse{}, err
		}

		// if answer does not exist, create it
		if !answerExists {
			_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
				ResponseID: currentResponse.ID,
				QuestionID: questionID,
				Type:       questionType[i],
				Value:      answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "create answer")
				span.RecordError(err)
				return FormResponse{}, err
			}
		}

		// if answer exists, check if it is the same as the new answer
		sameAnswer, err := s.queries.CheckAnswerContent(traceCtx, CheckAnswerContentParams{
			ResponseID: currentResponse.ID,
			QuestionID: questionID,
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "check answer content")
			span.RecordError(err)
			return FormResponse{}, err
		}

		// if answer is different, update it
		if !sameAnswer {
			answerID, err := s.queries.GetAnswerID(traceCtx, GetAnswerIDParams{
				ResponseID: currentResponse.ID,
				QuestionID: questionID,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "get answer id")
				span.RecordError(err)
				return FormResponse{}, err
			}
			_, err = s.queries.UpdateAnswer(traceCtx, UpdateAnswerParams{
				ID:    answerID,
				Value: answer.Value,
			})
			if err != nil {
				err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "id", answerID.String(), logger, "update answer")
				span.RecordError(err)
				return FormResponse{}, err
			}
		}
	}

	// update the value of updated_at of response
	err = s.queries.Update(traceCtx, UpdateParams{
		ID:       currentResponse.ID,
		Progress: currentResponse.Progress,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", currentResponse.ID.String(), logger, "update response")
		span.RecordError(err)
		return FormResponse{}, err
	}
	return currentResponse, nil
}

// CreateAnswer creates a new answer for a given response
func (s Service) CreateAnswer(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "CreateAnswer")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	newResponse, err := s.queries.Create(traceCtx, CreateParams{
		FormID:      formID,
		SubmittedBy: userID,
	})
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "create answer")
		span.RecordError(err)
		return FormResponse{}, err
	}

	for i, answer := range answers {
		questionID, err := internal.ParseUUID(answer.QuestionID)
		if err != nil {
			err = databaseutil.WrapDBError(err, logger, "parse question id")
			span.RecordError(err)
			return FormResponse{}, err
		}

		_, err = s.queries.CreateAnswer(traceCtx, CreateAnswerParams{
			ResponseID: newResponse.ID,
			QuestionID: questionID,
			Type:       questionType[i],
			Value:      answer.Value,
		})
		if err != nil {
			err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", newResponse.ID.String(), logger, "create answer")
			span.RecordError(err)
			return FormResponse{}, err
		}
	}

	return newResponse, nil
}

// Get retrieves a response and answers by id
func (s Service) Get(ctx context.Context, formID uuid.UUID, id uuid.UUID) (FormResponse, []Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "Get")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	currentResponse, err := s.queries.Get(traceCtx, GetParams{
		ID:     id,
		FormID: formID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "get response by id")
		span.RecordError(err)
		return FormResponse{}, []Answer{}, err
	}

	answers, err := s.queries.GetAnswersByResponseID(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "response_id", currentResponse.ID.String(), logger, "get answers by response id")
		span.RecordError(err)
		return FormResponse{}, []Answer{}, err
	}

	return currentResponse, answers, nil
}

// ListByFormID retrieves all responses for a given form
func (s Service) ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error) {
	traceCtx, span := s.tracer.Start(ctx, "ListByFormID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	responses, err := s.queries.ListByFormID(traceCtx, formID)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "form_id", formID.String(), logger, "list responses by form id")
		span.RecordError(err)
		return []FormResponse{}, err
	}

	return responses, nil
}

// Delete deletes a response by id
func (s Service) Delete(ctx context.Context, id uuid.UUID) error {
	traceCtx, span := s.tracer.Start(ctx, "Delete")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	err := s.queries.Delete(traceCtx, id)
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "response", "id", id.String(), logger, "delete response")
		span.RecordError(err)
		return err
	}

	return nil
}

// GetAnswersByQuestionID retrieves all answers for a given question
func (s Service) GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, responseID uuid.UUID) (Answer, error) {
	traceCtx, span := s.tracer.Start(ctx, "GetAnswersByQuestionID")
	defer span.End()
	logger := logutil.WithContext(traceCtx, s.logger)

	row, err := s.queries.GetAnswersByQuestionID(traceCtx, GetAnswersByQuestionIDParams{
		QuestionID: questionID,
		ResponseID: responseID,
	})
	if err != nil {
		err = databaseutil.WrapDBErrorWithKeyValue(err, "answer", "question_id", questionID.String(), logger, "get answers by question id")
		span.RecordError(err)
		return Answer{}, err
	}

	return row, nil
}

func (s Service) ListBySubmittedBy(ctx context.Context, userID uuid.UUID) ([]FormResponse, error) {
	ctx, span := s.tracer.Start(ctx, "ListBySubmittedBy")
	defer span.End()
	logger := logutil.WithContext(ctx, s.logger)

	responses, err := s.queries.ListBySubmittedBy(ctx, userID)
	if err != nil {
		err = databaseutil.WrapDBError(err, logger, "list responses by submitted by")
		span.RecordError(err)
		return nil, err
	}

	return responses, nil
}
