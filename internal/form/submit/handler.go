package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Request struct {
	Answers []AnswerRequest `json:"answers" validate:"required,dive"`
}

type AnswerRequest struct {
	QuestionID   string `json:"questionId" validate:"required,uuid"`
	QuestionType string `json:"questionType" validate:"required"`
	Value        string `json:"value" validate:"required"`
}

type UpdateAnswerRequest struct {
	Answers []json.RawMessage `json:"answers" validate:"required"`
}

// BaseAnswer contains common fields for all answer types
type BaseAnswer struct {
	QuestionID   string `json:"questionId"`
	QuestionType string `json:"questionType"`
}

func (a AnswerRequest) ToAnswerParam() shared.AnswerParam {
	return shared.AnswerParam{
		QuestionID: a.QuestionID,
		Value:      []byte(a.Value),
	}
}

type Response struct {
	ID        string    `json:"id" validate:"required,uuid"`
	FormID    string    `json:"formId" validate:"required,uuid"`
	CreatedAt time.Time `json:"createdAt" validate:"required,datetime"`
	UpdatedAt time.Time `json:"updatedAt" validate:"required,datetime"`
}

type Operator interface {
	Submit(ctx context.Context, formID uuid.UUID, userID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error)
	Update(ctx context.Context, userID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error)
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	operator      Operator
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, operator Operator) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		operator:      operator,
		tracer:        otel.Tracer("response/handler"),
	}
}

// SubmitHandler submits a response to a form
func (h *Handler) SubmitHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "SubmitHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIDStr := r.PathValue("formId")
	formID, err := internal.ParseUUID(formIDStr)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	var request Request
	err = handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	answerParams := make([]shared.AnswerParam, len(request.Answers))
	for i, answer := range request.Answers {
		answerParams[i] = answer.ToAnswerParam()
	}

	newResponse, errs := h.operator.Submit(traceCtx, formID, currentUser.ID, answerParams)
	if errs != nil {
		// Convert errors to strings and join them for better error handling
		errorStrings := make([]string, len(errs))
		for i, err := range errs {
			errorStrings[i] = err.Error()
		}
		combinedErr := errors.New("form submission failed: [" + strings.Join(errorStrings, "; ") + "]")
		h.problemWriter.WriteError(traceCtx, w, combinedErr, logger)
		return
	}

	submitResponse := Response{
		ID:        newResponse.ID.String(),
		FormID:    newResponse.FormID.String(),
		CreatedAt: newResponse.CreatedAt.Time,
		UpdatedAt: newResponse.UpdatedAt.Time,
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, submitResponse)
}

// UpdateHandler updates the answer for response
func (h *Handler) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "UpdateHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	var request UpdateAnswerRequest
	err := handlerutil.ParseAndValidateRequestBody(traceCtx, h.validator, r, &request)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	currentUser, ok := user.GetFromContext(traceCtx)
	if !ok {
		h.problemWriter.WriteError(traceCtx, w, internal.ErrNoUserInContext, logger)
		return
	}

	// Parse and convert answers based on their types
	answerParams, err := parseAnswers(request.Answers, h.validator)
	if err != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	_, errs := h.operator.Update(traceCtx, currentUser.ID, answerParams)

	handlerutil.WriteJSONResponse(w, http.StatusCreated, errs)
}

// parseAnswers converts raw JSON answers into AnswerParams based on question types
func parseAnswers(rawAnswers []json.RawMessage, validator *validator.Validate) ([]shared.AnswerParam, error) {
	answerParams := make([]shared.AnswerParam, 0, len(rawAnswers))

	for _, rawAnswer := range rawAnswers {
		// First, parse to get questionType
		var base BaseAnswer
		if err := json.Unmarshal(rawAnswer, &base); err != nil {
			return nil, internal.ErrValidationFailed
		}

		// Validate based on questionType and convert to proper format
		var valueJSON json.RawMessage
		switch base.QuestionType {
		case "LINEAR_SCALE", "RATING":
			var scaleAnswer shared.ScaleAnswerJSON
			if err := json.Unmarshal(rawAnswer, &scaleAnswer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			if err := validator.Struct(scaleAnswer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			valueJSON, _ = json.Marshal(scaleAnswer.Value)

		case "OAUTH_CONNECT":
			var oauthAnswer shared.OauthAnswerJSON
			if err := json.Unmarshal(rawAnswer, &oauthAnswer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			if err := validator.Struct(oauthAnswer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			oauthValue := map[string]string{
				"avatarUrl": oauthAnswer.AvatarURL,
				"username":  oauthAnswer.Username,
			}
			valueJSON, _ = json.Marshal(oauthValue)

		default:
			var answer shared.AnswerJSON
			if err := json.Unmarshal(rawAnswer, &answer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			if err := validator.Struct(answer); err != nil {
				return nil, internal.ErrValidationFailed
			}
			valueJSON, _ = json.Marshal(answer.Value)
		}

		answerParams = append(answerParams, shared.AnswerParam{
			QuestionID: base.QuestionID,
			Value:      valueJSON,
		})
	}

	return answerParams, nil
}
