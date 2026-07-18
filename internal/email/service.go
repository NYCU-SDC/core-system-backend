package email

import (
	"NYCU-SDC/core-system-backend/internal/form/response"
	"context"
	"fmt"
)

type Sender interface {
	Send(
		ctx context.Context,
		to string,
		subject string,
		body string,
	) error
}

type Service struct {
	sender Sender
}

func NewService(sender Sender) *Service {
	return &Service{
		sender: sender,
	}
}

func (s *Service) SendSubmissionMail(
	ctx context.Context,
	to string,
	formResponse response.FormResponse,
) error {
	if to == "" {
		return fmt.Errorf("recipient email is empty")
	}

	subject := "感謝您完成表單填寫"

	body := fmt.Sprintf(
		`您好：

感謝您完成表單填寫，我們已成功收到您的回覆。

回覆編號：%s
提交時間：%s

此信件由系統自動寄出，請勿直接回覆。`,
		formResponse.ID.String(),
		formResponse.UpdatedAt.Time.Format("2006-01-02 15:04:05"),
	)

	if err := s.sender.Send(ctx, to, subject, body); err != nil {
		return fmt.Errorf("send submission thank-you email: %w", err)
	}

	return nil
}
