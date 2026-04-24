package email

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"

	"tockr/internal/platform/config"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender struct {
	cfg config.Config
}

func NewSender(cfg config.Config) Sender {
	return Sender{cfg: cfg}
}

func (s Sender) Configured() bool {
	return strings.TrimSpace(s.cfg.SMTPHost) != "" && strings.TrimSpace(s.cfg.SMTPFrom) != ""
}

func (s Sender) Validate() error {
	if strings.TrimSpace(s.cfg.SMTPHost) == "" {
		return errors.New("TOCKR_SMTP_HOST is required")
	}
	if s.cfg.SMTPPort <= 0 {
		return errors.New("TOCKR_SMTP_PORT must be positive")
	}
	if _, err := mail.ParseAddress(s.cfg.SMTPFrom); err != nil {
		return errors.New("TOCKR_SMTP_FROM must be a valid email address")
	}
	if (s.cfg.SMTPUsername == "") != (s.cfg.SMTPPassword == "") {
		return errors.New("TOCKR_SMTP_USERNAME and TOCKR_SMTP_PASSWORD must be set together")
	}
	return nil
}

func (s Sender) Send(message Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return fmt.Errorf("recipient email is invalid: %w", err)
	}
	from, err := mail.ParseAddress(s.cfg.SMTPFrom)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.SMTPHost, fmt.Sprint(s.cfg.SMTPPort))
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if s.cfg.SMTPStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.cfg.SMTPUsername != "" {
		auth := smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(formatMessage(from.String(), to.String(), message.Subject, message.Text)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func formatMessage(from, to, subject, text string) []byte {
	var b bytes.Buffer
	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version": "1.0",
		"Content-Type": `text/plain; charset="utf-8"`,
	}
	for _, key := range []string{"From", "To", "Subject", "MIME-Version", "Content-Type"} {
		_, _ = fmt.Fprintf(&b, "%s: %s\r\n", key, headers[key])
	}
	_, _ = fmt.Fprint(&b, "\r\n")
	_, _ = fmt.Fprint(&b, strings.ReplaceAll(text, "\n", "\r\n"))
	return b.Bytes()
}
