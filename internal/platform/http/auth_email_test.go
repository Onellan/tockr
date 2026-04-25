package httpserver

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"tockr/internal/domain"
	"tockr/internal/platform/config"
)

func TestEmailSettingsPageAndTestDelivery(t *testing.T) {
	smtp := startTestSMTP(t)
	app, store := testAppWithConfig(t, config.Config{
		SMTPHost:     smtp.host,
		SMTPPort:     smtp.port,
		SMTPFrom:     "Tockr <noreply@example.com>",
		SMTPStartTLS: false,
		PublicURL:    "https://tockr.example.test",
	})
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")

	body := getWithCookie(app, "/admin/email", cookie).Body.String()
	for _, expected := range []string{"Email", "TOCKR_SMTP_HOST", smtp.host, "TOCKR_SMTP_FROM", "Ready", "TOCKR_PUBLIC_URL"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("email settings page missing %q", expected)
		}
	}
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/admin/email", cookie, url.Values{
		"csrf":             {csrf},
		"notify_old_email": {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save email settings returned %d", rec.Code)
	}
	rec = postFormWithCookie(app, "/admin/email/test", cookie, url.Values{
		"csrf": {csrf},
		"to":   {"tester@example.com"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("SMTP test returned %d", rec.Code)
	}
	msg := smtp.next(t)
	if !strings.Contains(msg, "Tockr SMTP test") || !strings.Contains(msg, "tester@example.com") {
		t.Fatalf("unexpected test email: %s", msg)
	}
}

func TestEmailChangeRequestValidationAndOTPFlow(t *testing.T) {
	smtp := startTestSMTP(t)
	app, store := testAppWithConfig(t, config.Config{SMTPHost: smtp.host, SMTPPort: smtp.port, SMTPFrom: "noreply@example.com", SMTPStartTLS: false})
	defer store.Close()
	ctx := context.Background()
	if err := store.CreateUser(ctx, domain.User{Email: "taken@example.com", Username: "taken", DisplayName: "Taken", Timezone: "UTC", Enabled: true}, "taken12345", []domain.Role{domain.RoleUser}); err != nil {
		t.Fatal(err)
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	if !strings.Contains(body, `action="/account/email"`) {
		t.Fatal("account page missing email change form")
	}
	csrf := csrfFromBody(t, body)
	rec := postFormWithCookie(app, "/account/email", cookie, url.Values{"csrf": {csrf}, "new_email": {"bad-email"}})
	if rec.Header().Get("Location") != "/account" {
		t.Fatalf("invalid email location = %s", rec.Header().Get("Location"))
	}
	if body := getWithCookies(app, "/account", withCookies(cookie, rec.Result().Cookies())...).Body.String(); !strings.Contains(body, "Enter a valid email address") {
		t.Fatal("invalid email should show account flash")
	}
	rec = postFormWithCookie(app, "/account/email", cookie, url.Values{"csrf": {csrf}, "new_email": {"taken@example.com"}})
	if rec.Header().Get("Location") != "/account" {
		t.Fatalf("duplicate email location = %s", rec.Header().Get("Location"))
	}
	if body := getWithCookies(app, "/account", withCookies(cookie, rec.Result().Cookies())...).Body.String(); !strings.Contains(body, "That email is already in use") {
		t.Fatal("duplicate email should show account flash")
	}
	rec = postFormWithCookie(app, "/account/email", cookie, url.Values{"csrf": {csrf}, "new_email": {"new-admin@example.com"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/account/email/verify" {
		t.Fatalf("email change request returned %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	code := firstCode(t, smtp.next(t))
	if loginCookie(t, app, "admin@example.com", "admin12345") == nil {
		t.Fatal("old email should still log in before OTP verification")
	}
	verifyBody := getWithCookies(app, "/account/email/verify", withCookies(cookie, rec.Result().Cookies())...).Body.String()
	if !strings.Contains(verifyBody, "new-admin@example.com") {
		t.Fatal("verify page missing pending email")
	}
	if !strings.Contains(verifyBody, "Verification code sent") {
		t.Fatal("verify page missing sent-code flash")
	}
	csrf = csrfFromBody(t, verifyBody)
	rec = postFormWithCookie(app, "/account/email/verify", cookie, url.Values{"csrf": {csrf}, "code": {"000000"}})
	if rec.Header().Get("Location") != "/account/email/verify" {
		t.Fatalf("wrong OTP location = %s", rec.Header().Get("Location"))
	}
	if body := getWithCookies(app, "/account/email/verify", withCookies(cookie, rec.Result().Cookies())...).Body.String(); !strings.Contains(body, "Verification code is invalid or expired") {
		t.Fatal("wrong OTP should show verify flash")
	}
	rec = postFormWithCookie(app, "/account/email/verify", cookie, url.Values{"csrf": {csrf}, "code": {code}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/account" {
		t.Fatalf("OTP verify returned %d location=%s", rec.Code, rec.Header().Get("Location"))
	}
	notification := smtp.next(t)
	if !strings.Contains(notification, "new-admin@example.com") || !strings.Contains(notification, "admin@example.com") {
		t.Fatalf("old email notification not sent as expected: %s", notification)
	}
	changed, _ := store.FindUserByEmail(ctx, "new-admin@example.com")
	if changed == nil {
		t.Fatal("active email was not changed")
	}
	rec = postFormWithCookie(app, "/account/email/verify", cookie, url.Values{"csrf": {csrf}, "code": {code}})
	if rec.Header().Get("Location") != "/account/email/verify" {
		t.Fatal("OTP should be single-use")
	}
	if loginCookie(t, app, "new-admin@example.com", "admin12345") == nil {
		t.Fatal("new email should log in")
	}
}

func TestEmailChangeOTPExpiry(t *testing.T) {
	smtp := startTestSMTP(t)
	app, store := testAppWithConfig(t, config.Config{SMTPHost: smtp.host, SMTPPort: smtp.port, SMTPFrom: "noreply@example.com", SMTPStartTLS: false})
	defer store.Close()
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/account", cookie).Body.String()
	csrf := csrfFromBody(t, body)
	postFormWithCookie(app, "/account/email", cookie, url.Values{"csrf": {csrf}, "new_email": {"expired@example.com"}})
	code := firstCode(t, smtp.next(t))
	if _, err := store.DB().ExecContext(context.Background(), `UPDATE email_change_otps SET expires_at=?`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	verifyBody := getWithCookie(app, "/account/email/verify", cookie).Body.String()
	csrf = csrfFromBody(t, verifyBody)
	rec := postFormWithCookie(app, "/account/email/verify", cookie, url.Values{"csrf": {csrf}, "code": {code}})
	if rec.Header().Get("Location") != "/account/email/verify" {
		t.Fatalf("expired OTP location = %s", rec.Header().Get("Location"))
	}
	if body := getWithCookies(app, "/account/email/verify", withCookies(cookie, rec.Result().Cookies())...).Body.String(); !strings.Contains(body, "Verification code is invalid or expired") {
		t.Fatal("expired OTP should show verify flash")
	}
	if user, _ := store.FindUserByEmail(context.Background(), "expired@example.com"); user != nil {
		t.Fatal("expired OTP should not change active email")
	}
}

func TestPasswordResetGenericResponseSuccessExpiryAndReuse(t *testing.T) {
	smtp := startTestSMTP(t)
	app, store := testAppWithConfig(t, config.Config{SMTPHost: smtp.host, SMTPPort: smtp.port, SMTPFrom: "noreply@example.com", SMTPStartTLS: false, PublicURL: "https://tockr.example.test"})
	defer store.Close()

	for _, email := range []string{"missing@example.com", "admin@example.com"} {
		rec := postPublicForm(app, "/forgot-password", url.Values{"email": {email}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/forgot-password" {
			t.Fatalf("forgot response for %s was %d %s", email, rec.Code, rec.Header().Get("Location"))
		}
		if body := getPublic(app, "/forgot-password", rec.Result().Cookies()...).Body.String(); !strings.Contains(body, "If that email exists") {
			t.Fatalf("forgot flash missing for %s", email)
		}
	}
	token := firstResetToken(t, smtp.next(t))

	if _, err := store.DB().ExecContext(context.Background(), `UPDATE password_reset_tokens SET expires_at=? WHERE token_hash IS NOT NULL`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	rec := postPublicForm(app, "/reset-password", url.Values{"token": {token}, "password": {"resetpass99"}, "confirm": {"resetpass99"}})
	if rec.Header().Get("Location") != "/reset-password" {
		t.Fatalf("expired reset location = %s", rec.Header().Get("Location"))
	}
	if body := getPublic(app, "/reset-password", rec.Result().Cookies()...).Body.String(); !strings.Contains(body, "Reset link is invalid or expired") {
		t.Fatal("expired reset should show flash")
	}

	postPublicForm(app, "/forgot-password", url.Values{"email": {"admin@example.com"}})
	token = firstResetToken(t, smtp.next(t))
	rec = postPublicForm(app, "/reset-password", url.Values{"token": {token}, "password": {"resetpass99"}, "confirm": {"resetpass99"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("reset success returned %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if body := getPublic(app, "/login", rec.Result().Cookies()...).Body.String(); !strings.Contains(body, "Password updated") {
		t.Fatal("reset success should show login flash")
	}
	rec = postPublicForm(app, "/reset-password", url.Values{"token": {token}, "password": {"againpass99"}, "confirm": {"againpass99"}})
	if rec.Header().Get("Location") != "/reset-password" {
		t.Fatal("reset token should be single-use")
	}
	if loginCookie(t, app, "admin@example.com", "resetpass99") == nil {
		t.Fatal("new password should log in")
	}
}

func TestAuthEmailPagesRenderAndMisconfiguredSMTPIsClear(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()
	if rec := getPublic(app, "/forgot-password"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Forgot password") {
		t.Fatalf("forgot page returned %d", rec.Code)
	}
	if rec := getPublic(app, "/reset-password?token=abc"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Reset password") {
		t.Fatalf("reset page returned %d", rec.Code)
	}
	rec := postPublicForm(app, "/forgot-password", url.Values{"email": {"admin@example.com"}})
	if rec.Header().Get("Location") != "/forgot-password" {
		t.Fatalf("misconfigured forgot location = %s", rec.Header().Get("Location"))
	}
	if body := getPublic(app, "/forgot-password", rec.Result().Cookies()...).Body.String(); !strings.Contains(body, "Password reset email is not configured") {
		t.Fatal("misconfigured forgot should show flash")
	}
	cookie := loginCookie(t, app, "admin@example.com", "admin12345")
	body := getWithCookie(app, "/admin/email", cookie).Body.String()
	if !strings.Contains(body, "Needs configuration") || !strings.Contains(body, "TOCKR_SMTP_HOST") {
		t.Fatal("admin email settings should make env-backed SMTP status clear")
	}
	body = getWithCookie(app, "/account/email/verify", cookie).Body.String()
	if !strings.Contains(body, "No pending email change") {
		t.Fatal("verify page should render without a pending change")
	}
}

func TestPublicAuthPagesRedirectTrailingSlash(t *testing.T) {
	app, store := testApp(t)
	defer store.Close()

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/login/", "/login"},
		{"/forgot-password/", "/forgot-password"},
		{"/reset-password/?token=abc", "/reset-password?token=abc"},
	} {
		rec := getPublic(app, tc.path)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != tc.want {
			t.Fatalf("%s returned %d location %q, want %q", tc.path, rec.Code, rec.Header().Get("Location"), tc.want)
		}
	}
}

func postPublicForm(app *Server, target string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func getPublic(app *Server, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, cookie := range cookies {
		if cookie != nil {
			req.AddCookie(cookie)
		}
	}
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func firstCode(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(body)
	if match == "" {
		t.Fatalf("no OTP code in email: %s", body)
	}
	return match
}

func firstResetToken(t *testing.T, body string) string {
	t.Helper()
	re := regexp.MustCompile(`token=([a-f0-9]+)`)
	match := re.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("no reset token in email: %s", body)
	}
	return match[1]
}

type testSMTP struct {
	host     string
	port     int
	listener net.Listener
	messages chan string
}

func startTestSMTP(t *testing.T) *testSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portText)
	server := &testSMTP{host: "127.0.0.1", port: port, listener: ln, messages: make(chan string, 20)}
	go server.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return server
}

func (s *testSMTP) next(t *testing.T) string {
	t.Helper()
	select {
	case msg := <-s.messages:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
		return ""
	}
}

func (s *testSMTP) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *testSMTP) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 test smtp")
	var rcpt string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250 test smtp")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			write("250 ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpt = cmd
			write("250 ok")
		case upper == "DATA":
			write("354 end with dot")
			var data strings.Builder
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(line) == "." {
					break
				}
				data.WriteString(line)
			}
			s.messages <- rcpt + "\n" + data.String()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}
