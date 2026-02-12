package response

import (
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth"
	"NYCU-SDC/core-system-backend/internal/form/question"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type QuestionAnswerForGetResponse struct {
	QuestionID string `json:"questionId" validate:"required,uuid"`
	Answer     string `json:"answer" validate:"required"`
}

type QuestionResponse struct {
	ID                 string            `json:"id" validate:"required"`
	QuestionAnswerPair shared.AnswerJSON `json:"questionAnswerPairs" validate:"required"`
}

type AnswerForQuestionResponse struct {
	ID          string    `json:"id" validate:"required,uuid"`
	ResponseID  string    `json:"responseId" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	Value       string    `json:"value" validate:"required"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"` // for marking if the answer is updated
}

type Response struct {
	ID          string    `json:"id" validate:"required,uuid"`
	SubmittedBy string    `json:"submittedBy" validate:"required,uuid"`
	CreatedAt   time.Time `json:"createdAt" validate:"required,datetime"`
	UpdatedAt   time.Time `json:"updatedAt" validate:"required,datetime"`
}

type CreateResponse struct {
	ID string `json:"id" validate:"required,uuid"`
}

type ListResponse struct {
	FormID        string     `json:"formId" validate:"required,uuid"`
	ResponseJSONs []Response `json:"responses" validate:"required,dive"`
}

type GetResponse struct {
	ID                   string                         `json:"id" validate:"required,uuid"`
	FormID               string                         `json:"formId" validate:"required,uuid"`
	SubmittedBy          string                         `json:"submittedBy" validate:"required,uuid"`
	QuestionsAnswerPairs []QuestionAnswerForGetResponse `json:"questionsAnswerPairs" validate:"required,dive"`
	CreatedAt            time.Time                      `json:"createdAt" validate:"required,datetime"` // for sorting
	UpdatedAt            time.Time                      `json:"updatedAt" validate:"required,datetime"` // for marking if the response is updated
}

type AnswersForQuestionResponse struct {
	Question question.Question           `json:"question" validate:"required"`
	Answers  []AnswerForQuestionResponse `json:"answers" validate:"required,dive"`
}

type SectionStatus string

const (
	SectionStatusDraft     SectionStatus = "DRAFT"
	SectionStatusSubmitted SectionStatus = "SUBMITTED"
)

type SectionSummary struct {
	ID       string        `json:"id" validate:"required,uuid"`
	Title    string        `json:"title" validate:"required"`
	Progress SectionStatus `json:"progress" validate:"required,oneof=DRAFT SUBMITTED"`
}

type ListSectionsResponse struct {
	Sections []SectionSummary `json:"sections" validate:"required,dive"`
}

type Store interface {
	Get(ctx context.Context, formID uuid.UUID, responseID uuid.UUID) (FormResponse, []Answer, error)
	ListByFormID(ctx context.Context, formID uuid.UUID) ([]FormResponse, error)
	Delete(ctx context.Context, responseID uuid.UUID) error
	GetAnswersByQuestionID(ctx context.Context, questionID uuid.UUID, responseID uuid.UUID) (Answer, error)
	Create(ctx context.Context, formID uuid.UUID, userID uuid.UUID) (FormResponse, error)
	UpdateAnswer(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error)
	CreateOrUpdate(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam, questionType []QuestionType) (FormResponse, error)
	ListSections(ctx context.Context, responseID uuid.UUID) ([]SectionSummary, error)
}

type QuestionStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	questionStore QuestionStore
	provider      map[string]auth.OAuthProvider
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, store Store, questionStore QuestionStore, providers map[string]auth.OAuthProvider) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		questionStore: questionStore,
		provider:      providers,
		tracer:        otel.Tracer("response/handler"),
	}
}

// ListHandler lists all responses for a form
func (h *Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses, err := h.store.ListByFormID(traceCtx, formID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	listResponse := ListResponse{
		FormID:        formID.String(),
		ResponseJSONs: make([]Response, len(responses)),
	}
	for i, currentResponse := range responses {
		listResponse.ResponseJSONs[i] = Response{
			ID:          currentResponse.ID.String(),
			SubmittedBy: currentResponse.SubmittedBy.String(),
			CreatedAt:   currentResponse.CreatedAt.Time,
			UpdatedAt:   currentResponse.UpdatedAt.Time,
		}
	}
	handlerutil.WriteJSONResponse(w, http.StatusOK, listResponse)
}

// GetHandler retrieves a response by id
func (h *Handler) GetHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	idStr := r.PathValue("responseId")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentResponse, answers, err := h.store.Get(traceCtx, formID, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionAnswerResponses := make([]QuestionAnswerForGetResponse, len(answers))
	for i, answer := range answers {
		q, err := h.questionStore.GetByID(traceCtx, answer.QuestionID)
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		questionAnswerResponses[i] = QuestionAnswerForGetResponse{
			QuestionID: q.Question().ID.String(),
			Answer:     string(answer.Value),
		}
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, GetResponse{
		ID:                   currentResponse.ID.String(),
		FormID:               currentResponse.FormID.String(),
		SubmittedBy:          currentResponse.SubmittedBy.String(),
		QuestionsAnswerPairs: questionAnswerResponses,
		CreatedAt:            currentResponse.CreatedAt.Time,
		UpdatedAt:            currentResponse.UpdatedAt.Time,
	})
}

// DeleteHandler deletes a response by id
func (h *Handler) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "DeleteHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	idStr := r.PathValue("responseId")
	id, err := internal.ParseUUID(idStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	err = h.store.Delete(traceCtx, id)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusNoContent, nil)
}

// GetAnswersByQuestionIDHandler retrieves an answer and converts choice IDs to names
func (h *Handler) GetAnswersByQuestionIDHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetAnswersByQuestionIDHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	responseIDStr := r.PathValue("responseId")
	responseID, err := internal.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIDStr := r.PathValue("questionId")
	questionID, err := internal.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	derivedQuestion, err := h.questionStore.GetByID(traceCtx, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Validate that the question has a source ID
	if !derivedQuestion.Question().SourceID.Valid {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("question does not have a source ID"), logger)
		return
	}

	sourceQuestionID := derivedQuestion.Question().SourceID.Bytes

	// Fetch the answer to the source question from this response
	answer, err := h.store.GetAnswersByQuestionID(traceCtx, sourceQuestionID, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Fetch the source question to get its choices metadata
	sourceQuestion, err := h.questionStore.GetByID(traceCtx, sourceQuestionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Extract choices from the source question's metadata
	choices, err := question.ExtractChoices(sourceQuestion.Question().Metadata)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Build a map for quick ID-to-name lookup
	choiceMap := make(map[uuid.UUID]string, len(choices))
	for _, choice := range choices {
		choiceMap[choice.ID] = choice.Name
	}

	// Parse the answer value as a JSON array of choice IDs
	var choiceIDs []string
	if err := json.Unmarshal(answer.Value, &choiceIDs); err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to parse answer value: %w", err), logger)
		return
	}

	// Convert choice IDs to choice names
	choiceNames := make([]string, 0, len(choiceIDs))
	for _, idStr := range choiceIDs {
		choiceID, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid choice ID in answer: %w", err), logger)
			return
		}

		if name, ok := choiceMap[choiceID]; ok {
			choiceNames = append(choiceNames, name)
		}
	}

	response := QuestionResponse{
		ID: responseIDStr,
		QuestionAnswerPair: shared.AnswerJSON{
			QuestionID:   questionIDStr,
			QuestionType: string(answer.Type),
			Value:        choiceNames,
		},
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// CreateHandler create an empty responses
func (h *Handler) CreateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "CreateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := h.store.Create(traceCtx, formID, currentUser.ID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, CreateResponse{ID: response.ID.String()})
}

// ListSectionsHandler lists all sections of a response.
// Response model:
// {
//   "sections": [{ "id": "<uuid>", "title": "...", "progress": "DRAFT|SUBMITTED" }]
// }
func (h *Handler) ListSectionsHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ListSectionsHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	responseIDStr := r.PathValue("id")
	responseID, err := internal.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	sections, err := h.store.ListSections(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, ListSectionsResponse{
		Sections: sections,
	})
}

// OauthQuestionHandler handles OAuth flow for questions - validates question type and redirects to OAuth provider
func (h *Handler) OauthQuestionHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "OauthQuestionHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	responseIDStr := r.URL.Query().Get("responseId")
	if responseIDStr == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("responseId is required"), logger)
		return
	}
	if _, err := internal.ParseUUID(responseIDStr); err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIDStr := r.URL.Query().Get("questionId")
	if questionIDStr == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("questionId is required"), logger)
		return
	}
	questionID, err := internal.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	providerName := r.PathValue("provider")
	if providerName == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("provider is required"), logger)
		return
	}
	providerName = strings.ToLower(providerName)

	// Validate question type and provider ONCE before redirecting
	answerable, err := h.questionStore.GetByID(traceCtx, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	if answerable.Question().Type != question.QuestionTypeOauthConnect {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("question is not an OAuth question"), logger)
		return
	}

	oauthQuestion, ok := answerable.(question.OAuthConnect)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("question is not an OAuthConnect type"), logger)
		return
	}

	if string(oauthQuestion.Provider) != providerName {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("provider mismatch: question expects %s, got %s", oauthQuestion.Provider, providerName), logger)
		return
	}

	provider := h.provider[providerName]
	if provider == nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("provider not found: %s", providerName), logger)
		return
	}

	// Get optional redirect URL parameter for redirect mode
	redirectURL := r.URL.Query().Get("r")

	// Create state containing responseId|questionId|provider|redirectURL
	// Format: "responseId|questionId|provider|redirectURL" (redirectURL can be empty for popup mode)
	state := responseIDStr + "|" + questionIDStr + "|" + providerName + "|" + redirectURL

	authURL := provider.Config().AuthCodeURL(state, oauth2.AccessTypeOffline)

	logger.Info("Redirecting to OAuth provider for question",
		zap.String("provider", providerName),
		zap.String("questionId", questionIDStr),
		zap.String("responseId", responseIDStr),
		zap.String("redirectURL", redirectURL))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// OauthQuestionCallback handles OAuth callback - validates state and stores OAuth profile as answer
func (h *Handler) OauthQuestionCallback(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "OauthQuestionCallback")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Get provider from path
	providerName := r.PathValue("provider")

	// Parse and validate OAuth callback parameters
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	oauthError := r.URL.Query().Get("error")

	if oauthError != "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("OAuth error: %s", oauthError), logger)
		return
	}

	if code == "" {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("authorization code not found"), logger)
		return
	}

	// Parse state: format is "responseId|questionId|provider|redirectURL"
	stateParts := strings.Split(state, "|")
	if len(stateParts) < 3 || len(stateParts) > 4 {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("invalid state parameter"), logger)
		return
	}

	responseIDStr := stateParts[0]
	questionIDStr := stateParts[1]
	stateProvider := stateParts[2]
	var redirectURL string
	if len(stateParts) == 4 {
		redirectURL = stateParts[3]
	}

	// Verify provider matches
	if stateProvider != providerName {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("state parameter mismatch"), logger)
		return
	}

	// Get question ID and provider
	questionID, err := internal.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	provider := h.provider[providerName]
	if provider == nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("provider not found: %s", providerName), logger)
		return
	}

	// Get question to retrieve formID
	answerable, err := h.questionStore.GetByID(traceCtx, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Exchange code for token
	token, err := provider.Exchange(traceCtx, code)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to exchange token: %v", err), logger)
		return
	}

	// Get user info from OAuth provider
	userInfo, _, _, err := provider.GetUserInfo(traceCtx, token)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to get user info: %v", err), logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("user must be authenticated to answer OAuth questions"), logger)
		return
	}

	// Store OAuth profile data as answer
	oauthData := map[string]string{
		"avatar_url": userInfo.AvatarUrl.String,
		"username":   userInfo.Username.String,
	}

	oauthDataJSON, err := json.Marshal(oauthData)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to marshal OAuth data: %v", err), logger)
		return
	}

	answerParam := shared.AnswerParam{
		QuestionID: questionIDStr,
		Value:      json.RawMessage(oauthDataJSON),
	}

	answers := []shared.AnswerParam{answerParam}
	questionTypes := []QuestionType{QuestionType(question.QuestionTypeOauthConnect)}

	formResponse, err := h.store.CreateOrUpdate(traceCtx, answerable.FormID(), currentUser.ID, answers, questionTypes)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("failed to store OAuth answer: %v", err), logger)
		return
	}

	logger.Info("Successfully stored OAuth question answer",
		zap.String("user_id", currentUser.ID.String()),
		zap.String("question_id", questionIDStr),
		zap.String("response_id", responseIDStr),
		zap.String("provider", providerName),
		zap.String("form_response_id", formResponse.ID.String()))

	http.Redirect(w, r, redirectURL, http.StatusFound)
}
