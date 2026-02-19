package submit

import (
	"NYCU-SDC/core-system-backend/internal/form/answer"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"context"
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
	Answers []answer.Payload `json:"answers" validate:"required,dive"`
}

type Response struct {
	ID        string    `json:"id"`
	FormID    string    `json:"formId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Progress  string    `json:"progress"`
}

type Operator interface {
	Submit(ctx context.Context, responseID uuid.UUID, answers []shared.AnswerParam) (response.FormResponse, []error)
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

	responseIDStr := r.PathValue("responseId")
	responseID, err := handlerutil.ParseUUID(responseIDStr)
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

	answerParams := make([]shared.AnswerParam, 0, len(request.Answers))
	for _, answerRequest := range request.Answers {
		answerParams = append(answerParams, shared.AnswerParam{
			QuestionID: answerRequest.QuestionID,
			Value:      answerRequest.Value,
		})
	}

	newResponse, errs := h.operator.Submit(traceCtx, responseID, answerParams)
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
		Progress:  strings.ToUpper(string(newResponse.Progress)),
	}

	handlerutil.WriteJSONResponse(w, http.StatusCreated, submitResponse)
}
