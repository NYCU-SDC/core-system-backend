package email

import (
	"context"
	"os"
	"testing"

	"NYCU-SDC/core-system-backend/internal/form/response"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func TestSendMail(t *testing.T) {
	_ = godotenv.Load()
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if username == "" || password == "" || from == "" {
		t.Skip("SMTP environment variables are not configured")
	}

	sender := NewSMTPSender(
		"smtp.gmail.com",
		"587",
		username,
		password,
		from,
	)

	service := NewService(sender)

	err := service.SendSubmissionMail(
		context.Background(),
		os.Getenv("SMTP_USERNAME"),
		response.FormResponse{
			ID: uuid.New(),
		},
	)

	if err != nil {
		t.Fatal(err)
	}
}
