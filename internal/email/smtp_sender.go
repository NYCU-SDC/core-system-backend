package email

import (
	"context"
	"fmt"
	"net/smtp"
)

type SMTPSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPSender(
	host string,
	port string,
	username string,
	password string,
	from string,
) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (s *SMTPSender) Send(
	ctx context.Context,
	to string,
	subject string,
	body string,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	address := s.host + ":" + s.port

	auth := smtp.PlainAuth(
		"",
		s.username,
		s.password,
		s.host,
	)

	message := []byte(
		"From: " + s.from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body,
	)

	if err := smtp.SendMail(
		address,
		auth,
		s.from,
		[]string{to},
		message,
	); err != nil {
		return fmt.Errorf("smtp send mail: %w", err)
	}

	return nil
}
