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
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type SMTPConfig struct {
	Host       string
	Port       int
	Encryption string
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	TLS        bool
}

const (
	EncryptionNone     = "none"
	EncryptionSTARTTLS = "starttls"
	EncryptionSSLTLS   = "ssl_tls"
)

func normalizeEncryption(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", EncryptionSTARTTLS:
		return EncryptionSTARTTLS
	case EncryptionNone:
		return EncryptionNone
	case EncryptionSSLTLS:
		return EncryptionSSLTLS
	default:
		return ""
	}
}

func (c SMTPConfig) encryptionMode() string {
	if mode := normalizeEncryption(c.Encryption); mode != "" {
		return mode
	}
	if c.TLS {
		return EncryptionSTARTTLS
	}
	return EncryptionNone
}

func (c SMTPConfig) FromAddress() string {
	email := strings.TrimSpace(c.FromEmail)
	name := strings.TrimSpace(c.FromName)
	if email == "" {
		return ""
	}
	if name == "" {
		return email
	}
	return (&mail.Address{Name: name, Address: email}).String()
}

type Sender struct {
	cfg SMTPConfig
}

func NewSender(cfg SMTPConfig) Sender {
	return Sender{cfg: cfg}
}

func (s Sender) Configured() bool {
	return strings.TrimSpace(s.cfg.Host) != "" && strings.TrimSpace(s.cfg.FromEmail) != ""
}

func (s Sender) Validate() error {
	if strings.TrimSpace(s.cfg.Host) == "" {
		return errors.New("SMTP host is required")
	}
	if strings.ContainsAny(s.cfg.Host, " \t\n\r") {
		return errors.New("SMTP host must not contain whitespace")
	}
	if s.cfg.Port <= 0 {
		return errors.New("SMTP port must be positive")
	}
	if s.cfg.encryptionMode() == "" {
		return errors.New("SMTP encryption must be one of none, starttls, or ssl_tls")
	}
	if _, err := mail.ParseAddress(s.cfg.FromAddress()); err != nil {
		return errors.New("SMTP from address must be valid")
	}
	if (s.cfg.Username == "") != (s.cfg.Password == "") {
		return errors.New("SMTP username and password must be set together")
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
	from, err := mail.ParseAddress(s.cfg.FromAddress())
	if err != nil {
		return err
	}
	encryption := s.cfg.encryptionMode()
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	var c *smtp.Client
	if encryption == EncryptionSSLTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return err
		}
	} else {
		c, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
		if encryption == EncryptionSTARTTLS {
			if ok, _ := c.Extension("STARTTLS"); !ok {
				_ = c.Close()
				return errors.New("SMTP server does not support STARTTLS")
			}
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = c.Close()
				return err
			}
		}
	}
	defer c.Close()
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
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
