package form

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// visibilityFromAPIFormat converts API visibility format (uppercase) to database format (lowercase).
func visibilityFromAPIFormat(v string) Visibility {
	switch v {
	case "PUBLIC":
		return VisibilityPublic
	case "PRIVATE":
		return VisibilityPrivate
	default:
		// Fallback for backward compatibility
		return Visibility(v)
	}
}

type formFields struct {
	title                  string
	descriptionJSON        []byte
	descriptionHTML        string
	previewMessage         pgtype.Text
	deadline               pgtype.Timestamptz
	publishTime            pgtype.Timestamptz
	googleSheetURL         pgtype.Text
	messageAfterSubmission string
	visibility             Visibility
	dressingColor          pgtype.Text
	dressingHeaderFont     pgtype.Text
	dressingQuestionFont   pgtype.Text
	dressingTextFont       pgtype.Text
	allowEditResponse      bool
}

func buildFormFieldsFromRequest(ctx context.Context, markdownStore MarkdownStore, request Request) (formFields, error) {
	form := formFields{}

	if request.Deadline != nil {
		form.deadline = pgtype.Timestamptz{Time: *request.Deadline, Valid: true}
	} else {
		form.deadline = pgtype.Timestamptz{Valid: false}
	}

	if request.PublishTime != nil {
		form.publishTime = pgtype.Timestamptz{Time: *request.PublishTime, Valid: true}
	} else {
		form.publishTime = pgtype.Timestamptz{Valid: false}
	}

	descJSON, descHTML, err := markdownStore.ProcessAPIText(ctx, []byte(request.Description))
	if err != nil {
		return formFields{}, err
	}
	form.descriptionJSON = descJSON
	form.descriptionHTML = descHTML

	preview := request.PreviewMessage
	if preview == "" {
		snip, err := markdownStore.PreviewSnippet(ctx, descJSON, 25)
		if err != nil {
			return formFields{}, err
		}
		preview = snip
	}

	if d := request.Dressing; d != nil {
		form.dressingColor = nonEmptyText(d.Color)
		form.dressingHeaderFont = nonEmptyText(d.HeaderFont)
		form.dressingQuestionFont = nonEmptyText(d.QuestionFont)
		form.dressingTextFont = nonEmptyText(d.TextFont)
	}

	form.previewMessage = pgtype.Text{String: preview, Valid: preview != ""}
	form.googleSheetURL = pgtype.Text{String: request.GoogleSheetURL, Valid: request.GoogleSheetURL != ""}
	form.messageAfterSubmission = request.MessageAfterSubmission
	form.visibility = visibilityFromAPIFormat(request.Visibility)
	form.title = request.Title
	form.allowEditResponse = request.AllowEditResponse

	return form, nil
}
