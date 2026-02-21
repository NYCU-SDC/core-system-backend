package answer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth/oauthprovider"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/shared"

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

// Payload represents a text-based answer (for most question types)
type Payload struct {
	QuestionID   string          `json:"questionId" validate:"required,uuid"`
	QuestionType string          `json:"questionType" validate:"required"`
	Value        json.RawMessage `json:"value" validate:"required"`
}

// AnswersRequest is the request body for updating answers
type AnswersRequest struct {
	Answers []Payload `json:"answers" validate:"required,dive"`
}

// Response is the response for getting a specific question's answer
type Response struct {
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	ResponseID   uuid.UUID `json:"responseId"`
	Payload      Payload   `json:"answer"`
	DisplayValue string    `json:"displayValue"`
}

// UpdateResponse is the response for updating answers
type UpdateResponse struct {
	Answers []Response `json:"answers"`
}

type Store interface {
	Get(ctx context.Context, formID, responseID, questionID uuid.UUID) (Answer, Answerable, error)
	Upsert(ctx context.Context, formID, responseID uuid.UUID, answers []shared.AnswerParam) ([]Answer, []Answerable, []error)
}

type QuestionGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type ResponseStore interface {
	GetFormIDByID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

// OAuthProvider is the minimal interface needed to initiate an OAuth flow.
type OAuthProvider interface {
	Name() string
	Config() *oauth2.Config
}

// JWTIssuer is the minimal interface needed to generate a form OAuth state token.
type JWTIssuer interface {
	NewFormState(ctx context.Context, callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string) (string, error)
	ParseFormState(ctx context.Context, tokenString string) (callbackURL string, responseID uuid.UUID, questionID uuid.UUID, redirectURL string, err error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	store         Store
	questionStore QuestionGetter
	responseStore ResponseStore
	jwtIssuer     JWTIssuer
	oauthProvider map[string]OAuthProvider
	baseURL       string
	tracer        trace.Tracer
}

func NewHandler(
	logger *zap.Logger,
	validator *validator.Validate,
	problemWriter *problem.HttpWriter,
	store Store,
	questionStore QuestionGetter,
	responseStore ResponseStore,
	jwtIssuer JWTIssuer,
	googleClientID, googleClientSecret string,
	githubClientID, githubClientSecret string,
	baseURL string,
) *Handler {
	getCallbackURL := func(provider string) string {
		return fmt.Sprintf("%s/api/oauth/callback/%s", baseURL, provider)
	}
	providers := map[string]OAuthProvider{
		"google": oauthprovider.NewGoogleConfig(googleClientID, googleClientSecret, getCallbackURL("google")),
		"github": oauthprovider.NewGitHubConfig(githubClientID, githubClientSecret, getCallbackURL("github")),
	}

	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		store:         store,
		questionStore: questionStore,
		responseStore: responseStore,
		jwtIssuer:     jwtIssuer,
		oauthProvider: providers,
		baseURL:       baseURL,
		tracer:        otel.Tracer("answer/handler"),
	}
}

// GetQuestionResponse gets a specific answer by question ID in the response
// GET /responses/{responseId}/questions/{questionId}
func (h *Handler) GetQuestionResponse(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "GetQuestionResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse responseId from path
	responseIDStr := r.PathValue("responseId")
	responseID, err := handlerutil.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse questionId from path
	questionIDStr := r.PathValue("questionId")
	questionID, err := handlerutil.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get formID from responseID
	formID, err := h.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get answer from store
	answer, answerable, err := h.store.Get(traceCtx, formID, responseID, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	response, err := h.ToResponse(traceCtx, answer, answerable, responseID)
	if err != nil {
		logger.Error("Failed to convert answer to response", zap.Error(err))
		span.RecordError(err)
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// UpdateFormResponse updates answers for the response
// PATCH /responses/{responseId}/answers
func (h *Handler) UpdateFormResponse(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateFormResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Parse responseId from path
	responseIDStr := r.PathValue("responseId")
	responseID, err := handlerutil.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Parse and validate request body
	var req AnswersRequest
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &req)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Get formID from responseID
	formID, err := h.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	answerParams := make([]shared.AnswerParam, 0, len(req.Answers))
	for _, answerRequest := range req.Answers {
		answerParams = append(answerParams, shared.AnswerParam{
			QuestionID: answerRequest.QuestionID,
			Value:      answerRequest.Value,
		})
	}

	// Upsert answers
	answers, answerableList, errs := h.store.Upsert(traceCtx, formID, responseID, answerParams)
	if len(errs) > 0 {
		errStrings := make([]string, 0, len(errs))
		for _, err := range errs {
			errStrings = append(errStrings, err.Error())
			span.RecordError(err)
		}

		err = handlerutil.NewValidationErrorWithErrors("validation errors occurred while upserting answers", errStrings)
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	responses := make([]Response, 0, len(answers))
	for i, answer := range answers {
		response, err := h.ToResponse(traceCtx, answer, answerableList[i], responseID)
		if err != nil {
			logger.Error("Failed to convert answer to response", zap.Error(err))
			span.RecordError(err)
			h.problemWriter.WriteError(traceCtx, w, err, logger)
			return
		}

		responses = append(responses, response)
	}

	response := UpdateResponse{
		Answers: responses,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, response)
}

// ConnectOAuthAccount initiates the OAuth flow for answering an oauth_connect question.
// GET /api/oauth/questions/{provider}?responseId=<uuid>&questionId=<uuid>&r=<optional_redirect>
func (h *Handler) ConnectOAuthAccount(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "ConnectOAuthAccount")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	// Resolve and validate provider from path
	providerName := strings.ToLower(r.PathValue("provider"))
	provider, ok := h.oauthProvider[providerName]
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %s", internal.ErrProviderNotFound, providerName), logger)
		return
	}

	// Parse query parameters
	responseIDStr := r.URL.Query().Get("responseId")
	responseID, err := handlerutil.ParseUUID(responseIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	questionIDStr := r.URL.Query().Get("questionId")
	questionID, err := handlerutil.ParseUUID(questionIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	redirectURL := r.URL.Query().Get("r")

	// Validate that the response exists
	_, err = h.responseStore.GetFormIDByID(traceCtx, responseID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	// Validate that the question exists, is an oauth_connect type, and matches the provider
	answerable, err := h.questionStore.GetByID(traceCtx, questionID)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	q := answerable.Question()
	if q.Type != question.QuestionTypeOauthConnect {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: question %s is not an oauth_connect question", internal.ErrNotFound, questionID), logger)
		return
	}

	oauthProvider, parseErr := question.ExtractOauthConnect(q.Metadata)
	if parseErr != nil {
		logger.Error("failed to extract oauth provider from question metadata", zap.String("questionID", questionID.String()), zap.Error(parseErr))
		span.RecordError(parseErr)
		h.problemWriter.WriteError(traceCtx, w, internal.ErrInternalServerError, logger)
		return
	}

	if string(oauthProvider) != providerName {
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: question %s requires provider %s, got %s", internal.ErrProviderNotFound, questionID, oauthProvider, providerName), logger)
		return
	}

	// Build the callback URL for this form OAuth flow
	callbackURL := fmt.Sprintf("%s/api/oauth/callback/%s", h.baseURL, providerName)

	// Generate a signed state JWT carrying the form context
	state, err := h.jwtIssuer.NewFormState(traceCtx, callbackURL, responseID, questionID, redirectURL)
	if err != nil {
		logger.Error("failed to generate form OAuth state", zap.Error(err))
		span.RecordError(err)
		h.problemWriter.WriteError(traceCtx, w, fmt.Errorf("%w: %v", internal.ErrNewStateFailed, err), logger)
		return
	}

	// Redirect to the OAuth provider's authorization page
	authURL := provider.Config().AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) ToResponse(context context.Context, answer Answer, answerable Answerable, responseID uuid.UUID) (Response, error) {
	traceCtx, span := h.tracer.Start(context, "ToResponse")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	questionID := answerable.Question().ID

	displayValue, err := answerable.DisplayValue(answer.Value)
	if err != nil {
		return Response{}, err
	}

	valueStruct, err := answerable.DecodeStorage(answer.Value)
	if err != nil {
		logger.Error("failed to decode answer value from storage", zap.String("questionID", questionID.String()), zap.Error(err))
		span.RecordError(err)
		return Response{}, err
	}

	payload, err := answerable.EncodeRequest(valueStruct)
	if err != nil {
		logger.Error("failed to encode answer value for response", zap.String("questionID", questionID.String()), zap.Error(err))
		span.RecordError(err)
		return Response{}, err
	}

	return Response{
		CreatedAt:    answer.CreatedAt.Time,
		UpdatedAt:    answer.UpdatedAt.Time,
		ResponseID:   responseID,
		DisplayValue: displayValue,
		Payload: Payload{
			QuestionID:   questionID.String(),
			QuestionType: strings.ToUpper(string(answerable.Question().Type)),
			Value:        payload,
		},
	}, nil
}
