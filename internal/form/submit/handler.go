package submit

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/shared"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	handlerutil "github.com/NYCU-SDC/summer/pkg/handler"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/problem"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type Request struct {
	Answers []AnswerRequest `json:"answers" validate:"required,dive"`
}

type AnswerRequest struct {
	QuestionID string `json:"questionId" validate:"required,uuid"`
	Value      string `json:"value" validate:"required"`
}

func (a AnswerRequest) ToAnswerParam() shared.AnswerParam {
	return shared.AnswerParam{
		QuestionID: a.QuestionID,
		Value:      a.Value,
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
}

type Handler struct {
	logger        *zap.Logger
	validator     *validator.Validate
	problemWriter *problem.HttpWriter
	operator      Operator
	questionStore QuestionStore
	tracer        trace.Tracer
}

func NewHandler(logger *zap.Logger, validator *validator.Validate, problemWriter *problem.HttpWriter, operator Operator, questionStore QuestionStore) *Handler {
	return &Handler{
		logger:        logger,
		validator:     validator,
		problemWriter: problemWriter,
		operator:      operator,
		questionStore: questionStore,
		tracer:        otel.Tracer("response/handler"),
	}
}

// SubmitHandler submits a response to a form
func (h *Handler) SubmitHandler(w http.ResponseWriter, r *http.Request) {
	traceCtx, span := h.tracer.Start(r.Context(), "SubmitFormHandler")
	defer span.End()
	logger := logutil.WithContext(traceCtx, h.logger)

	formIdStr := r.PathValue("formId")
	formId, err := internal.ParseUUID(formIdStr)
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

	newResponse, errs := h.operator.Submit(traceCtx, formId, currentUser.ID, answerParams)
	if errs != nil {
		h.problemWriter.WriteError(traceCtx, w, err, logger)
		return
	}

	submitResponse := Response{
		ID:        newResponse.ID.String(),
		FormID:    newResponse.FormID.String(),
		CreatedAt: newResponse.CreatedAt.Time,
		UpdatedAt: newResponse.UpdatedAt.Time,
	}

	handlerutil.WriteJSONResponse(w, http.StatusOK, submitResponse)
}
