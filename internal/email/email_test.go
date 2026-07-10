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
	err := godotenv.Load()
	if err != nil {
		t.Fatal(err)
	}
	sender := NewSMTPSender(
		"smtp.gmail.com",
		"587",
		os.Getenv("SMTP_USERNAME"),
		os.Getenv("SMTP_PASSWORD"),
		os.Getenv("SMTP_FROM"),
	)

	if os.Getenv("SMTP_USERNAME") == "" {
		t.Fatal("SMTP_USERNAME is empty")
	}
	
	service := NewService(sender)

	err = service.SendSubmissionMail(
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
