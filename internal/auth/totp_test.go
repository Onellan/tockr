package auth

import (
	"net/url"
	"testing"
)

func TestTOTPURIEscapesLabelAndQuery(t *testing.T) {
	uri := TOTPURI("Tockr Ops", "owner+admin@example.com", "ABC123")

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse TOTP URI: %v", err)
	}
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" {
		t.Fatalf("unexpected TOTP URI prefix: %s", uri)
	}
	if got, want := parsed.EscapedPath(), "/Tockr%20Ops:owner+admin@example.com"; got != want {
		t.Fatalf("escaped path = %q, want %q", got, want)
	}

	query := parsed.Query()
	if got, want := query.Get("issuer"), "Tockr Ops"; got != want {
		t.Fatalf("issuer = %q, want %q", got, want)
	}
	if got, want := query.Get("secret"), "ABC123"; got != want {
		t.Fatalf("secret = %q, want %q", got, want)
	}
	if got, want := query.Get("period"), "30"; got != want {
		t.Fatalf("period = %q, want %q", got, want)
	}
	if got, want := query.Get("digits"), "6"; got != want {
		t.Fatalf("digits = %q, want %q", got, want)
	}
}
