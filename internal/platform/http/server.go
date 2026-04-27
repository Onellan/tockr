package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/skip2/go-qrcode"

	"tockr/internal/auth"
	"tockr/internal/db/sqlite"
	"tockr/internal/domain"
	"tockr/internal/platform/config"
	emailer "tockr/internal/platform/email"
	templates "tockr/web/templates"
)

type Server struct {
	cfg              config.Config
	store            *sqlite.Store
	log              *slog.Logger
	router           chi.Router
	rateLimitMu      sync.Mutex
	rateLimitBuckets map[string]rateLimitBucket
}

type rateLimitBucket struct {
	Count       int
	WindowStart time.Time
	LastSeen    time.Time
}

type requestState struct {
	User    *domain.User
	Session *sqlite.Session
	Access  domain.AccessContext
}

type flashMessage struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type contextKey string

const stateKey contextKey = "state"
const flashCookieName = "tockr_flash"
const defaultProjectWorkstreamName = "Project management"

func New(cfg config.Config, store *sqlite.Store, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, store: store, log: log, rateLimitBuckets: map[string]rateLimitBucket{}}
	if err := s.bootstrapWorkspaceSMTPFromGlobal(context.Background()); err != nil {
		s.log.Warn("workspace smtp bootstrap failed", "err", err)
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(s.securityHeadersMiddleware)
	r.Use(s.rateLimitMiddleware)
	r.Use(s.sessionMiddleware)
	r.Use(s.csrfMiddleware)
	r.Get("/favicon.ico", s.iconAsset("favicon.ico"))
	r.Get("/favicon-16x16.png", s.iconAsset("favicon-16x16.png"))
	r.Get("/favicon-32x32.png", s.iconAsset("favicon-32x32.png"))
	r.Get("/apple-touch-icon.png", s.iconAsset("apple-touch-icon.png"))
	r.Get("/site.webmanifest", s.iconAsset("site.webmanifest"))
	r.Get("/static/*", s.static)
	r.Get("/healthz", s.health)
	r.Get("/login", s.loginPage)
	r.Get("/login/", redirectCanonical("/login"))
	r.Post("/login", s.login)
	r.Get("/forgot-password", s.forgotPasswordPage)
	r.Get("/forgot-password/", redirectCanonical("/forgot-password"))
	r.Post("/forgot-password", s.forgotPassword)
	r.Get("/reset-password", s.resetPasswordPage)
	r.Get("/reset-password/", redirectCanonical("/reset-password"))
	r.Post("/reset-password", s.resetPassword)
	r.Get("/login/otp", s.loginOTPPage)
	r.Post("/login/otp", s.loginOTP)
	r.Get("/reports/share/{token}", s.viewSharedReport)
	r.Post("/logout", s.requireLogin(s.logout))
	r.Group(func(r chi.Router) {
		r.Use(s.requireLoginMiddleware)
		r.Get("/", s.dashboard)
		r.Get("/admin", s.adminHome)
		r.Get("/admin/two-factor", s.adminTwoFactor)
		r.Get("/account", s.account)
		r.Post("/account", s.updateAccount)
		r.Post("/account/email", s.requestEmailChange)
		r.Get("/account/email/verify", s.verifyEmailPage)
		r.Post("/account/email/verify", s.verifyEmailChange)
		r.Post("/account/password", s.updatePassword)
		r.Post("/account/totp/enable", s.enableTOTP)
		r.Post("/account/totp/disable", s.disableTOTP)
		r.Post("/account/email-otp/enable", s.enableEmailOTP)
		r.Post("/account/email-otp/disable", s.disableEmailOTP)
		r.Post("/workspace", s.switchWorkspace)
		r.Get("/customers", s.customers)
		r.Post("/customers", s.requirePermission(auth.PermManageMaster, s.saveCustomer))
		r.Post("/customers/{id}", s.requirePermission(auth.PermManageMaster, s.saveCustomer))
		r.Get("/projects", s.projects)
		r.Get("/projects/create", s.requirePermission(auth.PermManageMaster, s.projectCreateWizardPage))
		r.Post("/projects/create", s.requirePermission(auth.PermManageMaster, s.projectCreateWizardSubmit))
		r.Get("/projects/{id}/edit", s.requirePermission(auth.PermManageMaster, s.editProject))
		r.Post("/projects", s.requirePermission(auth.PermManageMaster, s.saveProject))
		r.Post("/projects/{id}", s.requirePermission(auth.PermManageMaster, s.saveProject))
		r.Get("/project-dashboards", s.requirePermission(auth.PermManageProjects, s.projectDashboards))
		r.Get("/projects/{id}/dashboard", s.projectDashboard)
		r.Get("/projects/{id}/members", s.projectMembers)
		r.Post("/projects/{id}/members", s.projectMemberSave)
		r.Post("/projects/{id}/members/remove", s.projectMemberRemove)
		r.Post("/projects/{id}/groups", s.projectGroupSave)
		r.Post("/projects/{id}/groups/remove", s.projectGroupRemove)
		r.Get("/activities", s.activities)
		r.Post("/activities", s.requirePermission(auth.PermManageMaster, s.saveActivity))
		r.Post("/activities/{id}", s.requirePermission(auth.PermManageMaster, s.saveActivity))
		r.Get("/workstreams", s.requirePermission(auth.PermManageMaster, s.workstreams))
		r.Post("/workstreams", s.requirePermission(auth.PermManageMaster, s.saveWorkstream))
		r.Post("/workstreams/{id}", s.requirePermission(auth.PermManageMaster, s.saveWorkstream))
		r.Post("/workstreams/{id}/delete", s.requirePermission(auth.PermManageMaster, s.deleteWorkstream))
		r.Get("/projects/{id}/workstreams", s.requirePermission(auth.PermManageProjects, s.projectWorkstreams))
		r.Post("/projects/{id}/workstreams", s.requirePermission(auth.PermManageProjects, s.saveProjectWorkstream))
		r.Post("/projects/{id}/workstreams/{wsid}/remove", s.requirePermission(auth.PermManageProjects, s.removeProjectWorkstream))
		r.Get("/tasks", s.tasks)
		r.Post("/tasks", s.requirePermission(auth.PermManageMaster, s.saveTask))
		r.Post("/tasks/{id}", s.requirePermission(auth.PermManageMaster, s.saveTask))
		r.Post("/tasks/{id}/archive", s.requirePermission(auth.PermManageMaster, s.archiveTask))
		r.Get("/tags", s.tags)
		r.Post("/tags", s.requirePermission(auth.PermTrackTime, s.saveTag))
		r.Get("/groups", s.requirePermission(auth.PermManageGroups, s.groups))
		r.Post("/groups", s.requirePermission(auth.PermManageGroups, s.saveGroup))
		r.Post("/groups/{id}", s.requirePermission(auth.PermManageGroups, s.saveGroup))
		r.Get("/groups/{id}/members", s.requirePermission(auth.PermManageGroups, s.groupMembers))
		r.Post("/groups/{id}/members", s.requirePermission(auth.PermManageGroups, s.groupMemberSave))
		r.Post("/groups/{id}/members/remove", s.requirePermission(auth.PermManageGroups, s.groupMemberRemove))
		r.Get("/rates", s.requirePermission(auth.PermManageRates, s.rates))
		r.Post("/rates", s.requirePermission(auth.PermManageRates, s.saveRate))
		r.Post("/rates/costs", s.requirePermission(auth.PermManageRates, s.saveUserCostRate))
		r.Get("/timesheets", s.timesheets)
		r.Get("/calendar", s.calendar)
		r.Get("/timesheets/{id}/edit", s.editTimesheet)
		r.Post("/timesheets", s.saveTimesheet)
		r.Post("/timesheets/{id}", s.updateTimesheet)
		r.Post("/timesheets/start", s.startTimer)
		r.Post("/timesheets/stop", s.stopTimer)
		r.Get("/admin/schedule", s.requirePermission(auth.PermManageOrg, s.workScheduleSettings))
		r.Post("/admin/schedule", s.requirePermission(auth.PermManageOrg, s.saveWorkScheduleSettings))
		r.Get("/admin/email", s.requirePermission(auth.PermManageOrg, s.emailSettings))
		r.Post("/admin/email", s.requirePermission(auth.PermManageOrg, s.saveEmailSettings))
		r.Post("/admin/email/test", s.requirePermission(auth.PermManageOrg, s.testEmailSettings))
		r.Get("/admin/demo-data", s.requirePermission(auth.PermManageOrg, s.adminDemoData))
		r.Post("/admin/demo-data/add", s.requirePermission(auth.PermManageOrg, s.adminDemoDataAdd))
		r.Post("/admin/demo-data/remove", s.requirePermission(auth.PermManageOrg, s.adminDemoDataRemove))
		r.Get("/reports", s.requirePermission(auth.PermViewReports, s.reports))
		r.Post("/reports/saved", s.requirePermission(auth.PermViewReports, s.saveReport))
		r.Post("/reports/saved/{id}", s.requirePermission(auth.PermViewReports, s.editSavedReport))
		r.Post("/reports/saved/{id}/delete", s.requirePermission(auth.PermViewReports, s.deleteSavedReport))
		r.Post("/reports/saved/{id}/share", s.requirePermission(auth.PermViewReports, s.shareSavedReport))
		r.Get("/reports/utilization", s.requirePermission(auth.PermViewReports, s.utilization))
		r.Get("/reports/export", s.requirePermission(auth.PermViewReports, s.exportReports))
		r.Get("/timesheets/export", s.exportTimesheets)
		r.Get("/admin/exchange-rates", s.requirePermission(auth.PermManageRates, s.exchangeRates))
		r.Post("/admin/exchange-rates", s.requirePermission(auth.PermManageRates, s.saveExchangeRate))
		r.Post("/admin/exchange-rates/{id}/delete", s.requirePermission(auth.PermManageRates, s.deleteExchangeRate))
		r.Get("/admin/recalculate", s.requirePermission(auth.PermManageRates, s.recalculate))
		r.Post("/admin/recalculate", s.requirePermission(auth.PermManageRates, s.applyRecalculate))
		r.Get("/invoices", s.requirePermission(auth.PermManageInvoices, s.invoices))
		r.Post("/invoices", s.requirePermission(auth.PermManageInvoices, s.createInvoice))
		r.Get("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.webhooks))
		r.Post("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.createWebhook))
		r.Get("/project-templates", s.requirePermission(auth.PermManageProjects, s.projectTemplates))
		r.Post("/project-templates", s.requirePermission(auth.PermManageProjects, s.saveProjectTemplate))
		r.Post("/project-templates/use", s.requirePermission(auth.PermManageProjects, s.useProjectTemplate))
		r.Get("/project-templates/{id}", s.requirePermission(auth.PermManageProjects, s.projectTemplateDetail))
		r.Post("/project-templates/{id}", s.requirePermission(auth.PermManageProjects, s.saveProjectTemplate))
		r.Post("/project-templates/{id}/use", s.requirePermission(auth.PermManageProjects, s.useProjectTemplate))
		r.Get("/admin/workspaces", s.requirePermission(auth.PermManageOrg, s.workspacesAdmin))
		r.Post("/admin/workspaces", s.requirePermission(auth.PermManageOrg, s.saveWorkspace))
		r.Get("/admin/workspaces/{id}", s.requirePermission(auth.PermManageOrg, s.workspaceAdminDetail))
		r.Post("/admin/workspaces/{id}", s.requirePermission(auth.PermManageOrg, s.saveWorkspace))
		r.Post("/admin/workspaces/{id}/smtp/test", s.requirePermission(auth.PermManageOrg, s.workspaceSMTPTest))
		r.Post("/admin/workspaces/{id}/members", s.requirePermission(auth.PermManageOrg, s.workspaceMemberSave))
		r.Post("/admin/workspaces/{id}/members/remove", s.requirePermission(auth.PermManageOrg, s.workspaceMemberRemove))
		r.Get("/admin/users", s.requirePermission(auth.PermManageUsers, s.users))
		r.Post("/admin/users", s.requirePermission(auth.PermManageUsers, s.createUser))
		r.Post("/admin/users/{id}", s.requirePermission(auth.PermManageUsers, s.createUser))
	})
	r.Route("/api", func(r chi.Router) {
		r.Use(s.requireLoginMiddleware)
		r.Use(s.requirePermissionMiddleware(auth.PermUseAPI))
		r.Get("/status", s.apiStatus)
		r.Get("/customers", s.apiCustomers)
		r.Get("/projects", s.apiProjects)
		r.Get("/activities", s.apiActivities)
		r.Get("/tasks", s.apiTasks)
		r.Get("/timesheets", s.apiTimesheets)
		r.Post("/timer/start", s.startTimer)
		r.Post("/timer/stop", s.stopTimer)
		r.Get("/invoices/{id}/download", s.requirePermission(auth.PermManageInvoices, s.apiInvoiceDownload))
		r.Patch("/invoices/{id}/meta", s.requirePermission(auth.PermManageInvoices, s.apiInvoiceMeta))
		r.Get("/webhooks", s.requirePermission(auth.PermManageWebhooks, s.apiWebhooks))
	})
	s.router = r
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func redirectCanonical(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	setStaticHeaders(w, path)
	http.ServeFile(w, r, staticFile(path))
}

func (s *Server) iconAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setStaticHeaders(w, name)
		http.ServeFile(w, r, staticFile(name))
	}
}

func staticFile(name string) string {
	clean := filepath.Clean(strings.TrimPrefix(name, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") {
		return filepath.Join("web", "static", "__missing__")
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Join("web", "static", clean)
	}
	for {
		candidate := filepath.Join(wd, "web", "static", clean)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return filepath.Join("web", "static", clean)
}

func setStaticHeaders(w http.ResponseWriter, path string) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	switch {
	case strings.HasSuffix(path, ".webmanifest"):
		w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	case strings.HasSuffix(path, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	case strings.HasSuffix(path, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.Login(s.popFlash(w, r)))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	email := normalizeEmail(r.FormValue("email"))
	password := r.FormValue("password")
	user, err := s.store.FindUserByEmail(r.Context(), email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user == nil || !auth.CheckPassword(user.PasswordHash, password) || !user.Enabled {
		var userID *int64
		if user != nil {
			id := user.ID
			userID = &id
		}
		s.store.Audit(r.Context(), userID, "failed_login", "user", userID, "")
		s.redirectWithFlash(w, r, "/login", "error", "Invalid credentials")
		return
	}
	if s.totpAvailable() && user.TOTPEnabled {
		code := strings.TrimSpace(r.FormValue("totp"))
		valid := auth.VerifyTOTP(user.TOTPSecret, code, time.Now().UTC())
		if !valid {
			var err error
			valid, err = s.store.UseRecoveryCode(r.Context(), user.ID, code)
			if err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		if !valid {
			s.redirectWithFlash(w, r, "/login", "error", "Two-factor code required")
			return
		}
	}
	// Email OTP challenge: redirect to /login/otp if email 2FA is enabled and TOTP is not
	sender, err := s.senderForUser(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user.EmailOTPEnabled && !user.TOTPEnabled && sender.Configured() {
		code := numericCode(6)
		token, err := s.store.CreateLoginOTP(r.Context(), user.ID, code, 10*time.Minute)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if err := sender.Send(emailer.Message{
			To:      user.Email,
			Subject: "Your Tockr sign-in code",
			Text:    fmt.Sprintf("Your Tockr sign-in code is %s.\n\nIt expires in 10 minutes and can be used once.", code),
		}); err != nil {
			s.serverError(w, r, err)
			return
		}
		// #nosec G124
		http.SetCookie(w, &http.Cookie{
			Name:     "tockr_login_intent",
			Value:    token,
			Path:     "/login/otp",
			MaxAge:   600,
			HttpOnly: true,
			Secure:   s.cfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		s.store.Audit(r.Context(), &user.ID, "otp_sent", "login", &user.ID, "email")
		http.Redirect(w, r, "/login/otp", http.StatusSeeOther)
		return
	}
	session, err := s.store.CreateSession(r.Context(), user.ID, 0, 14*24*time.Hour)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.TouchLogin(r.Context(), user.ID)
	s.store.Audit(r.Context(), &user.ID, "login", "user", &user.ID, "")
	http.SetCookie(w, s.cookie(session.ID))
	if s.totpRequired() && !user.TOTPEnabled {
		s.redirectWithFlash(w, r, "/account", "warning", "Two-factor setup is required")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) forgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.ForgotPassword(s.popFlash(w, r)))
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	generic := "If that email exists, a reset link has been sent."
	email := strings.TrimSpace(r.FormValue("email"))
	user, err := s.store.FindUserByEmail(r.Context(), email)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user != nil && user.Enabled {
		sender, err := s.senderForUser(r.Context(), user.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if err := sender.Validate(); err != nil {
			s.redirectWithFlash(w, r, "/forgot-password", "error", "Password reset email is not configured. Contact an administrator.")
			return
		}
		token := randomToken(32)
		if err := s.store.CreatePasswordResetToken(r.Context(), user.ID, token, 30*time.Minute); err != nil {
			s.serverError(w, r, err)
			return
		}
		link := s.absoluteURL(r, "/reset-password?token="+url.QueryEscape(token))
		if err := sender.Send(emailer.Message{
			To:      user.Email,
			Subject: "Reset your Tockr password",
			Text:    fmt.Sprintf("Use this link to reset your Tockr password. It expires in 30 minutes and can be used once.\n\n%s\n\nIf you did not request this, you can ignore this email.", link),
		}); err != nil {
			s.serverError(w, r, err)
			return
		}
		s.store.Audit(r.Context(), &user.ID, "request", "password_reset", &user.ID, "")
	}
	s.redirectWithFlash(w, r, "/forgot-password", "info", generic)
}

func (s *Server) resetPasswordPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.ResetPassword(r.URL.Query().Get("token"), s.popFlash(w, r)))
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	password := r.FormValue("password")
	if len(password) < 8 || password != r.FormValue("confirm") {
		s.redirectWithFlash(w, r, "/reset-password?token="+url.QueryEscape(token), "error", "Password confirmation does not match")
		return
	}
	userID, ok, err := s.store.ResetPasswordWithToken(r.Context(), token, password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		s.redirectWithFlash(w, r, "/reset-password", "error", "Reset link is invalid or expired")
		return
	}
	s.store.Audit(r.Context(), &userID, "reset", "account", &userID, "password")
	s.redirectWithFlash(w, r, "/login", "success", "Password updated")
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	if state.Session != nil {
		_ = s.store.DeleteSession(r.Context(), state.Session.ID)
	}
	// #nosec G124
	http.SetCookie(w, &http.Cookie{Name: "tockr_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	s.render(w, r, templates.Account(s.nav(r), *state.User, s.popFlash(w, r)))
}

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request) {
	if !s.hasAnyAdminAccess(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.render(w, r, templates.AdminHome(s.nav(r)))
}

func (s *Server) adminTwoFactor(w http.ResponseWriter, r *http.Request) {
	if !s.hasAnyAdminAccess(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	state := s.state(r)
	secret := ""
	uri := ""
	if s.totpAvailable() && !state.User.TOTPEnabled {
		secret = auth.NewTOTPSecret()
		uri = auth.TOTPURI("Tockr", state.User.Email, secret)
	}
	qrDataURI := ""
	if uri != "" {
		if png, err := qrcode.Encode(uri, qrcode.Medium, 200); err == nil {
			qrDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}
	sender, err := s.senderForWorkspace(r.Context(), state.Access.WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	emailConfigured := sender.Configured()
	s.render(w, r, templates.AdminTwoFactor(s.nav(r), *state.User, s.cfg.TOTPMode, emailConfigured, secret, uri, qrDataURI, nil, s.popFlash(w, r)))
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := s.state(r).User.ID
	if err := s.store.UpdateProfile(r.Context(), userID, r.FormValue("display_name"), r.FormValue("timezone")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "update", "account", &userID, "profile")
	s.redirectWithFlash(w, r, "/account", "success", "Profile updated")
}

func (s *Server) requestEmailChange(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	state := s.state(r)
	newEmail := normalizeEmail(r.FormValue("new_email"))
	if _, err := mail.ParseAddress(newEmail); err != nil || !strings.Contains(newEmail, "@") {
		s.redirectWithFlash(w, r, "/account", "error", "Enter a valid email address")
		return
	}
	if strings.EqualFold(newEmail, state.User.Email) {
		s.redirectWithFlash(w, r, "/account", "info", "That email is already on your account")
		return
	}
	existing, err := s.store.FindUserByEmail(r.Context(), newEmail)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if existing != nil {
		s.redirectWithFlash(w, r, "/account", "error", "That email is already in use")
		return
	}
	sender, err := s.senderForWorkspace(r.Context(), state.Access.WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := sender.Validate(); err != nil {
		s.redirectWithFlash(w, r, "/account", "error", "Email sending is not configured")
		return
	}
	code := numericCode(6)
	if err := s.store.CreateEmailChangeOTP(r.Context(), state.User.ID, newEmail, code, 10*time.Minute); err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := sender.Send(emailer.Message{
		To:      newEmail,
		Subject: "Verify your Tockr email address",
		Text:    fmt.Sprintf("Your Tockr email change code is %s.\n\nIt expires in 10 minutes and can be used once.", code),
	}); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &state.User.ID, "request", "account_email", &state.User.ID, newEmail)
	s.redirectWithFlash(w, r, "/account/email/verify", "success", "Verification code sent")
}

func (s *Server) verifyEmailPage(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	pending, expires, err := s.store.PendingEmailChange(r.Context(), state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.VerifyEmail(s.nav(r), pending, expires, s.popFlash(w, r)))
}

func (s *Server) verifyEmailChange(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	state := s.state(r)
	oldEmail, newEmail, ok, err := s.store.UseEmailChangeOTP(r.Context(), state.User.ID, r.FormValue("code"), 5)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		s.redirectWithFlash(w, r, "/account/email/verify", "error", "Verification code is invalid or expired")
		return
	}
	settings := s.store.EmailSettings(r.Context())
	if settings.NotifyOldEmailOnChange && oldEmail != "" {
		if sender, err := s.senderForWorkspace(r.Context(), state.Access.WorkspaceID); err == nil {
			_ = sender.Send(emailer.Message{
				To:      oldEmail,
				Subject: "Your Tockr email address was changed",
				Text:    fmt.Sprintf("Your Tockr account email address was changed to %s.\n\nIf this was not you, contact an administrator immediately.", newEmail),
			})
		}
	}
	s.store.Audit(r.Context(), &state.User.ID, "verify", "account_email", &state.User.ID, oldEmail+" -> "+newEmail)
	s.redirectWithFlash(w, r, "/account", "success", "Email address updated")
}

func (s *Server) updatePassword(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	password := r.FormValue("password")
	if len(password) < 8 || password != r.FormValue("confirm") {
		s.redirectWithFlash(w, r, "/account", "error", "Password confirmation does not match")
		return
	}
	user := s.state(r).User
	if !auth.CheckPassword(user.PasswordHash, r.FormValue("current_password")) {
		s.redirectWithFlash(w, r, "/account", "error", "Current password is incorrect")
		return
	}
	userID := user.ID
	if err := s.store.UpdatePassword(r.Context(), userID, password); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "update", "account", &userID, "password")
	s.redirectWithFlash(w, r, "/account", "success", "Password updated")
}

func (s *Server) enableTOTP(w http.ResponseWriter, r *http.Request) {
	if !s.totpAvailable() {
		http.Error(w, "totp disabled", http.StatusForbidden)
		return
	}
	_ = r.ParseForm()
	secret := strings.TrimSpace(r.FormValue("secret"))
	if !auth.VerifyTOTP(secret, r.FormValue("code"), time.Now().UTC()) {
		s.redirectWithFlash(w, r, "/admin/two-factor", "error", "Invalid two-factor code")
		return
	}
	codes := auth.NewRecoveryCodes(8)
	userID := s.state(r).User.ID
	if err := s.store.EnableTOTP(r.Context(), userID, secret, codes); err != nil {
		s.serverError(w, r, err)
		return
	}
	user, _ := s.store.FindUserByID(r.Context(), userID)
	s.store.Audit(r.Context(), &userID, "enable", "totp", &userID, "")
	nav := s.nav(r)
	if user != nil {
		sender, _ := s.senderForWorkspace(r.Context(), s.access(r).WorkspaceID)
		s.render(w, r, templates.AdminTwoFactor(nav, *user, s.cfg.TOTPMode, sender.Configured(), "", "", "", codes, templates.Notice{Kind: "success", Message: "Two-factor authentication enabled"}))
		return
	}
	s.redirectWithFlash(w, r, "/admin/two-factor", "success", "Two-factor authentication enabled")
}

func (s *Server) disableTOTP(w http.ResponseWriter, r *http.Request) {
	if s.totpRequired() {
		s.redirectWithFlash(w, r, "/admin/two-factor", "warning", "Two-factor is required for this deployment")
		return
	}
	userID := s.state(r).User.ID
	if err := s.store.DisableTOTP(r.Context(), userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "disable", "totp", &userID, "")
	s.redirectWithFlash(w, r, "/admin/two-factor", "success", "Two-factor authentication disabled")
}

func (s *Server) loginOTPPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.LoginOTPChallenge(s.popFlash(w, r)))
}

func (s *Server) loginOTP(w http.ResponseWriter, r *http.Request) {
	intentCookie, err := r.Cookie("tockr_login_intent")
	if err != nil || intentCookie.Value == "" {
		s.redirectWithFlash(w, r, "/login", "error", "Session expired. Please log in again.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	userID, ok, err := s.store.UseLoginOTP(r.Context(), intentCookie.Value, strings.TrimSpace(r.FormValue("code")), 5)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	// Clear the intent cookie
	// #nosec G124
	http.SetCookie(w, &http.Cookie{
		Name:     "tockr_login_intent",
		Value:    "",
		Path:     "/login/otp",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	if !ok {
		s.redirectWithFlash(w, r, "/login/otp", "error", "Invalid or expired code")
		return
	}
	session, err := s.store.CreateSession(r.Context(), userID, 0, 14*24*time.Hour)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.store.TouchLogin(r.Context(), userID)
	s.store.Audit(r.Context(), &userID, "login", "user", &userID, "email_otp")
	http.SetCookie(w, s.cookie(session.ID))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) enableEmailOTP(w http.ResponseWriter, r *http.Request) {
	sender, err := s.senderForWorkspace(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !sender.Configured() {
		s.redirectWithFlash(w, r, "/admin/two-factor", "error", "Email is not configured on this deployment")
		return
	}
	userID := s.state(r).User.ID
	if err := s.store.EnableEmailOTP(r.Context(), userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "enable", "email_otp", &userID, "")
	s.redirectWithFlash(w, r, "/admin/two-factor", "success", "Email sign-in codes enabled")
}

func (s *Server) disableEmailOTP(w http.ResponseWriter, r *http.Request) {
	if s.totpRequired() {
		s.redirectWithFlash(w, r, "/admin/two-factor", "warning", "Two-factor is required for this deployment")
		return
	}
	userID := s.state(r).User.ID
	if err := s.store.DisableEmailOTP(r.Context(), userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.store.Audit(r.Context(), &userID, "disable", "email_otp", &userID, "")
	s.redirectWithFlash(w, r, "/admin/two-factor", "success", "Email sign-in codes disabled")
}

func (s *Server) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	workspaceID := formInt(r, "workspace_id")
	state := s.state(r)
	ok, err := s.store.UserCanAccessWorkspace(r.Context(), state.User.ID, workspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.store.SwitchSessionWorkspace(r.Context(), state.Session.ID, workspaceID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, safeReturn(r, "/"), http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	state := s.state(r)
	summary, err := s.store.Dashboard(r.Context(), state.Access)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	active, err := s.store.ActiveTimer(r.Context(), state.User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Dashboard(s.nav(r), summary, active, selectors))
}

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListCustomers(r.Context(), s.access(r), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, c := range items {
		actions := ""
		if s.hasPermission(r, auth.PermManageMaster) {
			actions, err = inlineEditAction(r.Context(), templates.CustomerForm(s.nav(r), &c))
			if err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		rows = append(rows, []string{html.EscapeString(c.Name), html.EscapeString(c.Company), html.EscapeString(c.Email), html.EscapeString(c.Currency), html.EscapeString(boolText(c.Visible)), html.EscapeString(boolText(c.Billable)), actions})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.CustomerForm(s.nav(r), nil)
	}
	s.render(w, r, templates.EntityListRaw("Clients", s.nav(r), []string{"Client", "Company", "Email", "Billing unit", "Visible", "Billable", "Actions"}, rows, form))
}

func (s *Server) saveCustomer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	c := &domain.Customer{ID: pathID(r), WorkspaceID: s.access(r).WorkspaceID, Name: r.FormValue("name"), Number: r.FormValue("number"), Company: r.FormValue("company"), Contact: r.FormValue("contact"), Email: r.FormValue("email"), Currency: r.FormValue("currency"), Timezone: r.FormValue("timezone"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertCustomer(r.Context(), c); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	action := "create"
	if pathID(r) != 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "customer", &c.ID, c.Name)
	http.Redirect(w, r, "/customers", http.StatusSeeOther)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListProjects(r.Context(), s.access(r), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, p := range items {
		actions := `<a class="table-action" href="/projects/` + fmt.Sprint(p.ID) + `/dashboard">Dashboard</a>`
		if s.hasPermission(r, auth.PermManageMaster) {
			actions += ` <a class="table-action" href="/projects/` + fmt.Sprint(p.ID) + `/edit">Edit</a>`
		}
		if s.access(r).ManagesProject(p.ID) {
			actions += templates.RowActionMenu(fmt.Sprintf("project-%d-actions", p.ID), "Project actions", s.nav(r).CSRF, []templates.MenuAction{
				{Label: "Members", Href: fmt.Sprintf("/projects/%d/members", p.ID)},
				{Label: "Workstreams", Href: fmt.Sprintf("/projects/%d/workstreams", p.ID)},
			})
		}
		estimateStatus := "Open-ended"
		if p.EstimateSeconds > 0 {
			estimateStatus = fmt.Sprintf("%dh planned", p.EstimateSeconds/3600)
		}
		budgetStatus := "No fixed fee"
		if p.BudgetCents > 0 {
			budgetStatus = templates.Money(p.BudgetCents)
		}
		flags := []string{}
		if p.Private {
			flags = append(flags, "Private")
		}
		if p.Billable {
			flags = append(flags, "Billable")
		} else {
			flags = append(flags, "Internal")
		}
		if !p.Visible {
			flags = append(flags, "Archived")
		}
		rows = append(rows, []string{html.EscapeString(p.Name), html.EscapeString(labelValue(selectors.CustomerLabels, p.CustomerID)), html.EscapeString(p.Number), html.EscapeString(strings.Join(flags, " · ")), html.EscapeString(estimateStatus), html.EscapeString(budgetStatus), `<div class="row-actions">` + actions + `</div>`})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ProjectCreateEntryCard(s.nav(r))
	}
	s.render(w, r, templates.EntityListRaw("Projects", s.nav(r), []string{"Project", "Client", "Code", "Status", "Estimate", "Budget", "Actions"}, rows, form))
}

func (s *Server) editProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.store.Project(r.Context(), pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.EditProject(s.nav(r), selectors, *project))
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := pathID(r)
	customerID := formInt(r, "customer_id")
	if !s.customerInScope(r, customerID) {
		http.Error(w, "invalid customer", http.StatusForbidden)
		return
	}
	p := &domain.Project{ID: projectID, WorkspaceID: s.access(r).WorkspaceID, CustomerID: customerID, Name: r.FormValue("name"), Number: r.FormValue("number"), OrderNo: r.FormValue("order_number"), Visible: checkbox(r, "visible"), Private: checkbox(r, "private"), Billable: checkbox(r, "billable"), EstimateSeconds: formInt(r, "estimate_hours") * 3600, BudgetCents: formIntAny(r, "budget", "budget_cents"), BudgetAlertPercent: formInt(r, "budget_alert_percent"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertProject(r.Context(), p); err != nil {
		s.serverError(w, r, err)
		return
	}
	if projectID == 0 {
		if err := s.ensureDefaultProjectWorkstream(r.Context(), p); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	uid := s.state(r).User.ID
	action := "create"
	if projectID != 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "project", &p.ID, p.Name)
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func (s *Server) ensureDefaultProjectWorkstream(ctx context.Context, project *domain.Project) error {
	workstreams, err := s.store.ListWorkstreams(ctx, project.WorkspaceID)
	if err != nil {
		return err
	}
	defaultWorkstreamID := int64(0)
	for _, ws := range workstreams {
		if strings.EqualFold(strings.TrimSpace(ws.Name), defaultProjectWorkstreamName) {
			defaultWorkstreamID = ws.ID
			break
		}
	}
	if defaultWorkstreamID == 0 {
		ws := &domain.Workstream{WorkspaceID: project.WorkspaceID, Name: defaultProjectWorkstreamName, Visible: true}
		if err := s.store.UpsertWorkstream(ctx, ws); err != nil {
			return err
		}
		defaultWorkstreamID = ws.ID
	}
	return s.store.UpsertProjectWorkstream(ctx, &domain.ProjectWorkstream{ProjectID: project.ID, WorkstreamID: defaultWorkstreamID, BudgetCents: 0, Active: true})
}

func (s *Server) projectDashboards(w http.ResponseWriter, r *http.Request) {
	selectData, err := s.selectorData(r, true, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectedProjectID := int64Param(r, "project_id")
	filter := projectDashboardFilterFromRequest(r)
	var dashboard *domain.ProjectDashboard
	if selectedProjectID > 0 {
		item, err := s.store.ProjectDashboard(r.Context(), s.access(r), selectedProjectID, filter)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			s.serverError(w, r, err)
			return
		}
		dashboard = &item
	}
	s.render(w, r, templates.ProjectDashboards(s.nav(r), selectData.Projects, selectedProjectID, dashboard, selectData))
}

func (s *Server) projectDashboard(w http.ResponseWriter, r *http.Request) {
	selectData, err := s.selectorData(r, true, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	dashboard, err := s.store.ProjectDashboard(r.Context(), s.access(r), pathID(r), projectDashboardFilterFromRequest(r))
	if err != nil || dashboard.Project.ID == 0 {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, templates.ProjectDashboard(s.nav(r), dashboard, selectData))
}

func (s *Server) projectMembers(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	members, err := s.store.ListProjectMembers(r.Context(), project.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	assignedGroups, err := s.store.ListProjectGroups(r.Context(), project.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.store.ListWorkspaceUsers(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	groups, err := s.store.ListGroups(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectMembers(s.nav(r), *project, members, users, assignedGroups, groups))
}

func (s *Server) projectMemberSave(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	role := domain.ProjectRole(r.FormValue("role"))
	if role != domain.ProjectRoleManager {
		role = domain.ProjectRoleMember
	}
	userIDs := formIntList(r, "user_id")
	if len(userIDs) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
		return
	}
	for _, userID := range userIDs {
		if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
			s.serverError(w, r, err)
			return
		} else if !ok {
			http.Error(w, "user is not in workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddProjectMember(r.Context(), project.ID, userID, role); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_members", &project.ID, fmt.Sprintf("users=%v role=%s", userIDs, role))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectMemberRemove(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userIDs := formIntList(r, "user_id")
	for _, userID := range userIDs {
		if err := s.store.RemoveProjectMember(r.Context(), project.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_members", &project.ID, fmt.Sprintf("remove_users=%v", userIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectGroupSave(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	groupIDs := formIntList(r, "group_id")
	for _, groupID := range groupIDs {
		if group, err := s.store.Group(r.Context(), groupID); err != nil {
			s.serverError(w, r, err)
			return
		} else if group == nil || group.WorkspaceID != s.access(r).WorkspaceID {
			http.Error(w, "group is outside workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddGroupToProject(r.Context(), project.ID, groupID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_groups", &project.ID, fmt.Sprintf("groups=%v", groupIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) projectGroupRemove(w http.ResponseWriter, r *http.Request) {
	project, ok := s.authorizedProjectManagement(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	groupIDs := formIntList(r, "group_id")
	for _, groupID := range groupIDs {
		if err := s.store.RemoveGroupFromProject(r.Context(), project.ID, groupID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_groups", &project.ID, fmt.Sprintf("remove_groups=%v", groupIDs))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/members", project.ID), http.StatusSeeOther)
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListTasks(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	canManage := s.hasPermission(r, auth.PermManageMaster)
	s.render(w, r, templates.Tasks(s.nav(r), items, selectors, canManage))
}

func (s *Server) saveTask(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	if !s.canTrackProject(r, projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	task := &domain.Task{
		ID:              pathID(r),
		WorkspaceID:     s.access(r).WorkspaceID,
		ProjectID:       projectID,
		Name:            r.FormValue("name"),
		Number:          r.FormValue("number"),
		Visible:         checkbox(r, "visible"),
		Billable:        checkbox(r, "billable"),
		EstimateSeconds: formInt(r, "estimate_hours") * 3600,
	}
	if err := s.store.UpsertTask(r.Context(), task); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	action := "create"
	if task.ID != 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "task", &task.ID, task.Name)
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) archiveTask(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := s.store.ArchiveTask(r.Context(), s.access(r).WorkspaceID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "archive", "task", &id, "")
	http.Redirect(w, r, "/tasks", http.StatusSeeOther)
}

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListActivities(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, a := range items {
		project := "Global"
		if a.ProjectID != nil {
			project = labelValue(selectors.ProjectLabels, *a.ProjectID)
		}
		actions := ""
		if s.hasPermission(r, auth.PermManageMaster) {
			actions, err = inlineEditAction(r.Context(), templates.ActivityForm(s.nav(r), selectors, &a))
			if err != nil {
				s.serverError(w, r, err)
				return
			}
		}
		rows = append(rows, []string{html.EscapeString(a.Name), html.EscapeString(project), html.EscapeString(a.Number), html.EscapeString(boolText(a.Visible)), html.EscapeString(boolText(a.Billable)), actions})
	}
	var form templ.Component
	if s.hasPermission(r, auth.PermManageMaster) {
		form = templates.ActivityForm(s.nav(r), selectors, nil)
	}
	s.render(w, r, templates.EntityListRaw("Deliverables", s.nav(r), []string{"Deliverable", "Project", "Code", "Visible", "Billable", "Actions"}, rows, form))
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var project *int64
	if value := formInt(r, "project_id"); value > 0 {
		if !s.canTrackProject(r, value) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		project = &value
	}
	a := &domain.Activity{ID: pathID(r), WorkspaceID: s.access(r).WorkspaceID, ProjectID: project, Name: r.FormValue("name"), Number: r.FormValue("number"), Visible: checkbox(r, "visible"), Billable: checkbox(r, "billable"), Comment: r.FormValue("comment")}
	if err := s.store.UpsertActivity(r.Context(), a); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	action := "create"
	if pathID(r) != 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "activity", &a.ID, a.Name)
	http.Redirect(w, r, "/activities", http.StatusSeeOther)
}

func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, tag := range tags {
		rows = append(rows, []string{tag.Name, boolText(tag.Visible)})
	}
	s.render(w, r, templates.EntityList[domain.Tag]("Tags", s.nav(r), []string{"Name", "Visible"}, rows, templates.TagForm(s.nav(r))))
}

func (s *Server) saveTag(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, err := s.store.UpsertTag(r.Context(), s.access(r).WorkspaceID, r.FormValue("name"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "tag", &id, r.FormValue("name"))
	http.Redirect(w, r, "/tags", http.StatusSeeOther)
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListGroups(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	csrf := s.nav(r).CSRF
	for _, group := range groups {
		actions := fmt.Sprintf(`<details class="inline-edit"><summary class="table-action">Edit</summary><form class="compact-form inline-edit-form" method="post" action="/groups/%d"><input type="hidden" name="csrf" value="%s"><div class="inline-edit-col"><label>Name<input name="name" value="%s" required></label></div><div class="inline-edit-col"><label>Description<textarea name="description">%s</textarea></label><div class="inline-edit-actions"><button class="primary small">Save</button><button class="ghost-button small" type="button" onclick="this.closest('details').removeAttribute('open')">Cancel</button></div></div></form></details><a class="table-action" href="/groups/%d/members">Members</a>`, group.ID, html.EscapeString(csrf), html.EscapeString(group.Name), html.EscapeString(group.Description), group.ID)
		rows = append(rows, []string{group.Name, group.Description, actions})
	}
	s.render(w, r, templates.EntityListRaw("Groups", s.nav(r), []string{"Name", "Description", "Action"}, rows, templates.GroupForm(s.nav(r))))
}

func (s *Server) saveGroup(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	groupID := pathID(r)
	name := r.FormValue("name")
	description := r.FormValue("description")
	var err error
	if groupID > 0 {
		group, groupErr := s.store.Group(r.Context(), groupID)
		if groupErr != nil {
			s.serverError(w, r, groupErr)
			return
		}
		if group == nil || group.WorkspaceID != s.access(r).WorkspaceID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		err = s.store.UpdateGroup(r.Context(), &domain.Group{ID: groupID, WorkspaceID: s.access(r).WorkspaceID, Name: name, Description: description})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	} else {
		groupID, err = s.store.CreateGroup(r.Context(), s.access(r).WorkspaceID, name, description)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	uid := s.state(r).User.ID
	action := "create"
	if pathID(r) > 0 {
		action = "update"
	}
	s.store.Audit(r.Context(), &uid, action, "group", &groupID, name)
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) groupMembers(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	members, err := s.store.ListGroupMembers(r.Context(), group.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.store.ListWorkspaceUsers(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.GroupMembers(s.nav(r), *group, members, users))
}

func (s *Server) groupMemberSave(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	for _, userID := range formIntList(r, "user_id") {
		if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
			s.serverError(w, r, err)
			return
		} else if !ok {
			http.Error(w, "user is not in workspace", http.StatusForbidden)
			return
		}
		if err := s.store.AddUserToGroup(r.Context(), group.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "group_members", &group.ID, "bulk_add")
	http.Redirect(w, r, fmt.Sprintf("/groups/%d/members", group.ID), http.StatusSeeOther)
}

func (s *Server) groupMemberRemove(w http.ResponseWriter, r *http.Request) {
	group, ok := s.currentWorkspaceGroup(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	for _, userID := range formIntList(r, "user_id") {
		if err := s.store.RemoveUserFromGroup(r.Context(), group.ID, userID); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "group_members", &group.ID, "bulk_remove")
	http.Redirect(w, r, fmt.Sprintf("/groups/%d/members", group.ID), http.StatusSeeOther)
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	costs, err := s.store.ListUserCostRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, true, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Rates(s.nav(r), rates, costs, selectors))
}

func (s *Server) saveRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if err := s.validateOptionalRateScope(r); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rate := &domain.Rate{
		WorkspaceID:         s.access(r).WorkspaceID,
		CustomerID:          formOptionalInt(r, "customer_id"),
		ProjectID:           formOptionalInt(r, "project_id"),
		ActivityID:          formOptionalInt(r, "activity_id"),
		TaskID:              formOptionalInt(r, "task_id"),
		UserID:              formOptionalInt(r, "user_id"),
		Kind:                "hourly",
		AmountCents:         formIntAny(r, "amount", "amount_cents"),
		InternalAmountCents: formOptionalIntAny(r, "internal_amount", "internal_amount_cents"),
		Fixed:               checkbox(r, "fixed"),
		EffectiveFrom:       formDateOrEpoch(r, "effective_from"),
		EffectiveTo:         formOptionalDate(r, "effective_to"),
	}
	if err := s.store.UpsertRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/rates", http.StatusSeeOther)
}

func (s *Server) saveUserCostRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	if ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID); err != nil {
		s.serverError(w, r, err)
		return
	} else if !ok {
		http.Error(w, "user is not in workspace", http.StatusForbidden)
		return
	}
	rate := &domain.UserCostRate{
		WorkspaceID:   s.access(r).WorkspaceID,
		UserID:        userID,
		AmountCents:   formIntAny(r, "amount", "amount_cents"),
		EffectiveFrom: formDateOrEpoch(r, "effective_from"),
		EffectiveTo:   formOptionalDate(r, "effective_to"),
	}
	if rate.UserID == 0 || rate.AmountCents < 0 {
		s.badRequest(w, r, errors.New("valid user and cost are required"))
		return
	}
	if err := s.store.UpsertUserCostRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/rates", http.StatusSeeOther)
}

func (s *Server) timesheets(w http.ResponseWriter, r *http.Request) {
	filter := s.timesheetScope(r)
	filter.Page = page(r)
	filter.Size = size(r)
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	favorites, err := s.store.ListFavorites(r.Context(), s.access(r).WorkspaceID, s.state(r).User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	prefill := timesheetPrefillFromRequest(r)
	prefill.Notice = s.popFlash(w, r)
	s.render(w, r, templates.Timesheets(s.nav(r), items, selectors, favorites, dashboardRecentFromTimesheets(items), prefill, s.editableTimesheetIDs(r, items)))
}

func (s *Server) calendar(w http.ResponseWriter, r *http.Request) {
	start := weekStart(time.Now().UTC())
	if date := parseDateParam(r, "date"); date != nil {
		start = weekStart(date.UTC())
	}
	end := start.AddDate(0, 0, 7)
	filter := s.timesheetScope(r)
	filter.Begin = &start
	filter.End = &end
	filter.Page = 1
	filter.Size = 500
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Calendar(s.nav(r), start, items, selectors, s.editableTimesheetIDs(r, items)))
}

func (s *Server) editTimesheet(w http.ResponseWriter, r *http.Request) {
	entry, status, err := s.editableTimesheet(r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.EditTimesheet(s.nav(r), selectors, timesheetPrefillFromEntry(*entry), "", entry.Exported))
}

func (s *Server) saveTimesheet(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	parsed, err := parseTimesheetTime(r)
	if err != nil {
		s.renderTimesheetCreateError(w, r, err.Error())
		return
	}
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, parsed.Start, parsed.End); err != nil {
		s.renderTimesheetCreateError(w, r, err.Error())
		return
	}
	projectID := formInt(r, "project_id")
	workstreamID, err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "workstream_id"), formOptionalInt(r, "task_id"))
	if err != nil {
		s.renderTimesheetCreateError(w, r, err.Error())
		return
	}
	t := &domain.Timesheet{WorkspaceID: s.access(r).WorkspaceID, UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: projectID, WorkstreamID: workstreamID, ActivityID: formInt(r, "activity_id"), TaskID: formOptionalInt(r, "task_id"), StartedAt: parsed.Start, EndedAt: &parsed.End, Timezone: s.cfg.DefaultTimezone, BreakSeconds: parsed.BreakSeconds, Billable: true, Description: r.FormValue("description")}
	if err := s.store.CreateTimesheet(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.created", t)
	http.Redirect(w, r, "/timesheets", http.StatusSeeOther)
}

func (s *Server) renderTimesheetCreateError(w http.ResponseWriter, r *http.Request, message string) {
	filter := s.timesheetScope(r)
	filter.Page = page(r)
	filter.Size = size(r)
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	favorites, err := s.store.ListFavorites(r.Context(), s.access(r).WorkspaceID, s.state(r).User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	prefill := timesheetPrefillFromForm(r)
	prefill.Message = message
	w.WriteHeader(http.StatusBadRequest)
	s.render(w, r, templates.Timesheets(s.nav(r), items, selectors, favorites, dashboardRecentFromTimesheets(items), prefill, s.editableTimesheetIDs(r, items)))
}

func (s *Server) updateTimesheet(w http.ResponseWriter, r *http.Request) {
	entry, status, err := s.editableTimesheet(r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	_ = r.ParseForm()
	selectors, selectorsErr := s.selectorData(r, false, false)
	if selectorsErr != nil {
		s.serverError(w, r, selectorsErr)
		return
	}
	prefill := timesheetPrefillFromForm(r)
	prefill.EntryID = entry.ID
	parsed, err := parseTimesheetTime(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, templates.EditTimesheet(s.nav(r), selectors, prefill, err.Error(), entry.Exported))
		return
	}
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, parsed.Start, parsed.End); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, templates.EditTimesheet(s.nav(r), selectors, prefill, err.Error(), entry.Exported))
		return
	}
	projectID := formInt(r, "project_id")
	workstreamID, err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "workstream_id"), formOptionalInt(r, "task_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, templates.EditTimesheet(s.nav(r), selectors, prefill, err.Error(), entry.Exported))
		return
	}
	entry.CustomerID = formInt(r, "customer_id")
	entry.ProjectID = projectID
	entry.WorkstreamID = workstreamID
	entry.ActivityID = formInt(r, "activity_id")
	entry.TaskID = formOptionalInt(r, "task_id")
	entry.StartedAt = parsed.Start
	entry.EndedAt = &parsed.End
	entry.Timezone = s.cfg.DefaultTimezone
	entry.BreakSeconds = parsed.BreakSeconds
	entry.Billable = checkbox(r, "billable")
	entry.Description = strings.TrimSpace(r.FormValue("description"))
	if err := s.store.UpdateTimesheet(r.Context(), entry, splitCSV(r.FormValue("tags"))); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "update", "timesheet", &entry.ID, "")
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.updated", entry)
	s.redirectWithFlash(w, r, "/timesheets", "success", "Entry updated")
}

func (s *Server) startTimer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	start := time.Now().UTC()
	if err := checkFuturePolicy(s.cfg.FutureTimePolicy, start, start); err != nil {
		s.badRequest(w, r, err)
		return
	}
	projectID := formInt(r, "project_id")
	workstreamID, err := s.validateWorkSelection(r, projectID, formInt(r, "customer_id"), formInt(r, "activity_id"), formOptionalInt(r, "workstream_id"), formOptionalInt(r, "task_id"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	t := &domain.Timesheet{WorkspaceID: s.access(r).WorkspaceID, UserID: s.state(r).User.ID, CustomerID: formInt(r, "customer_id"), ProjectID: projectID, WorkstreamID: workstreamID, ActivityID: formInt(r, "activity_id"), TaskID: formOptionalInt(r, "task_id"), StartedAt: start, Timezone: s.cfg.DefaultTimezone, Billable: true, Description: r.FormValue("description")}
	if err := s.store.StartTimer(r.Context(), t, splitCSV(r.FormValue("tags"))); err != nil {
		s.badRequest(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "start", "timesheet", &t.ID, "")
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.started", t)
	redirectOrJSON(w, r, "/")
}

func (s *Server) stopTimer(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.StopTimer(r.Context(), s.state(r).User.ID, time.Now().UTC())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if t != nil {
		uid := s.state(r).User.ID
		s.store.Audit(r.Context(), &uid, "stop", "timesheet", &t.ID, "")
		s.queueEvent(r.Context(), s.access(r).WorkspaceID, "timesheet.stopped", t)
	}
	redirectOrJSON(w, r, "/")
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	filter := domain.ReportFilter{
		Group:      defaultString(r.URL.Query().Get("group"), "user"),
		CustomerID: int64Param(r, "customer_id"),
		ProjectID:  int64Param(r, "project_id"),
		ActivityID: int64Param(r, "activity_id"),
		TaskID:     int64Param(r, "task_id"),
		UserID:     int64Param(r, "user_id"),
		GroupID:    int64Param(r, "group_id"),
	}
	filter.Begin = parseDateParam(r, "begin")
	filter.End = parseDateParam(r, "end")
	filter.Billable = parseBoolFilter(r.URL.Query().Get("billable"))
	rows, err := s.store.ListReports(r.Context(), s.access(r), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	saved, err := s.store.ListSavedReports(r.Context(), s.access(r).WorkspaceID, s.state(r).User.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, true, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Reports(s.nav(r), filter, rows, saved, selectors))
}

func (s *Server) saveReport(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	filter := map[string]string{
		"begin":       r.FormValue("begin"),
		"end":         r.FormValue("end"),
		"customer_id": r.FormValue("customer_id"),
		"project_id":  r.FormValue("project_id"),
		"activity_id": r.FormValue("activity_id"),
		"task_id":     r.FormValue("task_id"),
		"user_id":     r.FormValue("user_id"),
		"group_id":    r.FormValue("group_id"),
		"billable":    r.FormValue("billable"),
	}
	body, err := json.Marshal(filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	report := &domain.SavedReport{
		WorkspaceID: s.access(r).WorkspaceID,
		UserID:      s.state(r).User.ID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		GroupBy:     defaultString(r.FormValue("group"), "user"),
		FiltersJSON: string(body),
		Shared:      checkbox(r, "shared"),
	}
	if report.Name == "" {
		s.badRequest(w, r, errors.New("saved report name is required"))
		return
	}
	if err := s.store.CreateSavedReport(r.Context(), report); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "saved_report", &report.ID, report.Name)
	http.Redirect(w, r, "/reports?group="+report.GroupBy, http.StatusSeeOther)
}

func (s *Server) invoices(w http.ResponseWriter, r *http.Request) {
	items, _, err := s.store.ListInvoices(r.Context(), s.access(r).WorkspaceID, int64Param(r, "customer_id"), page(r), size(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Invoices(s.nav(r), items, selectors))
}

func (s *Server) createInvoice(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	begin, err := time.Parse("2006-01-02", r.FormValue("begin"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	end, err := time.Parse("2006-01-02", r.FormValue("end"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	customerID := formInt(r, "customer_id")
	if !s.customerInScope(r, customerID) {
		http.Error(w, "invalid customer", http.StatusForbidden)
		return
	}
	inv, err := s.store.CreateInvoice(r.Context(), s.access(r), s.state(r).User.ID, customerID, begin, end.Add(24*time.Hour), formInt(r, "tax"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := s.writeInvoiceFile(inv); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "create", "invoice", &inv.ID, inv.Number)
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "invoice.created", inv)
	http.Redirect(w, r, "/invoices", http.StatusSeeOther)
}

func (s *Server) webhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Webhooks(s.nav(r), hooks))
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	hook := &domain.WebhookEndpoint{WorkspaceID: s.access(r).WorkspaceID, Name: r.FormValue("name"), URL: r.FormValue("url"), Secret: r.FormValue("secret"), Events: splitCSV(r.FormValue("events")), Enabled: true}
	if err := s.store.CreateWebhookEndpoint(r.Context(), hook); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/webhooks", http.StatusSeeOther)
}

func (s *Server) projectTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjectTemplates(r.Context(), s.access(r).WorkspaceID, true)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for i := range items {
		full, err := s.store.ProjectTemplate(r.Context(), s.access(r).WorkspaceID, items[i].ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if full != nil {
			items[i] = *full
		}
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectTemplates(s.nav(r), items, selectors))
}

func (s *Server) projectTemplateDetail(w http.ResponseWriter, r *http.Request) {
	template, err := s.store.ProjectTemplate(r.Context(), s.access(r).WorkspaceID, pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if template == nil {
		http.NotFound(w, r)
		return
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectTemplateDetail(s.nav(r), *template, selectors))
}

func (s *Server) saveProjectTemplate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	template := projectTemplateFromForm(r)
	template.ID = pathID(r)
	template.WorkspaceID = s.access(r).WorkspaceID
	if err := s.store.UpsertProjectTemplate(r.Context(), &template); err != nil {
		s.badRequest(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "project_template", &template.ID, template.Name)
	http.Redirect(w, r, fmt.Sprintf("/project-templates/%d", template.ID), http.StatusSeeOther)
}

func (s *Server) useProjectTemplate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	templateID := pathID(r)
	if templateID == 0 {
		templateID = formInt(r, "template_id")
	}
	projectID, err := s.store.CreateProjectFromTemplate(r.Context(), s.access(r).WorkspaceID, templateID, formInt(r, "customer_id"), r.FormValue("project_name"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "create", "project", &projectID, fmt.Sprintf("from_template=%d", templateID))
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/dashboard", projectID), http.StatusSeeOther)
}

func (s *Server) workspacesAdmin(w http.ResponseWriter, r *http.Request) {
	workspaces, err := s.store.ListOrganizationWorkspaces(r.Context(), s.access(r).OrganizationID, r.URL.Query().Get("q"))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.WorkspaceAdmin(s.nav(r), workspaces))
}

func (s *Server) workspaceAdminDetail(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	smtpSettings, err := s.store.WorkspaceSMTPSettings(r.Context(), workspace.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	members, err := s.store.ListWorkspaceMembers(r.Context(), workspace.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	users, err := s.organizationUsers(r.Context(), s.access(r).OrganizationID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.WorkspaceDetail(s.nav(r), *workspace, smtpSettings, members, users, s.popFlash(w, r)))
}

func (s *Server) saveWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	workspace := &domain.Workspace{
		ID:              pathID(r),
		OrganizationID:  s.access(r).OrganizationID,
		Name:            r.FormValue("name"),
		Slug:            defaultString(r.FormValue("slug"), slugify(r.FormValue("name"))),
		Description:     r.FormValue("description"),
		DefaultCurrency: defaultString(r.FormValue("default_currency"), "USD"),
		Timezone:        defaultString(r.FormValue("timezone"), "UTC"),
		Archived:        checkbox(r, "archived"),
	}
	if workspace.ID > 0 {
		if existing, ok := s.organizationWorkspace(w, r); !ok {
			return
		} else {
			workspace.OrganizationID = existing.OrganizationID
			workspace.SMTP = existing.SMTP
		}
	}
	if err := s.store.UpsertWorkspace(r.Context(), workspace); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if workspace.ID > 0 {
		existingSMTP, err := s.store.WorkspaceSMTPSettings(r.Context(), workspace.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		smtpPort := 587
		if raw := strings.TrimSpace(r.FormValue("smtp_port")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				s.badRequest(w, r, errors.New("SMTP port must be a positive number"))
				return
			}
			smtpPort = parsed
		}
		smtpSettings := domain.WorkspaceSMTPSettings{
			Host:      strings.TrimSpace(r.FormValue("smtp_host")),
			Port:      smtpPort,
			Username:  strings.TrimSpace(r.FormValue("smtp_username")),
			Password:  r.FormValue("smtp_password"),
			FromEmail: strings.TrimSpace(r.FormValue("smtp_from_email")),
			FromName:  strings.TrimSpace(r.FormValue("smtp_from_name")),
			TLS:       checkbox(r, "smtp_tls"),
		}
		if smtpSettings.Password == "" {
			smtpSettings.PasswordEncrypted = existingSMTP.PasswordEncrypted
		}
		if smtpSettings.Host != "" || smtpSettings.Username != "" || smtpSettings.Password != "" || smtpSettings.FromEmail != "" || smtpSettings.FromName != "" {
			if err := emailer.NewSender(emailer.SMTPConfig{Host: smtpSettings.Host, Port: smtpSettings.Port, Username: smtpSettings.Username, Password: smtpSettings.Password, FromEmail: smtpSettings.FromEmail, FromName: smtpSettings.FromName, TLS: smtpSettings.TLS}).Validate(); err != nil {
				s.badRequest(w, r, err)
				return
			}
		}
		if smtpSettings.Host == "" && smtpSettings.FromEmail == "" {
			smtpSettings.Username = ""
			smtpSettings.Password = ""
			smtpSettings.PasswordEncrypted = ""
			smtpSettings.FromName = ""
		}
		if err := s.store.UpsertWorkspaceSMTPSettings(r.Context(), workspace.ID, smtpSettings); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	if workspace.ID > 0 && !workspace.Archived {
		actor := s.state(r).User.ID
		_ = s.store.SetWorkspaceMember(r.Context(), workspace.ID, actor, domain.WorkspaceRoleAdmin)
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace", &workspace.ID, workspace.Name)
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
}

func (s *Server) workspaceSMTPTest(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	to := strings.TrimSpace(r.FormValue("to"))
	if _, err := mail.ParseAddress(to); err != nil {
		s.redirectWithFlash(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), "error", "Enter a valid test recipient")
		return
	}
	sender, err := s.senderForWorkspace(r.Context(), workspace.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := sender.Send(emailer.Message{
		To:      to,
		Subject: "Tockr SMTP test",
		Text:    "This is a Tockr SMTP test email. If you received it, email sending is working.",
	}); err != nil {
		s.redirectWithFlash(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), "error", "SMTP test failed: "+err.Error())
		return
	}
	s.redirectWithFlash(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), "success", "SMTP test email sent")
}

func (s *Server) workspaceMemberSave(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	user, err := s.store.FindUserByID(r.Context(), userID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user == nil || user.OrganizationID != workspace.OrganizationID {
		http.Error(w, "user is outside organization", http.StatusForbidden)
		return
	}
	role := domain.WorkspaceRole(r.FormValue("role"))
	if err := s.store.SetWorkspaceMember(r.Context(), workspace.ID, userID, role); err != nil {
		s.serverError(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace_members", &workspace.ID, fmt.Sprintf("user=%d role=%s", userID, role))
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
}

func (s *Server) workspaceMemberRemove(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.organizationWorkspace(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	userID := formInt(r, "user_id")
	if err := s.store.RemoveWorkspaceMember(r.Context(), workspace.ID, userID); err != nil {
		s.serverError(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	s.store.Audit(r.Context(), &actor, "update", "workspace_members", &workspace.ID, fmt.Sprintf("remove_user=%d", userID))
	http.Redirect(w, r, fmt.Sprintf("/admin/workspaces/%d", workspace.ID), http.StatusSeeOther)
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	users, err := s.organizationUsers(r.Context(), s.access(r).OrganizationID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := [][]string{}
	for _, u := range users {
		roles := []string{}
		for _, role := range u.Roles {
			roles = append(roles, string(role))
		}
		actions, err := inlineEditAction(r.Context(), templates.UserForm(s.nav(r), &u))
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		rows = append(rows, []string{html.EscapeString(u.Email), html.EscapeString(u.DisplayName), html.EscapeString(strings.Join(roles, ",")), html.EscapeString(boolText(u.Enabled)), actions})
	}
	s.render(w, r, templates.EntityListRaw("Users", s.nav(r), []string{"Email", "Name", "Roles", "Enabled", "Actions"}, rows, templates.UserForm(s.nav(r), nil)))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	role := legacyRoleFromForm(r.FormValue("role"))
	targetID := pathID(r)
	user := domain.User{
		ID:             targetID,
		OrganizationID: s.access(r).OrganizationID,
		Email:          r.FormValue("email"),
		Username:       r.FormValue("username"),
		DisplayName:    r.FormValue("display_name"),
		Timezone:       defaultString(r.FormValue("timezone"), "UTC"),
		Enabled:        checkbox(r, "enabled"),
	}
	password := r.FormValue("password")
	var err error
	if targetID == 0 {
		user.Enabled = true
		err = s.store.CreateUser(r.Context(), user, password, []domain.Role{role})
	} else {
		existing, findErr := s.store.FindUserByID(r.Context(), targetID)
		if findErr != nil {
			s.serverError(w, r, findErr)
			return
		}
		if existing == nil || existing.OrganizationID != s.access(r).OrganizationID {
			http.NotFound(w, r)
			return
		}
		err = s.store.UpdateUser(r.Context(), user, password, []domain.Role{role})
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	actor := s.state(r).User.ID
	action := "create"
	var entityID *int64
	if targetID != 0 {
		action = "update"
		entityID = &user.ID
	}
	s.store.Audit(r.Context(), &actor, action, "user", entityID, user.Email)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "app": "tockr"})
}

func (s *Server) apiCustomers(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListCustomers(r.Context(), s.access(r), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiProjects(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListProjects(r.Context(), s.access(r), int64Param(r, "customer_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiActivities(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListActivities(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiTasks(w http.ResponseWriter, r *http.Request) {
	items, pageInfo, err := s.store.ListTasks(r.Context(), s.access(r), int64Param(r, "project_id"), r.URL.Query().Get("q"), page(r), size(r))
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiTimesheets(w http.ResponseWriter, r *http.Request) {
	filter := s.timesheetScope(r)
	filter.Page = page(r)
	filter.Size = size(r)
	items, pageInfo, err := s.store.ListTimesheets(r.Context(), filter)
	s.writePage(w, items, pageInfo, err)
}

func (s *Server) apiWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhookEndpoints(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (s *Server) apiInvoiceDownload(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	inv, err := s.store.Invoice(r.Context(), id)
	if err != nil || inv == nil {
		http.NotFound(w, r)
		return
	}
	if inv.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// Validate invoice filename to prevent path traversal
	if !isValidInvoiceFilename(inv.Filename) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.cfg.DataDir, "invoices", inv.Filename)
	if _, err := os.Stat(path); err != nil { // #nosec G703 path uses validated invoice filename
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, inv.Filename))
	http.ServeFile(w, r, path)
}

func (s *Server) apiInvoiceMeta(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	inv, err := s.store.Invoice(r.Context(), id)
	if err != nil || inv == nil {
		http.NotFound(w, r)
		return
	}
	if inv.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	name := r.FormValue("name")
	if name == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.badRequest(w, r, err)
			return
		}
		name = body.Name
		r.Form.Set("value", body.Value)
	}
	if name == "" {
		s.badRequest(w, r, errors.New("metadata name is required"))
		return
	}
	if err := s.store.SetInvoiceMeta(r.Context(), id, name, r.FormValue("value")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.queueEvent(r.Context(), s.access(r).WorkspaceID, "invoice.meta.updated", map[string]any{"invoice_id": id, "name": name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invoice_id": id, "name": name})
}

func (s *Server) writePage(w http.ResponseWriter, data any, pageInfo domain.Page, err error) {
	if err != nil {
		s.serverError(w, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "page": pageInfo})
}

func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &requestState{}
		if cookie, err := r.Cookie("tockr_session"); err == nil {
			if sid, ok := s.unsign(cookie.Value); ok {
				session, err := s.store.FindSession(r.Context(), sid)
				if err == nil && session != nil {
					user, err := s.store.FindUserByID(r.Context(), session.UserID)
					if err == nil && user != nil {
						state.User = user
						state.Session = session
						access, err := s.store.AccessForUserWorkspace(r.Context(), user.ID, session.WorkspaceID)
						if err == nil {
							state.Access = access
						}
					}
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), stateKey, state)))
	})
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		headers.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")

		if r.URL.Path == "/login" || r.URL.Path == "/forgot-password" || r.URL.Path == "/reset-password" || strings.HasPrefix(r.URL.Path, "/account") {
			headers.Set("Cache-Control", "no-store")
		}

		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		if scheme == "https" || s.cfg.CookieSecure {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.RateLimitEnabled {
			next.ServeHTTP(w, r)
			return
		}
		limit, window := s.rateLimitPolicy(r)
		if limit == 0 {
			next.ServeHTTP(w, r)
			return
		}

		now := time.Now().UTC()
		key := r.Method + "|" + r.URL.Path + "|" + clientIP(r)

		s.rateLimitMu.Lock()
		bucket := s.rateLimitBuckets[key]
		if bucket.WindowStart.IsZero() || now.Sub(bucket.WindowStart) >= window {
			bucket = rateLimitBucket{Count: 1, WindowStart: now, LastSeen: now}
			s.rateLimitBuckets[key] = bucket
			s.cleanupRateLimitBuckets(now)
			s.rateLimitMu.Unlock()
			next.ServeHTTP(w, r)
			return
		}
		bucket.Count++
		bucket.LastSeen = now
		s.rateLimitBuckets[key] = bucket
		s.cleanupRateLimitBuckets(now)
		s.rateLimitMu.Unlock()

		if bucket.Count > limit {
			retryAfter := int(window.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			if state := s.state(r); state != nil && state.User != nil {
				uid := state.User.ID
				s.store.Audit(r.Context(), &uid, "rate_limited", "http", nil, r.Method+" "+r.URL.Path)
			}
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) cleanupRateLimitBuckets(now time.Time) {
	if len(s.rateLimitBuckets) < 1024 {
		return
	}
	for key, bucket := range s.rateLimitBuckets {
		if now.Sub(bucket.LastSeen) > 30*time.Minute {
			delete(s.rateLimitBuckets, key)
		}
	}
}

func (s *Server) rateLimitPolicy(r *http.Request) (int, time.Duration) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		return 0, 0
	}
	path := r.URL.Path
	if path == "/login" {
		return 8, time.Minute
	}
	if path == "/forgot-password" || path == "/reset-password" {
		return 5, 10 * time.Minute
	}
	if path == "/account/password" || path == "/account/email" || path == "/account/email/verify" || path == "/account/totp/enable" || path == "/account/totp/disable" {
		return 20, 10 * time.Minute
	}
	if path == "/timesheets/start" || path == "/timesheets/stop" || path == "/api/timer/start" || path == "/api/timer/stop" {
		return 120, time.Minute
	}
	return 0, 0
}

func clientIP(r *http.Request) string {
	ip := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || r.URL.Path == "/login" || r.URL.Path == "/forgot-password" || r.URL.Path == "/reset-password" {
			next.ServeHTTP(w, r)
			return
		}
		state := s.state(r)
		if state.Session == nil {
			http.Error(w, "missing session", http.StatusForbidden)
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			_ = r.ParseForm()
			token = r.FormValue("csrf")
		}
		if token == "" || !hmac.Equal([]byte(token), []byte(state.Session.CSRFToken)) {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.state(r).User == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if s.mustSetupTOTP(r) {
			s.redirectWithFlash(w, r, "/account", "warning", "Two-factor setup is required")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireLoginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.state(r).User == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if s.mustSetupTOTP(r) {
			s.redirectWithFlash(w, r, "/account", "warning", "Two-factor setup is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.hasPermission(r, permission) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) requirePermissionMiddleware(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.hasPermission(r, permission) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) hasPermission(r *http.Request, permission string) bool {
	return auth.HasPermission(s.access(r), permission)
}

func (s *Server) access(r *http.Request) domain.AccessContext {
	return s.state(r).Access
}

func (s *Server) timesheetScope(r *http.Request) sqlite.TimesheetFilter {
	access := s.access(r)
	filter := sqlite.TimesheetFilter{WorkspaceID: access.WorkspaceID}
	if access.IsWorkspaceAdmin() || access.WorkspaceRole == domain.WorkspaceRoleAnalyst {
		return filter
	}
	projectIDs := accessibleProjectIDs(access)
	if len(projectIDs) > 0 {
		filter.ProjectIDs = projectIDs
		return filter
	}
	filter.UserID = access.UserID
	return filter
}

func (s *Server) editableTimesheetIDs(r *http.Request, items []domain.Timesheet) map[int64]bool {
	editable := make(map[int64]bool, len(items))
	for _, item := range items {
		if s.canEditTimesheet(r, item) {
			editable[item.ID] = true
		}
	}
	return editable
}

func (s *Server) canEditTimesheet(r *http.Request, item domain.Timesheet) bool {
	access := s.access(r)
	if item.WorkspaceID != access.WorkspaceID || item.Exported || item.EndedAt == nil {
		return false
	}
	return access.IsWorkspaceAdmin() || item.UserID == access.UserID || access.ManagesProject(item.ProjectID)
}

func (s *Server) editableTimesheet(r *http.Request) (*domain.Timesheet, int, error) {
	entry, err := s.store.Timesheet(r.Context(), pathID(r))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if entry == nil {
		return nil, http.StatusNotFound, errors.New("not found")
	}
	if entry.WorkspaceID != s.access(r).WorkspaceID {
		return nil, http.StatusForbidden, errors.New("forbidden")
	}
	if !s.canEditTimesheet(r, *entry) {
		if entry.Exported {
			return nil, http.StatusBadRequest, errors.New("exported entries cannot be edited")
		}
		return nil, http.StatusForbidden, errors.New("forbidden")
	}
	return entry, http.StatusOK, nil
}

func (s *Server) canTrackProject(r *http.Request, projectID int64) bool {
	access := s.access(r)
	if projectID == 0 {
		return true
	}
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil || project == nil || project.WorkspaceID != access.WorkspaceID {
		return false
	}
	return access.IsWorkspaceAdmin() || !project.Private || access.CanAccessProject(projectID)
}

func (s *Server) authorizedProjectManagement(w http.ResponseWriter, r *http.Request) (*domain.Project, bool) {
	projectID := pathID(r)
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil {
		s.serverError(w, r, err)
		return nil, false
	}
	if project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return nil, false
	}
	if !s.access(r).ManagesProject(projectID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return project, true
}

func (s *Server) organizationWorkspace(w http.ResponseWriter, r *http.Request) (*domain.Workspace, bool) {
	workspace := s.workspaceByID(w, r, pathID(r))
	if workspace == nil {
		return nil, false
	}
	if workspace.OrganizationID != s.access(r).OrganizationID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return workspace, true
}

func (s *Server) workspaceByID(w http.ResponseWriter, r *http.Request, id int64) *domain.Workspace {
	workspace, err := s.store.Workspace(r.Context(), id)
	if err != nil {
		s.serverError(w, r, err)
		return nil
	}
	if workspace == nil {
		http.NotFound(w, r)
		return nil
	}
	return workspace
}

func (s *Server) currentWorkspaceGroup(w http.ResponseWriter, r *http.Request) (*domain.Group, bool) {
	group, err := s.store.Group(r.Context(), pathID(r))
	if err != nil {
		s.serverError(w, r, err)
		return nil, false
	}
	if group == nil {
		http.NotFound(w, r)
		return nil, false
	}
	if group.WorkspaceID != s.access(r).WorkspaceID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return group, true
}

func (s *Server) organizationUsers(ctx context.Context, organizationID int64) ([]domain.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.User, 0, len(users))
	for _, user := range users {
		if user.OrganizationID == organizationID {
			filtered = append(filtered, user)
		}
	}
	return filtered, nil
}

func (s *Server) userInWorkspace(ctx context.Context, userID, workspaceID int64) (bool, error) {
	users, err := s.store.ListWorkspaceUsers(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user.ID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) selectorData(r *http.Request, includeUsers, includeGroups bool) (*templates.SelectorData, error) {
	access := s.access(r)
	customers, _, err := s.store.ListCustomers(r.Context(), access, "", 1, 100)
	if err != nil {
		return nil, err
	}
	projects, _, err := s.store.ListProjects(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	activities, _, err := s.store.ListActivities(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	workstreams, err := s.store.ListWorkstreams(r.Context(), access.WorkspaceID)
	if err != nil {
		return nil, err
	}
	tasks, _, err := s.store.ListTasks(r.Context(), access, 0, "", 1, 100)
	if err != nil {
		return nil, err
	}
	projectWorkstreamIDs := map[int64][]string{}
	for _, project := range projects {
		assigned, err := s.store.ListProjectWorkstreams(r.Context(), project.ID)
		if err != nil {
			return nil, err
		}
		for _, item := range assigned {
			projectWorkstreamIDs[item.WorkstreamID] = append(projectWorkstreamIDs[item.WorkstreamID], fmt.Sprint(project.ID))
		}
	}
	data := &templates.SelectorData{
		CustomerLabels:   map[int64]string{},
		ProjectLabels:    map[int64]string{},
		WorkstreamLabels: map[int64]string{},
		ActivityLabels:   map[int64]string{},
		TaskLabels:       map[int64]string{},
		UserLabels:       map[int64]string{},
		GroupLabels:      map[int64]string{},
	}
	for _, customer := range customers {
		label := customerLabel(customer)
		data.CustomerLabels[customer.ID] = label
		data.Customers = append(data.Customers, templates.SelectOption{Value: customer.ID, Label: label})
	}
	for _, project := range projects {
		label := projectLabel(project, data.CustomerLabels)
		data.ProjectLabels[project.ID] = label
		data.Projects = append(data.Projects, templates.SelectOption{Value: project.ID, Label: label, Attrs: map[string]string{"customer-id": fmt.Sprint(project.CustomerID)}})
	}
	for _, workstream := range workstreams {
		data.WorkstreamLabels[workstream.ID] = workstream.Name
		attrs := map[string]string{}
		if projectIDs := projectWorkstreamIDs[workstream.ID]; len(projectIDs) > 0 {
			attrs["project-ids"] = strings.Join(projectIDs, ",")
		}
		data.Workstreams = append(data.Workstreams, templates.SelectOption{Value: workstream.ID, Label: workstream.Name, Attrs: attrs})
	}
	for _, activity := range activities {
		label := activityLabel(activity, data.ProjectLabels)
		data.ActivityLabels[activity.ID] = label
		attrs := map[string]string{}
		if activity.ProjectID != nil {
			attrs["project-id"] = fmt.Sprint(*activity.ProjectID)
		}
		data.Activities = append(data.Activities, templates.SelectOption{Value: activity.ID, Label: label, Attrs: attrs})
	}
	for _, task := range tasks {
		label := taskLabel(task, data.ProjectLabels)
		data.TaskLabels[task.ID] = label
		data.Tasks = append(data.Tasks, templates.SelectOption{Value: task.ID, Label: label, Attrs: map[string]string{"project-id": fmt.Sprint(task.ProjectID)}})
	}
	if includeUsers {
		users, err := s.store.ListWorkspaceUsers(r.Context(), access.WorkspaceID)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			label := userLabel(user)
			data.UserLabels[user.ID] = label
			data.Users = append(data.Users, templates.SelectOption{Value: user.ID, Label: label})
		}
	}
	if includeGroups {
		groups, err := s.store.ListGroups(r.Context(), access.WorkspaceID)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			data.GroupLabels[group.ID] = group.Name
			data.Groups = append(data.Groups, templates.SelectOption{Value: group.ID, Label: group.Name})
		}
	}
	return data, nil
}

func customerLabel(customer domain.Customer) string {
	return strings.TrimSpace(customer.Name)
}

func projectLabel(project domain.Project, _ map[int64]string) string {
	return project.Name
}

func activityLabel(activity domain.Activity, _ map[int64]string) string {
	return activity.Name
}

func taskLabel(task domain.Task, _ map[int64]string) string {
	return task.Name
}

func userLabel(user domain.User) string {
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = user.Username
	}
	if user.Email != "" && user.Email != name {
		return name + " - " + user.Email
	}
	return name
}

func labelValue(labels map[int64]string, id int64) string {
	if labels != nil {
		if value := labels[id]; value != "" {
			return value
		}
	}
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("Unavailable #%d", id)
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func legacyRoleFromForm(value string) domain.Role {
	switch domain.Role(strings.TrimSpace(strings.ToLower(value))) {
	case domain.RoleSuperAdmin:
		return domain.RoleSuperAdmin
	case domain.RoleAdmin:
		return domain.RoleAdmin
	case domain.RoleTeamLead:
		return domain.RoleTeamLead
	default:
		return domain.RoleUser
	}
}

func renderComponentString(ctx context.Context, component templ.Component) (string, error) {
	if component == nil {
		return "", nil
	}
	var b strings.Builder
	if err := component.Render(ctx, &b); err != nil {
		return "", err
	}
	return b.String(), nil
}

func inlineEditAction(ctx context.Context, form templ.Component) (string, error) {
	body, err := renderComponentString(ctx, form)
	if err != nil {
		return "", err
	}
	return `<details class="inline-edit"><summary class="table-action">Edit</summary><div class="inline-edit-form inline-edit-generic">` + body + `<div class="inline-edit-actions"><button class="ghost-button small" type="button" onclick="this.closest('details').removeAttribute('open')">Cancel</button></div></div></details>`, nil
}

func (s *Server) customerInScope(r *http.Request, customerID int64) bool {
	if customerID == 0 {
		return false
	}
	customer, err := s.store.Customer(r.Context(), customerID)
	return err == nil && customer != nil && customer.WorkspaceID == s.access(r).WorkspaceID
}

func (s *Server) validateWorkSelection(r *http.Request, projectID, customerID, activityID int64, workstreamID, taskID *int64) (*int64, error) {
	if projectID == 0 || customerID == 0 || activityID == 0 {
		return nil, errors.New("customer, project, and activity are required")
	}
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	if project == nil || project.WorkspaceID != s.access(r).WorkspaceID || project.CustomerID != customerID || !s.canTrackProject(r, projectID) {
		return nil, errors.New("invalid project selection")
	}
	activity, err := s.store.Activity(r.Context(), activityID)
	if err != nil {
		return nil, err
	}
	if activity == nil || activity.WorkspaceID != s.access(r).WorkspaceID {
		return nil, errors.New("invalid activity selection")
	}
	if activity.ProjectID != nil && *activity.ProjectID != projectID {
		return nil, errors.New("activity does not belong to the selected project")
	}
	projectWorkstreams, err := s.store.ListProjectWorkstreams(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	if len(projectWorkstreams) > 0 {
		if workstreamID == nil || *workstreamID == 0 {
			return nil, errors.New("workstream is required for the selected project")
		}
		allowed := false
		for _, item := range projectWorkstreams {
			if item.Active && item.WorkstreamID == *workstreamID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("workstream does not belong to the selected project")
		}
	} else {
		// Projects without configured workstreams should still accept time entries.
		// Ignore any stale workstream IDs posted from old links/prefills.
		workstreamID = nil
	}
	if taskID != nil {
		task, err := s.store.Task(r.Context(), *taskID)
		if err != nil {
			return nil, err
		}
		if task == nil || task.WorkspaceID != s.access(r).WorkspaceID || task.ProjectID != projectID {
			return nil, errors.New("task does not belong to the selected project")
		}
	}
	return workstreamID, nil
}

func (s *Server) validateOptionalRateScope(r *http.Request) error {
	customerID := formInt(r, "customer_id")
	if customerID > 0 && !s.customerInScope(r, customerID) {
		return errors.New("invalid customer selection")
	}
	projectID := formInt(r, "project_id")
	if projectID > 0 && !s.canTrackProject(r, projectID) {
		return errors.New("invalid project selection")
	}
	if projectID > 0 && customerID > 0 {
		project, err := s.store.Project(r.Context(), projectID)
		if err != nil {
			return err
		}
		if project == nil || project.CustomerID != customerID {
			return errors.New("project does not belong to the selected customer")
		}
	}
	if activityID := formInt(r, "activity_id"); activityID > 0 {
		activity, err := s.store.Activity(r.Context(), activityID)
		if err != nil {
			return err
		}
		if activity == nil || activity.WorkspaceID != s.access(r).WorkspaceID || (activity.ProjectID != nil && projectID > 0 && *activity.ProjectID != projectID) {
			return errors.New("invalid activity selection")
		}
	}
	if taskID := formInt(r, "task_id"); taskID > 0 {
		task, err := s.store.Task(r.Context(), taskID)
		if err != nil {
			return err
		}
		if task == nil || task.WorkspaceID != s.access(r).WorkspaceID || (projectID > 0 && task.ProjectID != projectID) {
			return errors.New("invalid task selection")
		}
	}
	if userID := formInt(r, "user_id"); userID > 0 {
		ok, err := s.userInWorkspace(r.Context(), userID, s.access(r).WorkspaceID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("invalid user selection")
		}
	}
	return nil
}

func (s *Server) totpAvailable() bool {
	return s.cfg.TOTPMode == "optional" || s.cfg.TOTPMode == "required"
}

func (s *Server) totpRequired() bool {
	return s.cfg.TOTPMode == "required"
}

func (s *Server) mustSetupTOTP(r *http.Request) bool {
	state := s.state(r)
	if !s.totpRequired() || state.User == nil || state.User.TOTPEnabled {
		return false
	}
	path := r.URL.Path
	return path != "/account" && path != "/account/totp/enable" && path != "/logout" && !strings.HasPrefix(path, "/static/")
}

func accessibleProjectIDs(access domain.AccessContext) []int64 {
	seen := map[int64]bool{}
	ids := []int64{}
	for id := range access.ManagedProjectIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range access.MemberProjectIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Server) state(r *http.Request) *requestState {
	state, _ := r.Context().Value(stateKey).(*requestState)
	if state == nil {
		return &requestState{}
	}
	return state
}

func (s *Server) nav(r *http.Request) *templates.NavUser {
	state := s.state(r)
	if state.User == nil || state.Session == nil {
		return nil
	}
	permissions := map[string]bool{}
	for _, permission := range []string{
		auth.PermAdmin,
		auth.PermManageOrg,
		auth.PermManageUsers,
		auth.PermManageMaster,
		auth.PermManageRates,
		auth.PermTrackTime,
		auth.PermViewReports,
		auth.PermManageInvoices,
		auth.PermUseAPI,
		auth.PermManageWebhooks,
		auth.PermManageGroups,
		auth.PermManageProjects,
	} {
		permissions[permission] = auth.HasPermission(state.Access, permission)
	}
	workspaces, _ := s.store.ListUserWorkspaces(r.Context(), state.User.ID)
	currentName := ""
	for _, workspace := range workspaces {
		if workspace.ID == state.Access.WorkspaceID {
			currentName = workspace.Name
			break
		}
	}
	if currentName == "" {
		currentName = "Workspace"
	}
	return &templates.NavUser{DisplayName: state.User.DisplayName, Email: state.User.Email, CSRF: state.Session.CSRFToken, CurrentPath: r.URL.Path, Permissions: permissions, CurrentWorkspaceID: state.Access.WorkspaceID, CurrentWorkspaceName: currentName, Workspaces: workspaces}
}

func (s *Server) hasAnyAdminAccess(r *http.Request) bool {
	for _, permission := range []string{
		auth.PermManageOrg,
		auth.PermManageRates,
		auth.PermManageProjects,
		auth.PermManageUsers,
		auth.PermManageWebhooks,
	} {
		if s.hasPermission(r, permission) {
			return true
		}
	}
	return false
}

func (s *Server) bootstrapWorkspaceSMTPFromGlobal(ctx context.Context) error {
	legacy := s.legacyGlobalSMTPConfig()
	if !emailer.NewSender(legacy).Configured() {
		return nil
	}
	workspaceID, err := s.store.DefaultWorkspaceForOrganization(ctx, 1)
	if err != nil || workspaceID == 0 {
		return err
	}
	existing, err := s.store.WorkspaceSMTPSettings(ctx, workspaceID)
	if err != nil {
		return err
	}
	if existing.Host != "" && existing.FromEmail != "" {
		return nil
	}
	seed := domain.WorkspaceSMTPSettings{
		Host:      legacy.Host,
		Port:      legacy.Port,
		Username:  legacy.Username,
		Password:  legacy.Password,
		FromEmail: legacy.FromEmail,
		FromName:  legacy.FromName,
		TLS:       legacy.TLS,
	}
	return s.store.UpsertWorkspaceSMTPSettings(ctx, workspaceID, seed)
}

func (s *Server) legacyGlobalSMTPConfig() emailer.SMTPConfig {
	fromEmail := ""
	fromName := ""
	if parsed, err := mail.ParseAddress(strings.TrimSpace(s.cfg.SMTPFrom)); err == nil {
		fromEmail = parsed.Address
		fromName = parsed.Name
	} else {
		fromEmail = strings.TrimSpace(s.cfg.SMTPFrom)
	}
	return emailer.SMTPConfig{
		Host:      strings.TrimSpace(s.cfg.SMTPHost),
		Port:      s.cfg.SMTPPort,
		Username:  strings.TrimSpace(s.cfg.SMTPUsername),
		Password:  s.cfg.SMTPPassword,
		FromEmail: strings.TrimSpace(fromEmail),
		FromName:  strings.TrimSpace(fromName),
		TLS:       s.cfg.SMTPStartTLS,
	}
}

func (s *Server) smtpConfigForWorkspace(ctx context.Context, workspaceID int64) (emailer.SMTPConfig, error) {
	settings, err := s.store.WorkspaceSMTPSettings(ctx, workspaceID)
	if err != nil {
		return emailer.SMTPConfig{}, err
	}
	cfg := emailer.SMTPConfig{
		Host:      strings.TrimSpace(settings.Host),
		Port:      settings.Port,
		Username:  strings.TrimSpace(settings.Username),
		Password:  settings.Password,
		FromEmail: strings.TrimSpace(settings.FromEmail),
		FromName:  strings.TrimSpace(settings.FromName),
		TLS:       settings.TLS,
	}
	if emailer.NewSender(cfg).Configured() {
		return cfg, nil
	}
	if s.cfg.SMTPGlobalFallback {
		return s.legacyGlobalSMTPConfig(), nil
	}
	return cfg, nil
}

func (s *Server) senderForWorkspace(ctx context.Context, workspaceID int64) (emailer.Sender, error) {
	cfg, err := s.smtpConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return emailer.Sender{}, err
	}
	return emailer.NewSender(cfg), nil
}

func (s *Server) senderForUser(ctx context.Context, userID int64) (emailer.Sender, error) {
	workspaceID := s.store.DefaultWorkspaceForUser(ctx, userID)
	return s.senderForWorkspace(ctx, workspaceID)
}

func (s *Server) absoluteURL(r *http.Request, path string) string {
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/") + path
	}
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host + path
}

func (s *Server) cookie(sessionID string) *http.Cookie {
	// #nosec G124
	return &http.Cookie{Name: "tockr_session", Value: s.sign(sessionID), Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(14 * 24 * time.Hour)}
}

func (s *Server) flashCookie(kind, message string) *http.Cookie {
	// #nosec G124
	body, _ := json.Marshal(flashMessage{Kind: kind, Message: message})
	value := base64.RawURLEncoding.EncodeToString(body)
	return &http.Cookie{Name: flashCookieName, Value: s.sign(value), Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(5 * time.Minute)} // #nosec G124 cookie attributes are set explicitly
}

func (s *Server) clearFlashCookie() *http.Cookie {
	// #nosec G124
	return &http.Cookie{Name: flashCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode}
}

func (s *Server) setFlash(w http.ResponseWriter, kind, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	http.SetCookie(w, s.flashCookie(kind, message))
}

func (s *Server) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	s.setFlash(w, kind, message)
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) templates.Notice {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil {
		return templates.Notice{}
	}
	http.SetCookie(w, s.clearFlashCookie())
	value, ok := s.unsign(cookie.Value)
	if !ok {
		return templates.Notice{}
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return templates.Notice{}
	}
	var flash flashMessage
	if err := json.Unmarshal(body, &flash); err != nil {
		return templates.Notice{}
	}
	return templates.Notice{Kind: flash.Kind, Message: flash.Message}
}

func (s *Server) sign(value string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	_, _ = mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return value + "." + sig
}

func (s *Server) unsign(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	expected := s.sign(parts[0])
	return parts[0], hmac.Equal([]byte(value), []byte(expected))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		s.log.Error("render failed", "err", err)
	}
}

func (s *Server) badRequest(w http.ResponseWriter, _ *http.Request, err error) {
	if err != nil {
		s.log.Warn("bad request", "error_type", fmt.Sprintf("%T", err))
	}
	http.Error(w, "invalid request", http.StatusBadRequest)
}

func (s *Server) serverError(w http.ResponseWriter, _ *http.Request, err error) {
	if err != nil {
		s.log.Error("request failed", "err", err)
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// ─── Workstream handlers ──────────────────────────────────────────────────────

func (s *Server) workstreams(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListWorkstreams(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Workstreams(s.nav(r), items))
}

func (s *Server) saveWorkstream(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ws := &domain.Workstream{
		WorkspaceID: s.access(r).WorkspaceID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: r.FormValue("description"),
		Visible:     checkbox(r, "visible"),
	}
	if id := pathID(r); id > 0 {
		ws.ID = id
		ws.Code = r.FormValue("code")
	}
	if ws.Name == "" {
		s.badRequest(w, r, errors.New("workstream name is required"))
		return
	}
	if err := s.store.UpsertWorkstream(r.Context(), ws); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "upsert", "workstream", &ws.ID, ws.Name)
	http.Redirect(w, r, "/workstreams", http.StatusSeeOther)
}

func (s *Server) deleteWorkstream(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if err := s.store.DeleteWorkstream(r.Context(), s.access(r).WorkspaceID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/workstreams", http.StatusSeeOther)
}

func (s *Server) projectWorkstreams(w http.ResponseWriter, r *http.Request) {
	projectID := pathID(r)
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil || project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return
	}
	assigned, err := s.store.ListProjectWorkstreams(r.Context(), projectID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	all, err := s.store.ListWorkstreams(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ProjectWorkstreams(s.nav(r), project, assigned, all))
}

func (s *Server) saveProjectWorkstream(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := pathID(r)
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil || project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return
	}
	wsID := formInt(r, "workstream_id")
	if wsID == 0 {
		s.badRequest(w, r, errors.New("workstream is required"))
		return
	}
	pw := &domain.ProjectWorkstream{
		ProjectID:    projectID,
		WorkstreamID: wsID,
		BudgetCents:  formIntAny(r, "budget", "budget_cents"),
		Active:       true,
	}
	if err := s.store.UpsertProjectWorkstream(r.Context(), pw); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/workstreams", projectID), http.StatusSeeOther)
}

func (s *Server) removeProjectWorkstream(w http.ResponseWriter, r *http.Request) {
	projectID := pathID(r)
	project, err := s.store.Project(r.Context(), projectID)
	if err != nil || project == nil || project.WorkspaceID != s.access(r).WorkspaceID {
		http.NotFound(w, r)
		return
	}
	wsIDStr := chi.URLParam(r, "wsid")
	wsID, _ := strconv.ParseInt(wsIDStr, 10, 64)
	if wsID == 0 {
		s.badRequest(w, r, errors.New("invalid workstream id"))
		return
	}
	if err := s.store.RemoveProjectWorkstream(r.Context(), projectID, wsID); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/projects/%d/workstreams", projectID), http.StatusSeeOther)
}

// ─── Work schedule settings ────────────────────────────────────────────────────

func (s *Server) workScheduleSettings(w http.ResponseWriter, r *http.Request) {
	schedule := s.store.WorkSchedulePublic(r.Context())
	s.render(w, r, templates.WorkScheduleSettings(s.nav(r), schedule))
}

func (s *Server) saveWorkScheduleSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	hoursStr := r.FormValue("hours_per_day")
	hoursPerDay := 8.0
	if h, err := strconv.ParseFloat(hoursStr, 64); err == nil && h > 0 {
		hoursPerDay = h
	}
	// Parse working days checkboxes: mon=1,tue=2,...,sun=0
	dayNames := map[string]time.Weekday{
		"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
		"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
	}
	var days []time.Weekday
	for name, weekday := range dayNames {
		if r.FormValue("day_"+name) == "1" {
			days = append(days, weekday)
		}
	}
	if len(days) == 0 {
		days = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	}
	schedule := domain.WorkSchedule{WorkingDaysOfWeek: days, WorkingHoursPerDay: hoursPerDay}
	if err := s.store.UpsertWorkSchedule(r.Context(), schedule); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/schedule", http.StatusSeeOther)
}

func (s *Server) emailSettings(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.EmailSettings(s.nav(r), s.store.EmailSettings(r.Context()), s.popFlash(w, r)))
}

func (s *Server) saveEmailSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	settings := domain.EmailSettings{NotifyOldEmailOnChange: checkbox(r, "notify_old_email")}
	if err := s.store.UpsertEmailSettings(r.Context(), settings); err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "update", "email_settings", nil, "")
	s.redirectWithFlash(w, r, "/admin/email", "success", "Email settings saved")
}

func (s *Server) testEmailSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	to := strings.TrimSpace(r.FormValue("to"))
	if _, err := mail.ParseAddress(to); err != nil {
		s.redirectWithFlash(w, r, "/admin/email", "error", "Enter a valid test recipient")
		return
	}
	sender, err := s.senderForWorkspace(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := sender.Send(emailer.Message{
		To:      to,
		Subject: "Tockr SMTP test",
		Text:    "This is a Tockr SMTP test email. If you received it, email sending is working.",
	}); err != nil {
		s.redirectWithFlash(w, r, "/admin/email", "error", "SMTP test failed: "+err.Error())
		return
	}
	s.redirectWithFlash(w, r, "/admin/email", "success", "SMTP test email sent")
}

func (s *Server) adminDemoData(w http.ResponseWriter, r *http.Request) {
	workspaceName := ""
	workspaceID, err := s.store.DefaultWorkspaceForOrganization(r.Context(), s.access(r).OrganizationID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.serverError(w, r, err)
		return
	}
	if workspaceID > 0 {
		workspace, err := s.store.Workspace(r.Context(), workspaceID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		if workspace != nil {
			workspaceName = workspace.Name
		}
	}
	s.render(w, r, templates.AdminDemoData(s.nav(r), workspaceName, s.popFlash(w, r)))
}

func (s *Server) adminDemoDataAdd(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.store.DefaultWorkspaceForOrganization(r.Context(), s.access(r).OrganizationID)
	if err != nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Unable to show demo data: "+err.Error())
		return
	}
	workspace, err := s.store.Workspace(r.Context(), workspaceID)
	if err != nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Unable to show demo data: "+err.Error())
		return
	}
	if workspace == nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Default workspace not found")
		return
	}
	if err := s.store.ClearDemoDataForWorkspace(r.Context(), workspaceID); err != nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Unable to show demo data: "+err.Error())
		return
	}
	if _, err := s.store.SeedDemoDataForWorkspace(r.Context(), workspaceID, s.state(r).User.Email, workspace.Timezone, workspace.DefaultCurrency); err != nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Unable to show demo data: "+err.Error())
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "seed", "demo_data", &workspaceID, "show")
	s.redirectWithFlash(w, r, "/admin/demo-data", "success", "Demo data shown in the default workspace")
}

func (s *Server) adminDemoDataRemove(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.store.ClearDemoData(r.Context(), s.access(r).OrganizationID)
	if err != nil {
		s.redirectWithFlash(w, r, "/admin/demo-data", "error", "Unable to hide demo data: "+err.Error())
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "seed", "demo_data", &workspaceID, "hide")
	s.redirectWithFlash(w, r, "/admin/demo-data", "success", "Demo data hidden from the default workspace")
}

// ─── Saved report edit/delete/share ───────────────────────────────────────────

func (s *Server) editSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	access := s.access(r)
	userID := s.state(r).User.ID
	filter := map[string]string{
		"begin":       r.FormValue("begin"),
		"end":         r.FormValue("end"),
		"customer_id": r.FormValue("customer_id"),
		"project_id":  r.FormValue("project_id"),
		"activity_id": r.FormValue("activity_id"),
		"task_id":     r.FormValue("task_id"),
		"user_id":     r.FormValue("user_id"),
		"group_id":    r.FormValue("group_id"),
		"billable":    r.FormValue("billable"),
	}
	body, err := json.Marshal(filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.store.UpdateSavedReport(r.Context(), access.WorkspaceID, userID, id,
		r.FormValue("name"), defaultString(r.FormValue("group"), "user"), string(body), checkbox(r, "shared")); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) deleteSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	access := s.access(r)
	userID := s.state(r).User.ID
	if err := s.store.DeleteSavedReport(r.Context(), access.WorkspaceID, userID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) shareSavedReport(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	access := s.access(r)
	userID := s.state(r).User.ID
	if r.FormValue("action") == "revoke" {
		if err := s.store.RevokeReportShareToken(r.Context(), access.WorkspaceID, userID, id); err != nil {
			s.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/reports", http.StatusSeeOther)
		return
	}
	token := randomToken(24)
	days := 30
	if v := r.FormValue("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	expiresAt := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	if err := s.store.SetReportShareToken(r.Context(), access.WorkspaceID, userID, id, token, expiresAt); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusSeeOther)
}

func (s *Server) viewSharedReport(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.NotFound(w, r)
		return
	}
	report, err := s.store.FindSharedReport(r.Context(), token)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if report == nil {
		http.NotFound(w, r)
		return
	}
	if report.ShareExpiresAt != nil && report.ShareExpiresAt.Before(time.Now().UTC()) {
		http.Error(w, "This shared report link has expired.", http.StatusGone)
		return
	}
	filter := domain.ReportFilter{}
	if report.FiltersJSON != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(report.FiltersJSON), &m); err == nil {
			filter.Group = m["group"]
		}
	}
	access := domain.AccessContext{WorkspaceID: report.WorkspaceID, UserID: report.UserID, WorkspaceRole: domain.WorkspaceRoleAdmin}
	rows, err := s.store.ListReports(r.Context(), access, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.SharedReport(report.Name, rows))
}

// ─── Utilization dashboard ────────────────────────────────────────────────────

func (s *Server) utilization(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var begin, end *time.Time
	if v := q.Get("begin"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			begin = &t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			end = &t
		}
	}
	rows, err := s.store.UtilizationReport(r.Context(), s.access(r), begin, end)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Utilization(s.nav(r), rows, q.Get("begin"), q.Get("end")))
}

// ─── CSV exports ──────────────────────────────────────────────────────────────

func (s *Server) exportReports(w http.ResponseWriter, r *http.Request) {
	access := s.access(r)
	filter := domain.ReportFilter{
		Group:      defaultString(r.URL.Query().Get("group"), "user"),
		CustomerID: int64Param(r, "customer_id"),
		ProjectID:  int64Param(r, "project_id"),
		ActivityID: int64Param(r, "activity_id"),
		TaskID:     int64Param(r, "task_id"),
		UserID:     int64Param(r, "user_id"),
		GroupID:    int64Param(r, "group_id"),
	}
	filter.Begin = parseDateParam(r, "begin")
	filter.End = parseDateParam(r, "end")
	filter.Billable = parseBoolFilter(r.URL.Query().Get("billable"))
	rows, err := s.store.ListReports(r.Context(), access, filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Name", "Count", "TrackedSeconds", "Amount"})
	for _, row := range rows {
		_ = cw.Write([]string{
			fmt.Sprintf("%v", row["name"]),
			fmt.Sprintf("%v", row["count"]),
			fmt.Sprintf("%v", row["seconds"]),
			fmt.Sprintf("%v", row["cents"]),
		})
	}
	cw.Flush()
}

func (s *Server) exportTimesheets(w http.ResponseWriter, r *http.Request) {
	filter := s.timesheetScope(r)
	filter.CustomerID = int64Param(r, "customer_id")
	filter.ProjectID = int64Param(r, "project_id")
	filter.ActivityID = int64Param(r, "activity_id")
	filter.TaskID = int64Param(r, "task_id")
	filter.Begin = parseDateParam(r, "begin")
	filter.End = parseDateParam(r, "end")
	filter.Page = 1
	filter.Size = 10000
	items, _, err := s.store.ListTimesheets(r.Context(), filter)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="timesheets.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "Date", "User", "Project", "Task", "Description", "Duration", "Billable", "Rate"})
	for _, ts := range items {
		date := ""
		if !ts.StartedAt.IsZero() {
			date = ts.StartedAt.Format("2006-01-02")
		}
		dur := ""
		if ts.DurationSeconds > 0 {
			dur = strconv.FormatInt(ts.DurationSeconds, 10)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(ts.ID, 10),
			date,
			strconv.FormatInt(ts.UserID, 10),
			strconv.FormatInt(ts.ProjectID, 10),
			func() string {
				if ts.TaskID != nil {
					return strconv.FormatInt(*ts.TaskID, 10)
				}
				return ""
			}(),
			ts.Description,
			dur,
			boolText(ts.Billable),
			formatCents(ts.RateCents),
		})
	}
	cw.Flush()
}

// ─── Exchange Rates ───────────────────────────────────────────────────────────

func (s *Server) exchangeRates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.store.ListExchangeRates(r.Context(), s.access(r).WorkspaceID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.ExchangeRates(s.nav(r), rates))
}

func (s *Server) saveExchangeRate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	from := strings.TrimSpace(r.FormValue("from_currency"))
	to := strings.TrimSpace(r.FormValue("to_currency"))
	if len(from) != 3 || len(to) != 3 {
		s.badRequest(w, r, errors.New("currency codes must be 3 letters"))
		return
	}
	rateStr := strings.TrimSpace(r.FormValue("rate"))
	rateFloat, err := strconv.ParseFloat(rateStr, 64)
	if err != nil || rateFloat <= 0 {
		s.badRequest(w, r, errors.New("rate must be a positive number"))
		return
	}
	effStr := strings.TrimSpace(r.FormValue("effective_from"))
	effDate, err := time.Parse("2006-01-02", effStr)
	if err != nil {
		s.badRequest(w, r, errors.New("effective_from must be YYYY-MM-DD"))
		return
	}
	rate := &domain.ExchangeRate{
		WorkspaceID:     s.access(r).WorkspaceID,
		FromCurrency:    strings.ToUpper(from),
		ToCurrency:      strings.ToUpper(to),
		RateThousandths: int64(rateFloat * 1000),
		EffectiveFrom:   effDate.UTC(),
	}
	if err := s.store.UpsertExchangeRate(r.Context(), rate); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/exchange-rates", http.StatusSeeOther)
}

func (s *Server) deleteExchangeRate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteExchangeRate(r.Context(), s.access(r).WorkspaceID, id); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin/exchange-rates", http.StatusSeeOther)
}

// ─── Rate Recalculation ───────────────────────────────────────────────────────

func (s *Server) recalculate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := int64Param(r, "project_id")
	var since *time.Time
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			since = &t
		}
	}
	var preview []domain.RecalcPreviewRow
	if projectID > 0 || since != nil {
		var err error
		preview, err = s.store.RecalcPreview(r.Context(), s.access(r), projectID, since)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	selectors, err := s.selectorData(r, false, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, templates.Recalculate(s.nav(r), preview, selectors, projectID, q.Get("since"), s.popFlash(w, r)))
}

func (s *Server) applyRecalculate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	projectID := formInt(r, "project_id")
	var since *time.Time
	if v := r.FormValue("since"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			since = &t
		}
	}
	if projectID == 0 {
		s.badRequest(w, r, errors.New("project_id is required"))
		return
	}
	count, err := s.store.ApplyRecalc(r.Context(), s.access(r), projectID, since)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	uid := s.state(r).User.ID
	s.store.Audit(r.Context(), &uid, "recalculate", "rates", nil, fmt.Sprintf("updated %d timesheets", count))
	s.redirectWithFlash(w, r, fmt.Sprintf("/admin/recalculate?project_id=%d&since=%s",
		projectID, url.QueryEscape(r.FormValue("since"))), "success", fmt.Sprintf("Recalculated %d timesheets", count))
}

func (s *Server) writeInvoiceFile(inv *domain.Invoice) error {
	ctx := context.Background()
	detail, err := s.store.InvoiceDetails(ctx, inv.ID)
	if err != nil || detail == nil {
		detail = &domain.InvoiceDetail{Invoice: *inv}
	}
	dir := filepath.Join(s.cfg.DataDir, "invoices")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Invoice `)
	sb.WriteString(htmlEscape(inv.Number))
	sb.WriteString(`</title><style>
body{font-family:sans-serif;max-width:800px;margin:40px auto;padding:0 20px;color:#333}
table{width:100%;border-collapse:collapse;margin:20px 0}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid #ddd}
th{background:#f5f5f5;font-weight:600}
.total-row td{font-weight:bold;border-top:2px solid #333}
.header{display:flex;justify-content:space-between;margin-bottom:40px}
.from,.to{min-width:220px}
h1{margin:0 0 4px}
.meta{color:#666;font-size:.9em}
</style></head><body>`)
	sb.WriteString(`<div class="header"><div class="from">`)
	if detail.WorkspaceName != "" {
		sb.WriteString(`<h1>`)
		sb.WriteString(htmlEscape(detail.WorkspaceName))
		sb.WriteString(`</h1>`)
	} else {
		sb.WriteString(`<h1>Invoice</h1>`)
	}
	sb.WriteString(`<div class="meta">Invoice #: `)
	sb.WriteString(htmlEscape(inv.Number))
	sb.WriteString(`<br>Date: `)
	sb.WriteString(inv.CreatedAt.Format("2006-01-02"))
	sb.WriteString(`</div></div>`)
	sb.WriteString(`<div class="to"><strong>Bill To:</strong><br>`)
	sb.WriteString(htmlEscape(detail.Customer.Name))
	if detail.Customer.Email != "" {
		sb.WriteString(`<br>`)
		sb.WriteString(htmlEscape(detail.Customer.Email))
	}
	sb.WriteString(`</div></div>`)
	sb.WriteString(`<table><thead><tr><th>Description</th><th>Qty</th><th>Unit</th><th>Total</th></tr></thead><tbody>`)
	if len(detail.Items) > 0 {
		for _, item := range detail.Items {
			sb.WriteString(`<tr><td>`)
			sb.WriteString(htmlEscape(item.Description))
			sb.WriteString(`</td><td>`)
			sb.WriteString(fmt.Sprintf("%d", item.Quantity))
			sb.WriteString(`</td><td>`)
			sb.WriteString(formatCents(item.UnitCents))
			sb.WriteString(`</td><td>`)
			sb.WriteString(formatCents(item.TotalCents))
			sb.WriteString(`</td></tr>`)
		}
	} else {
		sb.WriteString(`<tr><td colspan="4">(no line items)</td></tr>`)
	}
	sb.WriteString(`</tbody><tfoot>`)
	if inv.TaxCents > 0 {
		sb.WriteString(`<tr><td colspan="3">Subtotal</td><td>`)
		sb.WriteString(formatCents(inv.TotalCents - inv.TaxCents))
		sb.WriteString(`</td></tr><tr><td colspan="3">Tax</td><td>`)
		sb.WriteString(formatCents(inv.TaxCents))
		sb.WriteString(`</td></tr>`)
	}
	sb.WriteString(`<tr class="total-row"><td colspan="3">Total</td><td>`)
	sb.WriteString(formatCents(inv.TotalCents))
	sb.WriteString(`</td></tr></tfoot></table>`)
	sb.WriteString(`</body></html>`)
	// Validate filename before writing to prevent path traversal
	if !isValidInvoiceFilename(inv.Filename) {
		return fmt.Errorf("invalid invoice filename: %s", inv.Filename)
	}
	// #nosec G703
	return os.WriteFile(filepath.Join(dir, inv.Filename), []byte(sb.String()), 0o600)
}

func randomToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// isValidInvoiceFilename validates that an invoice filename is safe for file operations.
// It prevents path traversal attacks by ensuring the filename doesn't contain ".." or "/" separators.
func isValidInvoiceFilename(filename string) bool {
	clean := filepath.Clean(filename)
	// Reject if the filename contains path separators or tries to traverse up
	return filename == clean && !strings.Contains(filename, "/") && !strings.Contains(filename, string(filepath.Separator))
}

func numericCode(digits int) string {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%0*d", digits, n.Int64())
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func formatCents(amount int64) string {
	return fmt.Sprintf("%.2f", float64(amount))
}

func (s *Server) queueEvent(ctx context.Context, workspaceID int64, event string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := s.store.QueueWebhook(ctx, workspaceID, event, body); err != nil {
		s.log.Warn("queue webhook failed", "err", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func redirectOrJSON(w http.ResponseWriter, r *http.Request, location string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func safeReturn(r *http.Request, fallback string) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return fallback
	}
	if parsed, err := url.Parse(ref); err == nil && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/") {
		return parsed.RequestURI()
	}
	if parsed, err := url.Parse(ref); err == nil && parsed.Host == r.Host && strings.HasPrefix(parsed.Path, "/") {
		return parsed.RequestURI()
	}
	return fallback
}

func page(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if value == 0 {
		return 1
	}
	return value
}

func size(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if value == 0 {
		return 25
	}
	return value
}

func pathID(r *http.Request) int64 {
	value, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return value
}

func int64Param(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return value
}

func parseDateParam(r *http.Request, key string) *time.Time {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}

func projectDashboardFilterFromRequest(r *http.Request) domain.ProjectDashboardFilter {
	return domain.ProjectDashboardFilter{
		Begin:        parseDateParam(r, "begin"),
		End:          parseDateParam(r, "end"),
		WorkstreamID: int64Param(r, "workstream_id"),
		ActivityID:   int64Param(r, "activity_id"),
		TaskID:       int64Param(r, "task_id"),
		UserID:       int64Param(r, "user_id"),
		GroupID:      int64Param(r, "group_id"),
	}
}

func formInt(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.FormValue(key), 10, 64)
	return value
}

func formIntAny(r *http.Request, keys ...string) int64 {
	for _, key := range keys {
		value := strings.TrimSpace(r.FormValue(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	}
	return 0
}

func formIntList(r *http.Request, key string) []int64 {
	raw := r.Form[key]
	out := make([]int64, 0, len(raw))
	seen := map[int64]bool{}
	for _, value := range raw {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func formOptionalInt(r *http.Request, key string) *int64 {
	if strings.TrimSpace(r.FormValue(key)) == "" {
		return nil
	}
	value := formInt(r, key)
	return &value
}

func formOptionalIntAny(r *http.Request, keys ...string) *int64 {
	for _, key := range keys {
		if strings.TrimSpace(r.FormValue(key)) == "" {
			continue
		}
		value := formInt(r, key)
		return &value
	}
	return nil
}

func formDateOrEpoch(r *http.Request, key string) time.Time {
	if value := formOptionalDate(r, key); value != nil {
		return *value
	}
	return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
}

func formOptionalDate(r *http.Request, key string) *time.Time {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func checkbox(r *http.Request, key string) bool {
	value := r.FormValue(key)
	return value == "on" || value == "true" || value == "1"
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	previousDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			previousDash = false
			continue
		}
		if !previousDash && b.Len() > 0 {
			b.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func projectTemplateFromForm(r *http.Request) domain.ProjectTemplate {
	return domain.ProjectTemplate{
		Name:               r.FormValue("name"),
		Description:        r.FormValue("description"),
		ProjectName:        r.FormValue("project_name"),
		ProjectNumber:      r.FormValue("project_number"),
		OrderNo:            r.FormValue("order_number"),
		Visible:            checkbox(r, "visible"),
		Private:            checkbox(r, "private"),
		Billable:           checkbox(r, "billable"),
		EstimateSeconds:    formInt(r, "estimate_hours") * 3600,
		BudgetCents:        formIntAny(r, "budget", "budget_cents"),
		BudgetAlertPercent: defaultInt64(formInt(r, "budget_alert_percent"), 80),
		Archived:           checkbox(r, "archived"),
		Tasks:              projectTemplateTasksFromText(r.FormValue("tasks")),
		Activities:         projectTemplateActivitiesFromText(r.FormValue("activities")),
	}
}

func projectTemplateTasksFromText(value string) []domain.ProjectTemplateTask {
	lines := splitCSV(strings.ReplaceAll(value, "\n", ","))
	tasks := make([]domain.ProjectTemplateTask, 0, len(lines))
	for _, line := range lines {
		tasks = append(tasks, domain.ProjectTemplateTask{Name: line, Visible: true, Billable: true})
	}
	return tasks
}

func projectTemplateActivitiesFromText(value string) []domain.ProjectTemplateActivity {
	lines := splitCSV(strings.ReplaceAll(value, "\n", ","))
	activities := make([]domain.ProjectTemplateActivity, 0, len(lines))
	for _, line := range lines {
		activities = append(activities, domain.ProjectTemplateActivity{Name: line, Visible: true, Billable: true})
	}
	return activities
}

func defaultInt64(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	return value
}

type parsedTimesheetTime struct {
	Start        time.Time
	End          time.Time
	BreakSeconds int64
}

func parseTimesheetTime(r *http.Request) (parsedTimesheetTime, error) {
	mode := strings.ToLower(strings.TrimSpace(r.FormValue("entry_mode")))
	if mode == "" && (strings.TrimSpace(r.FormValue("start")) != "" || strings.TrimSpace(r.FormValue("end")) != "") {
		mode = "range"
	}
	if mode == "" {
		mode = "manual"
	}
	switch mode {
	case "manual":
		return parseManualTimesheetTime(r.FormValue("date"), r.FormValue("hours"), r.FormValue("minutes"))
	case "range":
		start, end, err := parseRange(r.FormValue("start"), r.FormValue("end"))
		if err != nil {
			return parsedTimesheetTime{}, err
		}
		breakMinutes, err := parseNonNegativeInt(r.FormValue("break_minutes"), "break minutes")
		if err != nil {
			return parsedTimesheetTime{}, err
		}
		breakSeconds := breakMinutes * 60
		if int64(end.Sub(start).Seconds())-breakSeconds <= 0 {
			return parsedTimesheetTime{}, errors.New("duration must be greater than zero")
		}
		return parsedTimesheetTime{Start: start, End: end, BreakSeconds: breakSeconds}, nil
	default:
		return parsedTimesheetTime{}, errors.New("valid entry mode is required")
	}
}

func parseManualTimesheetTime(dateValue, hoursValue, minutesValue string) (parsedTimesheetTime, error) {
	dateValue = strings.TrimSpace(dateValue)
	if dateValue == "" {
		return parsedTimesheetTime{}, errors.New("date is required")
	}
	start, err := time.ParseInLocation("2006-01-02", dateValue, time.Local)
	if err != nil {
		return parsedTimesheetTime{}, errors.New("valid date is required")
	}
	hours, err := parseNonNegativeInt(hoursValue, "hours")
	if err != nil {
		return parsedTimesheetTime{}, err
	}
	minutes, err := parseNonNegativeInt(minutesValue, "minutes")
	if err != nil {
		return parsedTimesheetTime{}, err
	}
	if minutes > 59 {
		return parsedTimesheetTime{}, errors.New("minutes must be between 0 and 59")
	}
	totalSeconds := hours*3600 + minutes*60
	if totalSeconds <= 0 {
		return parsedTimesheetTime{}, errors.New("duration must be greater than zero")
	}
	end := start.Add(time.Duration(totalSeconds) * time.Second)
	return parsedTimesheetTime{Start: start.UTC(), End: end.UTC()}, nil
}

func parseNonNegativeInt(value, label string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "0"
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("valid %s are required", label)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s cannot be negative", label)
	}
	return parsed, nil
}

func parseRange(startValue, endValue string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01-02T15:04", startValue, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.ParseInLocation("2006-01-02T15:04", endValue, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("end must be after start")
	}
	return start.UTC(), end.UTC(), nil
}

func weekStart(value time.Time) time.Time {
	value = value.UTC()
	y, m, d := value.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return day.AddDate(0, 0, 1-weekday)
}

func checkFuturePolicy(policy string, start, end time.Time) error {
	now := time.Now().UTC()
	switch policy {
	case "allow":
		return nil
	case "deny":
		if start.After(now) || end.After(now) {
			return errors.New("future times are not allowed")
		}
	case "end_of_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		limit := now.AddDate(0, 0, 7-weekday).Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
		if start.After(limit) || end.After(limit) {
			return errors.New("future times after this week are not allowed")
		}
	default:
		limit := now.Truncate(24 * time.Hour).Add(24*time.Hour - time.Second)
		if start.After(limit) || end.After(limit) {
			return errors.New("future times after today are not allowed")
		}
	}
	return nil
}

func dashboardRecentFromTimesheets(items []domain.Timesheet) []domain.DashboardRecentWork {
	recent := make([]domain.DashboardRecentWork, 0, minInt(len(items), 8))
	for _, item := range items {
		recent = append(recent, domain.DashboardRecentWork{
			TimesheetID:     item.ID,
			CustomerID:      item.CustomerID,
			ProjectID:       item.ProjectID,
			WorkstreamID:    item.WorkstreamID,
			ActivityID:      item.ActivityID,
			TaskID:          item.TaskID,
			Description:     item.Description,
			DurationSeconds: item.DurationSeconds,
			StartedAt:       item.StartedAt,
			Billable:        item.Billable,
			Exported:        item.Exported,
		})
		if len(recent) == 8 {
			break
		}
	}
	return recent
}

func timesheetPrefillFromRequest(r *http.Request) templates.TimesheetPrefill {
	q := r.URL.Query()
	today := time.Now().UTC().Format("2006-01-02")
	entryMode := strings.ToLower(strings.TrimSpace(q.Get("entry_mode")))
	if entryMode != "range" {
		entryMode = "manual"
	}
	prefill := templates.TimesheetPrefill{
		EntryMode:     entryMode,
		CustomerID:    int64Param(r, "customer_id"),
		ProjectID:     int64Param(r, "project_id"),
		WorkstreamID:  int64Param(r, "workstream_id"),
		ActivityID:    int64Param(r, "activity_id"),
		TaskID:        int64Param(r, "task_id"),
		Date:          defaultString(strings.TrimSpace(q.Get("date")), today),
		ManualHours:   defaultString(strings.TrimSpace(q.Get("hours")), "0"),
		ManualMinutes: defaultString(strings.TrimSpace(q.Get("minutes")), "0"),
		Description:   strings.TrimSpace(q.Get("description")),
		Tags:          strings.TrimSpace(q.Get("tags")),
		Billable:      true,
	}
	if breakMinutes := strings.TrimSpace(q.Get("break_minutes")); breakMinutes != "" {
		prefill.BreakMinutes = breakMinutes
	} else {
		prefill.BreakMinutes = "0"
	}
	if billable := parseBoolFilter(q.Get("billable")); billable != nil {
		prefill.Billable = *billable
	}
	if start := q.Get("start"); start != "" {
		prefill.Start = start
		if q.Get("entry_mode") == "" {
			prefill.EntryMode = "range"
		}
	}
	if end := q.Get("end"); end != "" {
		prefill.End = end
		if q.Get("entry_mode") == "" {
			prefill.EntryMode = "range"
		}
	}
	if prefill.Start == "" {
		prefill.Start = prefill.Date + "T08:00"
	}
	if prefill.End == "" {
		prefill.End = prefill.Date + "T17:00"
	}
	return prefill
}

func timesheetPrefillFromEntry(entry domain.Timesheet) templates.TimesheetPrefill {
	tags := make([]string, 0, len(entry.Tags))
	for _, tag := range entry.Tags {
		if strings.TrimSpace(tag.Name) != "" {
			tags = append(tags, tag.Name)
		}
	}
	durationSeconds := entry.DurationSeconds
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	entryMode := "range"
	localStart := entry.StartedAt.Local()
	if entry.BreakSeconds == 0 && localStart.Hour() == 0 && localStart.Minute() == 0 && localStart.Second() == 0 {
		entryMode = "manual"
	}
	prefill := templates.TimesheetPrefill{
		EntryID:       entry.ID,
		EntryMode:     entryMode,
		CustomerID:    entry.CustomerID,
		ProjectID:     entry.ProjectID,
		WorkstreamID:  valueOrZero(entry.WorkstreamID),
		ActivityID:    entry.ActivityID,
		Description:   entry.Description,
		Date:          localStart.Format("2006-01-02"),
		ManualHours:   fmt.Sprint(durationSeconds / 3600),
		ManualMinutes: fmt.Sprint((durationSeconds % 3600) / 60),
		Start:         entry.StartedAt.Local().Format("2006-01-02T15:04"),
		BreakMinutes:  fmt.Sprint(entry.BreakSeconds / 60),
		Tags:          strings.Join(tags, ","),
		Billable:      entry.Billable,
	}
	if entry.TaskID != nil {
		prefill.TaskID = *entry.TaskID
	}
	if entry.EndedAt != nil {
		prefill.End = entry.EndedAt.Local().Format("2006-01-02T15:04")
	}
	return prefill
}

func timesheetPrefillFromForm(r *http.Request) templates.TimesheetPrefill {
	entryMode := strings.ToLower(strings.TrimSpace(r.FormValue("entry_mode")))
	if entryMode != "range" {
		entryMode = "manual"
	}
	prefill := templates.TimesheetPrefill{
		EntryMode:     entryMode,
		CustomerID:    formInt(r, "customer_id"),
		ProjectID:     formInt(r, "project_id"),
		WorkstreamID:  formInt(r, "workstream_id"),
		ActivityID:    formInt(r, "activity_id"),
		Date:          strings.TrimSpace(r.FormValue("date")),
		ManualHours:   strings.TrimSpace(defaultString(r.FormValue("hours"), "0")),
		ManualMinutes: strings.TrimSpace(defaultString(r.FormValue("minutes"), "0")),
		Start:         strings.TrimSpace(r.FormValue("start")),
		End:           strings.TrimSpace(r.FormValue("end")),
		BreakMinutes:  strings.TrimSpace(defaultString(r.FormValue("break_minutes"), "0")),
		Description:   strings.TrimSpace(r.FormValue("description")),
		Tags:          strings.TrimSpace(r.FormValue("tags")),
		Billable:      checkbox(r, "billable"),
	}
	if taskID := formOptionalInt(r, "task_id"); taskID != nil {
		prefill.TaskID = *taskID
	}
	return prefill
}

func parseBoolFilter(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "billable":
		v := true
		return &v
	case "0", "false", "no", "nonbillable", "non-billable":
		v := false
		return &v
	default:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func ptrText(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(*value)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
