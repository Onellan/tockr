package config

import "testing"

func TestSMTPConfigLoading(t *testing.T) {
	t.Setenv("TOCKR_PUBLIC_URL", "https://tockr.example.test")
	t.Setenv("TOCKR_SMTP_HOST", "smtp.example.test")
	t.Setenv("TOCKR_SMTP_PORT", "2525")
	t.Setenv("TOCKR_SMTP_USERNAME", "smtp-user")
	t.Setenv("TOCKR_SMTP_PASSWORD", "smtp-pass")
	t.Setenv("TOCKR_SMTP_FROM", "Tockr <noreply@example.test>")
	t.Setenv("TOCKR_SMTP_STARTTLS", "false")

	cfg := Load()
	if cfg.PublicURL != "https://tockr.example.test" ||
		cfg.SMTPHost != "smtp.example.test" ||
		cfg.SMTPPort != 2525 ||
		cfg.SMTPUsername != "smtp-user" ||
		cfg.SMTPPassword != "smtp-pass" ||
		cfg.SMTPFrom != "Tockr <noreply@example.test>" ||
		cfg.SMTPStartTLS {
		t.Fatalf("unexpected SMTP config: %#v", cfg)
	}
}
