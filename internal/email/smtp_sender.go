package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const defaultSMTPTimeout = 10 * time.Second

type SMTPSender struct {
	host     string
	port     string
	username string
	password string
	from     string
	timeout  time.Duration
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
		timeout:  defaultSMTPTimeout,
	}
}

func (s *SMTPSender) Send(
	ctx context.Context,
	to string,
	subject string,
	body string,
) error {
	err := ctx.Err()
	if err != nil {
		return err
	}

	err = validateHeader("from", s.from)
	if err != nil {
		return err
	}
	err = validateHeader("to", to)
	if err != nil {
		return err
	}
	err = validateHeader("subject", subject)
	if err != nil {
		return err
	}

	address := net.JoinHostPort(s.host, s.port)

	dialer := net.Dialer{
		Timeout: s.timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	deadline := time.Now().Add(s.timeout)
	ctxDeadline, ok := ctx.Deadline()
	if ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	err = conn.SetDeadline(deadline)
	if err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	tlsConfig := &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
	}

	err = client.StartTLS(tlsConfig)
	if err != nil {
		return fmt.Errorf("smtp start TLS: %w", err)
	}

	auth := smtp.PlainAuth(
		"",
		s.username,
		s.password,
		s.host,
	)

	err = client.Auth(auth)
	if err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	err = client.Mail(s.from)
	if err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}

	err = client.Rcpt(to)
	if err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}

	encodedSubject := mime.QEncoding.Encode("UTF-8", subject)

	message := []byte(
		"From: " + s.from + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + encodedSubject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body + "\r\n",
	)

	_, err = writer.Write(message)
	if err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("close smtp message: %w", err)
	}

	err = client.Quit()
	if err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}

	return nil
}

func validateHeader(name, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains invalid newline characters", name)
	}
	return nil
}
