package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"tockr/internal/auth"
	"tockr/internal/domain"
)

type Store struct {
	db *sql.DB
}

type Session struct {
	ID          string
	UserID      int64
	WorkspaceID int64
	CSRFToken   string
	ExpiresAt   time.Time
}

type TimesheetFilter struct {
	WorkspaceID int64
	UserID      int64
	CustomerID  int64
	ProjectID   int64
	ProjectIDs  []int64
	ActivityID  int64
	TaskID      int64
	GroupID     int64
	Begin       *time.Time
	End         *time.Time
	Exported    *bool
	Billable    *bool
	Page        int
	Size        int
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, query := range pragmas {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	return s.ensureHierarchy(ctx)
}

func (s *Store) SeedAdmin(ctx context.Context, email, password, timezone, currency string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := utcNow()
	var orgID, workspaceID int64 = 1, 1
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM organizations ORDER BY id LIMIT 1`).Scan(&orgID)
	_ = s.db.QueryRowContext(ctx, `SELECT id FROM workspaces ORDER BY id LIMIT 1`).Scan(&workspaceID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	res, err := tx.ExecContext(ctx, `INSERT INTO users(organization_id, email, username, display_name, password_hash, timezone, enabled, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		orgID, email, "admin", "Administrator", hash, timezone, 1, now)
	if err != nil {
		return err
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, role := range []domain.Role{domain.RoleSuperAdmin} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id, role_id) SELECT ?, id FROM roles WHERE name = ?`, userID, string(role)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organization_members(organization_id, user_id, role, created_at) VALUES(?,?,?,?)`, orgID, userID, string(domain.OrgRoleOwner), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at) VALUES(?,?,?,?)`, workspaceID, userID, string(domain.WorkspaceRoleAdmin), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(name, value) VALUES('default_currency', ?), ('future_time_policy', ?)`, currency, "end_of_day"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, entity string, entityID *int64, detail string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_log(user_id, action, entity, entity_id, detail, created_at) VALUES(?,?,?,?,?,?)`,
		userID, action, entity, entityID, detail, utcNow())
}

func (s *Store) AccessForUser(ctx context.Context, userID int64) (domain.AccessContext, error) {
	return s.AccessForUserWorkspace(ctx, userID, 0)
}

func (s *Store) AccessForUserWorkspace(ctx context.Context, userID, workspaceID int64) (domain.AccessContext, error) {
	access := domain.AccessContext{
		UserID:            userID,
		ManagedProjectIDs: map[int64]bool{},
		MemberProjectIDs:  map[int64]bool{},
	}
	row := s.db.QueryRowContext(ctx, `SELECT u.organization_id, COALESCE(om.role,'') FROM users u LEFT JOIN organization_members om ON om.user_id=u.id AND om.organization_id=u.organization_id WHERE u.id=?`, userID)
	var orgRole string
	if err := row.Scan(&access.OrganizationID, &orgRole); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return access, nil
		}
		return access, err
	}
	access.OrganizationRole = domain.OrganizationRole(orgRole)
	var workspaceRole string
	if workspaceID > 0 {
		err := s.db.QueryRowContext(ctx, `SELECT w.id, COALESCE(wm.role,'') FROM workspaces w LEFT JOIN workspace_members wm ON wm.workspace_id=w.id AND wm.user_id=? WHERE w.id=?`, userID, workspaceID).
			Scan(&access.WorkspaceID, &workspaceRole)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return access, err
		}
		if access.WorkspaceID > 0 && workspaceRole == "" && !access.IsOrgAdmin() {
			access.WorkspaceID = 0
		}
	} else {
		err := s.db.QueryRowContext(ctx, `SELECT w.id, wm.role FROM workspace_members wm JOIN workspaces w ON w.id=wm.workspace_id WHERE wm.user_id=? ORDER BY w.id LIMIT 1`, userID).
			Scan(&access.WorkspaceID, &workspaceRole)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return access, err
		}
	}
	access.WorkspaceRole = domain.WorkspaceRole(workspaceRole)
	if access.WorkspaceID == 0 && access.IsOrgAdmin() {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE organization_id=? ORDER BY id LIMIT 1`, access.OrganizationID).Scan(&access.WorkspaceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return access, err
		}
		access.WorkspaceRole = domain.WorkspaceRoleAdmin
	}
	rows, err := s.db.QueryContext(ctx, `SELECT pm.project_id, pm.role FROM project_members pm JOIN projects p ON p.id=pm.project_id WHERE pm.user_id=? AND p.workspace_id=?`, userID, access.WorkspaceID)
	if err != nil {
		return access, err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int64
		var role string
		if err := rows.Scan(&projectID, &role); err != nil {
			return access, err
		}
		if domain.ProjectRole(role) == domain.ProjectRoleManager {
			access.ManagedProjectIDs[projectID] = true
		}
		access.MemberProjectIDs[projectID] = true
	}
	if err := rows.Err(); err != nil {
		return access, err
	}
	groupRows, err := s.db.QueryContext(ctx, `SELECT DISTINCT pg.project_id FROM group_members gm JOIN project_groups pg ON pg.group_id=gm.group_id JOIN projects p ON p.id=pg.project_id WHERE gm.user_id=? AND p.workspace_id=?`, userID, access.WorkspaceID)
	if err != nil {
		return access, err
	}
	defer groupRows.Close()
	for groupRows.Next() {
		var projectID int64
		if err := groupRows.Scan(&projectID); err != nil {
			return access, err
		}
		access.MemberProjectIDs[projectID] = true
	}
	return access, groupRows.Err()
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.scanUser(ctx, `WHERE lower(email)=lower(?)`, email)
}

func (s *Store) FindUserByID(ctx context.Context, id int64) (*domain.User, error) {
	return s.scanUser(ctx, `WHERE id=?`, id)
}

func (s *Store) scanUser(ctx context.Context, where string, args ...any) (*domain.User, error) {
	q := `SELECT id, organization_id, email, username, display_name, password_hash, timezone, enabled, totp_secret, totp_enabled, created_at, last_login_at FROM users ` + where
	row := s.db.QueryRowContext(ctx, q, args...)
	var u domain.User
	var created string
	var last sql.NullString
	if err := row.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Timezone, &u.Enabled, &u.TOTPSecret, &u.TOTPEnabled, &created, &last); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.CreatedAt = parseTime(created)
	if last.Valid {
		t := parseTime(last.String)
		u.LastLoginAt = &t
	}
	roles, err := s.userRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return &u, nil
}

func (s *Store) userRoles(ctx context.Context, userID int64) ([]domain.Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.name FROM roles r JOIN user_roles ur ON ur.role_id=r.id WHERE ur.user_id=? ORDER BY r.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []domain.Role
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, domain.Role(role))
	}
	return roles, rows.Err()
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, organization_id, email, username, display_name, password_hash, timezone, enabled, totp_secret, totp_enabled, created_at, last_login_at FROM users ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	var users []domain.User
	for rows.Next() {
		var u domain.User
		var created string
		var last sql.NullString
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Timezone, &u.Enabled, &u.TOTPSecret, &u.TOTPEnabled, &created, &last); err != nil {
			return nil, err
		}
		u.CreatedAt = parseTime(created)
		if last.Valid {
			t := parseTime(last.String)
			u.LastLoginAt = &t
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range users {
		roles, err := s.userRoles(ctx, users[i].ID)
		if err != nil {
			return nil, err
		}
		users[i].Roles = roles
	}
	return users, nil
}

func (s *Store) ListWorkspaceUsers(ctx context.Context, workspaceID int64) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.organization_id, u.email, u.username, u.display_name, u.password_hash, u.timezone, u.enabled, u.totp_secret, u.totp_enabled, u.created_at, u.last_login_at
		FROM users u
		JOIN workspace_members wm ON wm.user_id=u.id
		WHERE wm.workspace_id=?
		ORDER BY u.display_name`, workspaceID)
	if err != nil {
		return nil, err
	}
	var users []domain.User
	for rows.Next() {
		var u domain.User
		var created string
		var last sql.NullString
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Timezone, &u.Enabled, &u.TOTPSecret, &u.TOTPEnabled, &created, &last); err != nil {
			_ = rows.Close()
			return nil, err
		}
		u.CreatedAt = parseTime(created)
		if last.Valid {
			t := parseTime(last.String)
			u.LastLoginAt = &t
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range users {
		roles, err := s.userRoles(ctx, users[i].ID)
		if err != nil {
			return nil, err
		}
		users[i].Roles = roles
	}
	return users, nil
}

func (s *Store) CreateUser(ctx context.Context, u domain.User, password string, roles []domain.Role) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if u.OrganizationID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT id FROM organizations ORDER BY id LIMIT 1`).Scan(&u.OrganizationID)
		if u.OrganizationID == 0 {
			u.OrganizationID = 1
		}
	}
	now := utcNow()
	res, err := tx.ExecContext(ctx, `INSERT INTO users(organization_id, email, username, display_name, password_hash, timezone, enabled, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		u.OrganizationID, u.Email, u.Username, u.DisplayName, hash, defaultString(u.Timezone, "UTC"), boolInt(u.Enabled), now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, role := range roles {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(user_id, role_id) SELECT ?, id FROM roles WHERE name=?`, id, string(role)); err != nil {
			return err
		}
	}
	if orgRole := organizationRoleFromLegacy(roles); orgRole != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organization_members(organization_id, user_id, role, created_at) VALUES(?,?,?,?)`, u.OrganizationID, id, string(orgRole), now); err != nil {
			return err
		}
	}
	var workspaceID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE organization_id=? ORDER BY id LIMIT 1`, u.OrganizationID).Scan(&workspaceID); err == nil && workspaceID > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at) VALUES(?,?,?,?)`, workspaceID, id, string(workspaceRoleFromLegacy(roles)), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateProfile(ctx context.Context, userID int64, displayName, timezone string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET display_name=?, timezone=? WHERE id=?`, strings.TrimSpace(displayName), defaultString(strings.TrimSpace(timezone), "UTC"), userID)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, userID int64, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, userID)
	return err
}

func (s *Store) EnableTOTP(ctx context.Context, userID int64, secret string, recoveryCodes []string) error {
	hashes := []string{}
	for _, code := range recoveryCodes {
		hash, err := auth.HashPassword(code)
		if err != nil {
			return err
		}
		hashes = append(hashes, hash)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret=?, totp_enabled=1, totp_recovery_hashes=? WHERE id=?`, secret, strings.Join(hashes, "\n"), userID)
	return err
}

func (s *Store) DisableTOTP(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret='', totp_enabled=0, totp_recovery_hashes='' WHERE id=?`, userID)
	return err
}

func (s *Store) UseRecoveryCode(ctx context.Context, userID int64, code string) (bool, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT totp_recovery_hashes FROM users WHERE id=?`, userID).Scan(&raw); err != nil {
		return false, err
	}
	hashes := splitLines(raw)
	for index, hash := range hashes {
		if auth.CheckPassword(hash, code) {
			hashes = append(hashes[:index], hashes[index+1:]...)
			_, err := s.db.ExecContext(ctx, `UPDATE users SET totp_recovery_hashes=? WHERE id=?`, strings.Join(hashes, "\n"), userID)
			return true, err
		}
	}
	return false, nil
}

func splitLines(value string) []string {
	raw := strings.Split(value, "\n")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func hasOrgAdminMembership(ctx context.Context, db *sql.DB, userID, organizationID int64) bool {
	var role string
	err := db.QueryRowContext(ctx, `SELECT role FROM organization_members WHERE user_id=? AND organization_id=?`, userID, organizationID).Scan(&role)
	return err == nil && (role == string(domain.OrgRoleOwner) || role == string(domain.OrgRoleAdmin))
}

func (s *Store) ListUserWorkspaces(ctx context.Context, userID int64) ([]domain.Workspace, error) {
	user, err := s.FindUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, err
	}
	query := `SELECT w.id, w.organization_id, w.name, w.slug, w.default_currency, w.timezone, w.created_at
		FROM workspaces w
		JOIN workspace_members wm ON wm.workspace_id=w.id
		WHERE wm.user_id=?
		ORDER BY w.name`
	args := []any{userID}
	if hasOrgAdminMembership(ctx, s.db, userID, user.OrganizationID) {
		query = `SELECT w.id, w.organization_id, w.name, w.slug, w.default_currency, w.timezone, w.created_at
			FROM workspaces w
			WHERE w.organization_id=?
			ORDER BY w.name`
		args = []any{user.OrganizationID}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := []domain.Workspace{}
	for rows.Next() {
		var w domain.Workspace
		var created string
		if err := rows.Scan(&w.ID, &w.OrganizationID, &w.Name, &w.Slug, &w.DefaultCurrency, &w.Timezone, &created); err != nil {
			return nil, err
		}
		w.CreatedAt = parseTime(created)
		workspaces = append(workspaces, w)
	}
	return workspaces, rows.Err()
}

func (s *Store) Workspace(ctx context.Context, id int64) (*domain.Workspace, error) {
	var w domain.Workspace
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, organization_id, name, slug, default_currency, timezone, created_at FROM workspaces WHERE id=?`, id).
		Scan(&w.ID, &w.OrganizationID, &w.Name, &w.Slug, &w.DefaultCurrency, &w.Timezone, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	w.CreatedAt = parseTime(created)
	return &w, nil
}

func (s *Store) UserCanAccessWorkspace(ctx context.Context, userID, workspaceID int64) (bool, error) {
	workspaces, err := s.ListUserWorkspaces(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) SwitchSessionWorkspace(ctx context.Context, sessionID string, workspaceID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET workspace_id=? WHERE id=?`, workspaceID, sessionID)
	return err
}

func (s *Store) defaultWorkspaceForUser(ctx context.Context, userID int64) int64 {
	var workspaceID int64
	if err := s.db.QueryRowContext(ctx, `SELECT workspace_id FROM workspace_members WHERE user_id=? ORDER BY workspace_id LIMIT 1`, userID).Scan(&workspaceID); err == nil && workspaceID > 0 {
		return workspaceID
	}
	var organizationID int64
	if err := s.db.QueryRowContext(ctx, `SELECT organization_id FROM users WHERE id=?`, userID).Scan(&organizationID); err == nil && organizationID > 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE organization_id=? ORDER BY id LIMIT 1`, organizationID).Scan(&workspaceID); err == nil && workspaceID > 0 {
			return workspaceID
		}
	}
	return 1
}

func (s *Store) ListGroups(ctx context.Context, workspaceID int64) ([]domain.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, name, description, created_at FROM groups WHERE workspace_id=? ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []domain.Group
	for rows.Next() {
		var g domain.Group
		var created string
		if err := rows.Scan(&g.ID, &g.WorkspaceID, &g.Name, &g.Description, &created); err != nil {
			return nil, err
		}
		g.CreatedAt = parseTime(created)
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, workspaceID int64, name, description string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO groups(workspace_id, name, description, created_at) VALUES(?,?,?,?)`, workspaceID, strings.TrimSpace(name), strings.TrimSpace(description), utcNow())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AddUserToGroup(ctx context.Context, groupID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO group_members(group_id, user_id, created_at) VALUES(?,?,?)`, groupID, userID, utcNow())
	return err
}

func (s *Store) AddProjectMember(ctx context.Context, projectID, userID int64, role domain.ProjectRole) error {
	if role == "" {
		role = domain.ProjectRoleMember
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO project_members(project_id, user_id, role, created_at) VALUES(?,?,?,?) ON CONFLICT(project_id, user_id) DO UPDATE SET role=excluded.role`, projectID, userID, string(role), utcNow())
	return err
}

func (s *Store) RemoveProjectMember(ctx context.Context, projectID, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM project_members WHERE project_id=? AND user_id=?`, projectID, userID)
	return err
}

func (s *Store) AddGroupToProject(ctx context.Context, projectID, groupID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO project_groups(project_id, group_id, created_at) VALUES(?,?,?)`, projectID, groupID, utcNow())
	return err
}

func (s *Store) RemoveGroupFromProject(ctx context.Context, projectID, groupID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM project_groups WHERE project_id=? AND group_id=?`, projectID, groupID)
	return err
}

func (s *Store) ListProjectMembers(ctx context.Context, projectID int64) ([]domain.ProjectMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, user_id, role, created_at FROM project_members WHERE project_id=? ORDER BY user_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []domain.ProjectMember{}
	for rows.Next() {
		var member domain.ProjectMember
		var role, created string
		if err := rows.Scan(&member.ProjectID, &member.UserID, &role, &created); err != nil {
			return nil, err
		}
		member.Role = domain.ProjectRole(role)
		member.CreatedAt = parseTime(created)
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) ListProjectGroups(ctx context.Context, projectID int64) ([]domain.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.workspace_id, g.name, g.description, g.created_at FROM groups g JOIN project_groups pg ON pg.group_id=g.id WHERE pg.project_id=? ORDER BY g.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []domain.Group{}
	for rows.Next() {
		var group domain.Group
		var created string
		if err := rows.Scan(&group.ID, &group.WorkspaceID, &group.Name, &group.Description, &created); err != nil {
			return nil, err
		}
		group.CreatedAt = parseTime(created)
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (s *Store) ListFavorites(ctx context.Context, workspaceID, userID int64) ([]domain.Favorite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, user_id, name, customer_id, project_id, activity_id, task_id, description, tags, created_at FROM favorites WHERE workspace_id=? AND user_id=? ORDER BY id DESC`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var favorites []domain.Favorite
	for rows.Next() {
		var f domain.Favorite
		var task sql.NullInt64
		var created string
		if err := rows.Scan(&f.ID, &f.WorkspaceID, &f.UserID, &f.Name, &f.CustomerID, &f.ProjectID, &f.ActivityID, &task, &f.Description, &f.Tags, &created); err != nil {
			return nil, err
		}
		f.TaskID = nullableInt(task)
		f.CreatedAt = parseTime(created)
		favorites = append(favorites, f)
	}
	return favorites, rows.Err()
}

func (s *Store) Favorite(ctx context.Context, workspaceID, userID, id int64) (*domain.Favorite, error) {
	var f domain.Favorite
	var task sql.NullInt64
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, user_id, name, customer_id, project_id, activity_id, task_id, description, tags, created_at FROM favorites WHERE workspace_id=? AND user_id=? AND id=?`, workspaceID, userID, id).
		Scan(&f.ID, &f.WorkspaceID, &f.UserID, &f.Name, &f.CustomerID, &f.ProjectID, &f.ActivityID, &task, &f.Description, &f.Tags, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	f.TaskID = nullableInt(task)
	f.CreatedAt = parseTime(created)
	return &f, nil
}

func (s *Store) CreateFavorite(ctx context.Context, f *domain.Favorite) error {
	if f.WorkspaceID == 0 {
		f.WorkspaceID = 1
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO favorites(workspace_id, user_id, name, customer_id, project_id, activity_id, task_id, description, tags, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		f.WorkspaceID, f.UserID, f.Name, f.CustomerID, f.ProjectID, f.ActivityID, f.TaskID, f.Description, f.Tags, utcNow())
	if err != nil {
		return err
	}
	f.ID, err = res.LastInsertId()
	return err
}

func (s *Store) TouchLogin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, utcNow(), userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID, workspaceID int64, ttl time.Duration) (*Session, error) {
	if workspaceID == 0 {
		workspaceID = s.defaultWorkspaceForUser(ctx, userID)
	}
	session := &Session{ID: randomToken(32), UserID: userID, WorkspaceID: workspaceID, CSRFToken: randomToken(32), ExpiresAt: time.Now().UTC().Add(ttl)}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id, user_id, workspace_id, csrf_token, expires_at, created_at) VALUES(?,?,?,?,?,?)`,
		session.ID, session.UserID, session.WorkspaceID, session.CSRFToken, formatTime(session.ExpiresAt), utcNow())
	return session, err
}

func (s *Store) FindSession(ctx context.Context, id string) (*Session, error) {
	var session Session
	var expires string
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, workspace_id, csrf_token, expires_at FROM sessions WHERE id=?`, id).Scan(&session.ID, &session.UserID, &session.WorkspaceID, &session.CSRFToken, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	session.ExpiresAt = parseTime(expires)
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.DeleteSession(ctx, id)
		return nil, nil
	}
	return &session, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}

func (s *Store) UpsertCustomer(ctx context.Context, c *domain.Customer) error {
	now := utcNow()
	if c.WorkspaceID == 0 {
		c.WorkspaceID = 1
	}
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO customers(workspace_id, name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.WorkspaceID, c.Name, c.Number, c.Company, c.Contact, c.Email, defaultString(c.Currency, "USD"), defaultString(c.Timezone, "UTC"), boolInt(c.Visible), boolInt(c.Billable), c.Comment, c.LegacyJSON, now)
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE customers SET workspace_id=?, name=?, number=?, company=?, contact=?, email=?, currency=?, timezone=?, visible=?, billable=?, comment=?, legacy_json=? WHERE id=?`,
		c.WorkspaceID, c.Name, c.Number, c.Company, c.Contact, c.Email, c.Currency, c.Timezone, boolInt(c.Visible), boolInt(c.Billable), c.Comment, c.LegacyJSON, c.ID)
	return err
}

func (s *Store) ListCustomers(ctx context.Context, access domain.AccessContext, term string, page, size int) ([]domain.Customer, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := scopedSearchWhere("workspace_id", access.WorkspaceID, "name", term)
	if !access.IsWorkspaceAdmin() {
		where += ` AND EXISTS (
			SELECT 1 FROM projects p
			WHERE p.customer_id=customers.id AND p.workspace_id=customers.workspace_id
			AND (p.private=0 OR p.id IN (SELECT project_id FROM project_members WHERE user_id=?) OR p.id IN (SELECT pg.project_id FROM project_groups pg JOIN group_members gm ON gm.group_id=pg.group_id WHERE gm.user_id=?))
		)`
		args = append(args, access.UserID, access.UserID)
	}
	total, err := s.count(ctx, "customers", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at FROM customers `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	items := []domain.Customer{}
	for rows.Next() {
		var c domain.Customer
		var created string
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Number, &c.Company, &c.Contact, &c.Email, &c.Currency, &c.Timezone, &c.Visible, &c.Billable, &c.Comment, &c.LegacyJSON, &created); err != nil {
			return nil, domain.Page{}, err
		}
		c.CreatedAt = parseTime(created)
		items = append(items, c)
	}
	return items, makePage(page, size, total), rows.Err()
}

func (s *Store) Customer(ctx context.Context, id int64) (*domain.Customer, error) {
	var c domain.Customer
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at FROM customers WHERE id=?`, id).
		Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Number, &c.Company, &c.Contact, &c.Email, &c.Currency, &c.Timezone, &c.Visible, &c.Billable, &c.Comment, &c.LegacyJSON, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	c.CreatedAt = parseTime(created)
	return &c, nil
}

func (s *Store) UpsertProject(ctx context.Context, p *domain.Project) error {
	now := utcNow()
	if p.WorkspaceID == 0 {
		p.WorkspaceID = 1
	}
	if p.ID == 0 {
		if p.BudgetAlertPercent == 0 {
			p.BudgetAlertPercent = 80
		}
		res, err := s.db.ExecContext(ctx, `INSERT INTO projects(workspace_id, customer_id, name, number, order_number, visible, private, billable, estimate_seconds, budget_cents, budget_alert_percent, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.WorkspaceID, p.CustomerID, p.Name, p.Number, p.OrderNo, boolInt(p.Visible), boolInt(p.Private), boolInt(p.Billable), p.EstimateSeconds, p.BudgetCents, p.BudgetAlertPercent, p.Comment, p.LegacyJSON, now)
		if err != nil {
			return err
		}
		p.ID, err = res.LastInsertId()
		return err
	}
	if p.BudgetAlertPercent == 0 {
		p.BudgetAlertPercent = 80
	}
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET workspace_id=?, customer_id=?, name=?, number=?, order_number=?, visible=?, private=?, billable=?, estimate_seconds=?, budget_cents=?, budget_alert_percent=?, comment=?, legacy_json=? WHERE id=?`,
		p.WorkspaceID, p.CustomerID, p.Name, p.Number, p.OrderNo, boolInt(p.Visible), boolInt(p.Private), boolInt(p.Billable), p.EstimateSeconds, p.BudgetCents, p.BudgetAlertPercent, p.Comment, p.LegacyJSON, p.ID)
	return err
}

func (s *Store) ListProjects(ctx context.Context, access domain.AccessContext, customerID int64, term string, page, size int) ([]domain.Project, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := scopedSearchWhere("workspace_id", access.WorkspaceID, "name", term)
	if customerID > 0 {
		if where == "" {
			where = "WHERE customer_id=?"
		} else {
			where += " AND customer_id=?"
		}
		args = append(args, customerID)
	}
	if !access.IsWorkspaceAdmin() {
		where += ` AND (private=0 OR id IN (SELECT project_id FROM project_members WHERE user_id=?) OR id IN (SELECT pg.project_id FROM project_groups pg JOIN group_members gm ON gm.group_id=pg.group_id WHERE gm.user_id=?))`
		args = append(args, access.UserID, access.UserID)
	}
	total, err := s.count(ctx, "projects", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, customer_id, name, number, order_number, visible, private, billable, estimate_seconds, budget_cents, budget_alert_percent, comment, legacy_json, created_at FROM projects `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var items []domain.Project
	for rows.Next() {
		var p domain.Project
		var created string
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.CustomerID, &p.Name, &p.Number, &p.OrderNo, &p.Visible, &p.Private, &p.Billable, &p.EstimateSeconds, &p.BudgetCents, &p.BudgetAlertPercent, &p.Comment, &p.LegacyJSON, &created); err != nil {
			return nil, domain.Page{}, err
		}
		p.CreatedAt = parseTime(created)
		items = append(items, p)
	}
	return items, makePage(page, size, total), rows.Err()
}

func (s *Store) Project(ctx context.Context, id int64) (*domain.Project, error) {
	var p domain.Project
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, customer_id, name, number, order_number, visible, private, billable, estimate_seconds, budget_cents, budget_alert_percent, comment, legacy_json, created_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.WorkspaceID, &p.CustomerID, &p.Name, &p.Number, &p.OrderNo, &p.Visible, &p.Private, &p.Billable, &p.EstimateSeconds, &p.BudgetCents, &p.BudgetAlertPercent, &p.Comment, &p.LegacyJSON, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.CreatedAt = parseTime(created)
	return &p, nil
}

func (s *Store) UpsertActivity(ctx context.Context, a *domain.Activity) error {
	now := utcNow()
	if a.WorkspaceID == 0 {
		a.WorkspaceID = 1
	}
	if a.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO activities(workspace_id, project_id, name, number, visible, billable, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			a.WorkspaceID, a.ProjectID, a.Name, a.Number, boolInt(a.Visible), boolInt(a.Billable), a.Comment, a.LegacyJSON, now)
		if err != nil {
			return err
		}
		a.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE activities SET workspace_id=?, project_id=?, name=?, number=?, visible=?, billable=?, comment=?, legacy_json=? WHERE id=?`,
		a.WorkspaceID, a.ProjectID, a.Name, a.Number, boolInt(a.Visible), boolInt(a.Billable), a.Comment, a.LegacyJSON, a.ID)
	return err
}

func (s *Store) ListActivities(ctx context.Context, access domain.AccessContext, projectID int64, term string, page, size int) ([]domain.Activity, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := scopedSearchWhere("workspace_id", access.WorkspaceID, "name", term)
	if projectID > 0 {
		if where == "" {
			where = "WHERE project_id=?"
		} else {
			where += " AND project_id=?"
		}
		args = append(args, projectID)
	}
	if !access.IsWorkspaceAdmin() {
		where += ` AND (project_id IS NULL OR project_id IN (SELECT id FROM projects WHERE private=0 AND workspace_id=?) OR project_id IN (SELECT project_id FROM project_members WHERE user_id=?) OR project_id IN (SELECT pg.project_id FROM project_groups pg JOIN group_members gm ON gm.group_id=pg.group_id WHERE gm.user_id=?))`
		args = append(args, access.WorkspaceID, access.UserID, access.UserID)
	}
	total, err := s.count(ctx, "activities", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, project_id, name, number, visible, billable, comment, legacy_json, created_at FROM activities `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var items []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var project sql.NullInt64
		var created string
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &project, &a.Name, &a.Number, &a.Visible, &a.Billable, &a.Comment, &a.LegacyJSON, &created); err != nil {
			return nil, domain.Page{}, err
		}
		if project.Valid {
			a.ProjectID = &project.Int64
		}
		a.CreatedAt = parseTime(created)
		items = append(items, a)
	}
	return items, makePage(page, size, total), rows.Err()
}

func (s *Store) Activity(ctx context.Context, id int64) (*domain.Activity, error) {
	var a domain.Activity
	var project sql.NullInt64
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, project_id, name, number, visible, billable, comment, legacy_json, created_at FROM activities WHERE id=?`, id).
		Scan(&a.ID, &a.WorkspaceID, &project, &a.Name, &a.Number, &a.Visible, &a.Billable, &a.Comment, &a.LegacyJSON, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if project.Valid {
		a.ProjectID = &project.Int64
	}
	a.CreatedAt = parseTime(created)
	return &a, nil
}

func (s *Store) UpsertTask(ctx context.Context, t *domain.Task) error {
	if t.WorkspaceID == 0 {
		t.WorkspaceID = 1
	}
	now := utcNow()
	if t.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO tasks(workspace_id, project_id, name, number, visible, billable, estimate_seconds, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			t.WorkspaceID, t.ProjectID, t.Name, t.Number, boolInt(t.Visible), boolInt(t.Billable), t.EstimateSeconds, now)
		if err != nil {
			return err
		}
		t.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET workspace_id=?, project_id=?, name=?, number=?, visible=?, billable=?, estimate_seconds=? WHERE id=?`,
		t.WorkspaceID, t.ProjectID, t.Name, t.Number, boolInt(t.Visible), boolInt(t.Billable), t.EstimateSeconds, t.ID)
	return err
}

func (s *Store) ListTasks(ctx context.Context, access domain.AccessContext, projectID int64, term string, page, size int) ([]domain.Task, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := scopedSearchWhere("workspace_id", access.WorkspaceID, "name", term)
	if projectID > 0 {
		where += " AND project_id=?"
		args = append(args, projectID)
	}
	if !access.IsWorkspaceAdmin() {
		where += ` AND project_id IN (SELECT id FROM projects WHERE workspace_id=? AND (private=0 OR id IN (SELECT project_id FROM project_members WHERE user_id=?) OR id IN (SELECT pg.project_id FROM project_groups pg JOIN group_members gm ON gm.group_id=pg.group_id WHERE gm.user_id=?)))`
		args = append(args, access.WorkspaceID, access.UserID, access.UserID)
	}
	total, err := s.count(ctx, "tasks", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, project_id, name, number, visible, billable, estimate_seconds, created_at FROM tasks `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var tasks []domain.Task
	for rows.Next() {
		var task domain.Task
		var created string
		if err := rows.Scan(&task.ID, &task.WorkspaceID, &task.ProjectID, &task.Name, &task.Number, &task.Visible, &task.Billable, &task.EstimateSeconds, &created); err != nil {
			return nil, domain.Page{}, err
		}
		task.CreatedAt = parseTime(created)
		tasks = append(tasks, task)
	}
	return tasks, makePage(page, size, total), rows.Err()
}

func (s *Store) UpsertTag(ctx context.Context, workspaceID int64, name string) (int64, error) {
	if workspaceID == 0 {
		workspaceID = 1
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("empty tag")
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO tags(workspace_id, name, visible) VALUES(?,?,1)`, workspaceID, name)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM tags WHERE workspace_id=? AND name=?`, workspaceID, name).Scan(&id)
	return id, err
}

func (s *Store) ListTags(ctx context.Context, workspaceID int64) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, name, visible FROM tags WHERE workspace_id=? ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []domain.Tag
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.WorkspaceID, &tag.Name, &tag.Visible); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *Store) UpsertRate(ctx context.Context, r *domain.Rate) error {
	if r.WorkspaceID == 0 {
		r.WorkspaceID = 1
	}
	if r.EffectiveFrom.IsZero() {
		r.EffectiveFrom = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var effectiveTo any
	if r.EffectiveTo != nil {
		effectiveTo = formatTime(*r.EffectiveTo)
	}
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO rates(workspace_id, customer_id, project_id, activity_id, task_id, user_id, kind, amount_cents, internal_amount_cents, fixed, effective_from, effective_to) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.WorkspaceID, r.CustomerID, r.ProjectID, r.ActivityID, r.TaskID, r.UserID, defaultString(r.Kind, "hourly"), r.AmountCents, r.InternalAmountCents, boolInt(r.Fixed), formatTime(r.EffectiveFrom), effectiveTo)
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE rates SET workspace_id=?, customer_id=?, project_id=?, activity_id=?, task_id=?, user_id=?, kind=?, amount_cents=?, internal_amount_cents=?, fixed=?, effective_from=?, effective_to=? WHERE id=?`,
		r.WorkspaceID, r.CustomerID, r.ProjectID, r.ActivityID, r.TaskID, r.UserID, r.Kind, r.AmountCents, r.InternalAmountCents, boolInt(r.Fixed), formatTime(r.EffectiveFrom), effectiveTo, r.ID)
	return err
}

func (s *Store) ListRates(ctx context.Context, workspaceID int64) ([]domain.Rate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, customer_id, project_id, activity_id, task_id, user_id, kind, amount_cents, internal_amount_cents, fixed, effective_from, effective_to FROM rates WHERE workspace_id=? ORDER BY effective_from DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rates []domain.Rate
	for rows.Next() {
		var r domain.Rate
		var customer, project, activity, task, user, internal sql.NullInt64
		var effectiveFrom string
		var effectiveTo sql.NullString
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &customer, &project, &activity, &task, &user, &r.Kind, &r.AmountCents, &internal, &r.Fixed, &effectiveFrom, &effectiveTo); err != nil {
			return nil, err
		}
		r.CustomerID = nullableInt(customer)
		r.ProjectID = nullableInt(project)
		r.ActivityID = nullableInt(activity)
		r.TaskID = nullableInt(task)
		r.UserID = nullableInt(user)
		r.InternalAmountCents = nullableInt(internal)
		r.EffectiveFrom = parseTime(effectiveFrom)
		if effectiveTo.Valid {
			value := parseTime(effectiveTo.String)
			r.EffectiveTo = &value
		}
		rates = append(rates, r)
	}
	return rates, rows.Err()
}

func (s *Store) ResolveRate(ctx context.Context, workspaceID, userID, customerID, projectID, activityID int64) (int64, *int64, error) {
	return s.ResolveRateAt(ctx, workspaceID, userID, customerID, projectID, activityID, nil, time.Now().UTC())
}

func (s *Store) ResolveRateAt(ctx context.Context, workspaceID, userID, customerID, projectID, activityID int64, taskID *int64, at time.Time) (int64, *int64, error) {
	candidates := []struct {
		where string
		args  []any
	}{
		{"activity_id=? AND task_id IS NULL AND user_id=?", []any{activityID, userID}},
		{"activity_id=? AND task_id IS NULL AND user_id IS NULL", []any{activityID}},
		{"project_id=? AND task_id IS NULL AND user_id=?", []any{projectID, userID}},
		{"project_id=? AND task_id IS NULL AND user_id IS NULL", []any{projectID}},
		{"customer_id=? AND project_id IS NULL AND activity_id IS NULL AND task_id IS NULL AND user_id=?", []any{customerID, userID}},
		{"customer_id=? AND project_id IS NULL AND activity_id IS NULL AND task_id IS NULL AND user_id IS NULL", []any{customerID}},
		{"customer_id IS NULL AND project_id IS NULL AND activity_id IS NULL AND task_id IS NULL AND user_id=?", []any{userID}},
		{"customer_id IS NULL AND project_id IS NULL AND activity_id IS NULL AND task_id IS NULL AND user_id IS NULL", nil},
	}
	if taskID != nil {
		taskCandidates := []struct {
			where string
			args  []any
		}{
			{"task_id=? AND user_id=?", []any{*taskID, userID}},
			{"task_id=? AND user_id IS NULL", []any{*taskID}},
		}
		candidates = append(taskCandidates, candidates...)
	}
	for _, candidate := range candidates {
		var amount int64
		var internal sql.NullInt64
		args := append([]any{workspaceID}, candidate.args...)
		args = append(args, formatTime(at))
		err := s.db.QueryRowContext(ctx, `SELECT amount_cents, internal_amount_cents FROM rates WHERE workspace_id=? AND `+candidate.where+` AND effective_from<=? AND (effective_to IS NULL OR effective_to='' OR effective_to>?) ORDER BY effective_from DESC, id DESC LIMIT 1`, append(args, formatTime(at))...).Scan(&amount, &internal)
		if err == nil {
			cost := nullableInt(internal)
			if cost == nil {
				userCost, err := s.ResolveUserCostAt(ctx, workspaceID, userID, at)
				if err != nil {
					return 0, nil, err
				}
				cost = userCost
			}
			return amount, cost, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, nil, err
		}
	}
	userCost, err := s.ResolveUserCostAt(ctx, workspaceID, userID, at)
	return 0, userCost, err
}

func (s *Store) UpsertUserCostRate(ctx context.Context, r *domain.UserCostRate) error {
	if r.WorkspaceID == 0 {
		r.WorkspaceID = 1
	}
	if r.EffectiveFrom.IsZero() {
		r.EffectiveFrom = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var effectiveTo any
	if r.EffectiveTo != nil {
		effectiveTo = formatTime(*r.EffectiveTo)
	}
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO user_cost_rates(workspace_id, user_id, amount_cents, effective_from, effective_to, created_at) VALUES(?,?,?,?,?,?)`,
			r.WorkspaceID, r.UserID, r.AmountCents, formatTime(r.EffectiveFrom), effectiveTo, utcNow())
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE user_cost_rates SET workspace_id=?, user_id=?, amount_cents=?, effective_from=?, effective_to=? WHERE id=?`,
		r.WorkspaceID, r.UserID, r.AmountCents, formatTime(r.EffectiveFrom), effectiveTo, r.ID)
	return err
}

func (s *Store) ListUserCostRates(ctx context.Context, workspaceID int64) ([]domain.UserCostRate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, user_id, amount_cents, effective_from, effective_to, created_at FROM user_cost_rates WHERE workspace_id=? ORDER BY effective_from DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rates []domain.UserCostRate
	for rows.Next() {
		var rate domain.UserCostRate
		var effectiveFrom, created string
		var effectiveTo sql.NullString
		if err := rows.Scan(&rate.ID, &rate.WorkspaceID, &rate.UserID, &rate.AmountCents, &effectiveFrom, &effectiveTo, &created); err != nil {
			return nil, err
		}
		rate.EffectiveFrom = parseTime(effectiveFrom)
		if effectiveTo.Valid {
			value := parseTime(effectiveTo.String)
			rate.EffectiveTo = &value
		}
		rate.CreatedAt = parseTime(created)
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

func (s *Store) ResolveUserCostAt(ctx context.Context, workspaceID, userID int64, at time.Time) (*int64, error) {
	var amount int64
	err := s.db.QueryRowContext(ctx, `SELECT amount_cents FROM user_cost_rates WHERE workspace_id=? AND user_id=? AND effective_from<=? AND (effective_to IS NULL OR effective_to='' OR effective_to>?) ORDER BY effective_from DESC, id DESC LIMIT 1`,
		workspaceID, userID, formatTime(at), formatTime(at)).Scan(&amount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &amount, nil
}

func (s *Store) StartTimer(ctx context.Context, t *domain.Timesheet, tagNames []string) error {
	existing, err := s.ActiveTimer(ctx, t.UserID)
	if err != nil {
		return err
	}
	if existing != nil {
		return errors.New("an active timer already exists")
	}
	now := utcNow()
	if t.StartedAt.IsZero() {
		t.StartedAt = time.Now().UTC()
	}
	t.CreatedAt = parseTime(now)
	t.UpdatedAt = t.CreatedAt
	if t.WorkspaceID == 0 {
		t.WorkspaceID = 1
	}
	rate, internal, err := s.ResolveRateAt(ctx, t.WorkspaceID, t.UserID, t.CustomerID, t.ProjectID, t.ActivityID, t.TaskID, t.StartedAt)
	if err != nil {
		return err
	}
	t.RateCents = rate
	t.InternalRateCents = internal
	return s.insertTimesheet(ctx, t, tagNames)
}

func (s *Store) CreateTimesheet(ctx context.Context, t *domain.Timesheet, tagNames []string) error {
	if t.EndedAt != nil {
		t.DurationSeconds = int64(t.EndedAt.Sub(t.StartedAt).Seconds()) - t.BreakSeconds
		if t.DurationSeconds < 0 {
			t.DurationSeconds = 0
		}
	}
	if t.WorkspaceID == 0 {
		t.WorkspaceID = 1
	}
	rate, internal, err := s.ResolveRateAt(ctx, t.WorkspaceID, t.UserID, t.CustomerID, t.ProjectID, t.ActivityID, t.TaskID, t.StartedAt)
	if err != nil {
		return err
	}
	t.RateCents = rate
	t.InternalRateCents = internal
	return s.insertTimesheet(ctx, t, tagNames)
}

func (s *Store) insertTimesheet(ctx context.Context, t *domain.Timesheet, tagNames []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	now := utcNow()
	var ended any
	if t.EndedAt != nil {
		ended = formatTime(*t.EndedAt)
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO timesheets(workspace_id, user_id, customer_id, project_id, activity_id, task_id, started_at, ended_at, timezone, duration_seconds, break_seconds, rate_cents, internal_rate_cents, billable, exported, description, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.WorkspaceID, t.UserID, t.CustomerID, t.ProjectID, t.ActivityID, t.TaskID, formatTime(t.StartedAt), ended, defaultString(t.Timezone, "UTC"), t.DurationSeconds, t.BreakSeconds, t.RateCents, t.InternalRateCents, boolInt(t.Billable), boolInt(t.Exported), t.Description, now, now)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(workspace_id, name, visible) VALUES(?,?,1)`, t.WorkspaceID, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO timesheet_tags(timesheet_id, tag_id) SELECT ?, id FROM tags WHERE workspace_id=? AND name=?`, t.ID, t.WorkspaceID, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) StopTimer(ctx context.Context, userID int64, end time.Time) (*domain.Timesheet, error) {
	active, err := s.ActiveTimer(ctx, userID)
	if err != nil || active == nil {
		return active, err
	}
	duration := int64(end.Sub(active.StartedAt).Seconds()) - active.BreakSeconds
	if duration < 0 {
		duration = 0
	}
	_, err = s.db.ExecContext(ctx, `UPDATE timesheets SET ended_at=?, duration_seconds=?, updated_at=? WHERE id=?`, formatTime(end.UTC()), duration, utcNow(), active.ID)
	if err != nil {
		return nil, err
	}
	active.EndedAt = &end
	active.DurationSeconds = duration
	return active, nil
}

func (s *Store) ActiveTimer(ctx context.Context, userID int64) (*domain.Timesheet, error) {
	items, _, err := s.ListTimesheets(ctx, TimesheetFilter{UserID: userID, Page: 1, Size: 1})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.EndedAt == nil {
			return &item, nil
		}
	}
	return nil, nil
}

func (s *Store) ListTimesheets(ctx context.Context, f TimesheetFilter) ([]domain.Timesheet, domain.Page, error) {
	f.Page, f.Size = domain.NormalizePage(f.Page, f.Size)
	where := []string{"1=1"}
	args := []any{}
	if f.WorkspaceID > 0 {
		where = append(where, "workspace_id=?")
		args = append(args, f.WorkspaceID)
	}
	if f.UserID > 0 {
		where = append(where, "user_id=?")
		args = append(args, f.UserID)
	}
	if f.CustomerID > 0 {
		where = append(where, "customer_id=?")
		args = append(args, f.CustomerID)
	}
	if f.ProjectID > 0 {
		where = append(where, "project_id=?")
		args = append(args, f.ProjectID)
	}
	if len(f.ProjectIDs) > 0 {
		placeholders := make([]string, len(f.ProjectIDs))
		for i, id := range f.ProjectIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, "project_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if f.ActivityID > 0 {
		where = append(where, "activity_id=?")
		args = append(args, f.ActivityID)
	}
	if f.TaskID > 0 {
		where = append(where, "task_id=?")
		args = append(args, f.TaskID)
	}
	if f.GroupID > 0 {
		where = append(where, "user_id IN (SELECT user_id FROM group_members WHERE group_id=?)")
		args = append(args, f.GroupID)
	}
	if f.Begin != nil {
		where = append(where, "started_at>=?")
		args = append(args, formatTime(*f.Begin))
	}
	if f.End != nil {
		where = append(where, "started_at<=?")
		args = append(args, formatTime(*f.End))
	}
	if f.Exported != nil {
		where = append(where, "exported=?")
		args = append(args, boolInt(*f.Exported))
	}
	if f.Billable != nil {
		where = append(where, "billable=?")
		args = append(args, boolInt(*f.Billable))
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	total, err := s.count(ctx, "timesheets", whereSQL, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, user_id, customer_id, project_id, activity_id, task_id, started_at, ended_at, timezone, duration_seconds, break_seconds, rate_cents, internal_rate_cents, billable, exported, description, created_at, updated_at FROM timesheets `+whereSQL+` ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		append(args, f.Size, (f.Page-1)*f.Size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var items []domain.Timesheet
	for rows.Next() {
		item, err := scanTimesheet(rows)
		if err != nil {
			return nil, domain.Page{}, err
		}
		items = append(items, item)
	}
	return items, makePage(f.Page, f.Size, total), rows.Err()
}

func (s *Store) CreateInvoice(ctx context.Context, access domain.AccessContext, userID, customerID int64, begin, end time.Time, taxBasisPoints int64) (*domain.Invoice, error) {
	billable := true
	exported := false
	items, _, err := s.ListTimesheets(ctx, TimesheetFilter{WorkspaceID: access.WorkspaceID, CustomerID: customerID, Begin: &begin, End: &end, Billable: &billable, Exported: &exported, Page: 1, Size: 100})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("no unexported billable timesheets found")
	}
	var subtotal int64
	for _, item := range items {
		subtotal += item.RateCents * item.DurationSeconds / 3600
	}
	tax := subtotal * taxBasisPoints / 10000
	invoice := &domain.Invoice{
		WorkspaceID:   access.WorkspaceID,
		Number:        fmt.Sprintf("INV-%s-%04d", time.Now().UTC().Format("20060102"), time.Now().UTC().Unix()%10000),
		CustomerID:    customerID,
		UserID:        userID,
		Status:        "new",
		Currency:      "USD",
		SubtotalCents: subtotal,
		TaxCents:      tax,
		TotalCents:    subtotal + tax,
		CreatedAt:     time.Now().UTC(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	invoice.Filename = strings.ToLower(invoice.Number) + ".html"
	res, err := tx.ExecContext(ctx, `INSERT INTO invoices(workspace_id, number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		invoice.WorkspaceID, invoice.Number, invoice.CustomerID, invoice.UserID, invoice.Status, invoice.Currency, invoice.SubtotalCents, invoice.TaxCents, invoice.TotalCents, invoice.Filename, formatTime(invoice.CreatedAt))
	if err != nil {
		return nil, err
	}
	invoice.ID, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		total := item.RateCents * item.DurationSeconds / 3600
		if _, err := tx.ExecContext(ctx, `INSERT INTO invoice_items(invoice_id, timesheet_id, description, quantity, unit_cents, total_cents) VALUES(?,?,?,?,?,?)`,
			invoice.ID, item.ID, item.Description, item.DurationSeconds, item.RateCents, total); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE timesheets SET exported=1, updated_at=? WHERE id=?`, utcNow(), item.ID); err != nil {
			return nil, err
		}
	}
	return invoice, tx.Commit()
}

func (s *Store) ListInvoices(ctx context.Context, workspaceID, customerID int64, page, size int) ([]domain.Invoice, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where := "WHERE workspace_id=?"
	args := []any{workspaceID}
	if customerID > 0 {
		where += " AND customer_id=?"
		args = append(args, customerID)
	}
	total, err := s.count(ctx, "invoices", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, payment_date, created_at FROM invoices `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var invoices []domain.Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, domain.Page{}, err
		}
		invoices = append(invoices, inv)
	}
	return invoices, makePage(page, size, total), rows.Err()
}

func (s *Store) Invoice(ctx context.Context, id int64) (*domain.Invoice, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, payment_date, created_at FROM invoices WHERE id=?`, id)
	inv, err := scanInvoice(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &inv, nil
}

func (s *Store) SetInvoiceMeta(ctx context.Context, invoiceID int64, name, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO invoice_meta(invoice_id, name, value) VALUES(?,?,?) ON CONFLICT(invoice_id, name) DO UPDATE SET value=excluded.value`, invoiceID, name, value)
	return err
}

func (s *Store) CreateSavedReport(ctx context.Context, report *domain.SavedReport) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO saved_reports(workspace_id, user_id, name, group_by, filters_json, shared, created_at) VALUES(?,?,?,?,?,?,?)`,
		report.WorkspaceID, report.UserID, report.Name, defaultString(report.GroupBy, "user"), defaultString(report.FiltersJSON, "{}"), boolInt(report.Shared), utcNow())
	if err != nil {
		return err
	}
	report.ID, err = res.LastInsertId()
	return err
}

func (s *Store) ListSavedReports(ctx context.Context, workspaceID, userID int64) ([]domain.SavedReport, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, user_id, name, group_by, filters_json, shared, created_at FROM saved_reports WHERE workspace_id=? AND (user_id=? OR shared=1) ORDER BY name`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []domain.SavedReport
	for rows.Next() {
		var report domain.SavedReport
		var created string
		if err := rows.Scan(&report.ID, &report.WorkspaceID, &report.UserID, &report.Name, &report.GroupBy, &report.FiltersJSON, &report.Shared, &created); err != nil {
			return nil, err
		}
		report.CreatedAt = parseTime(created)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (s *Store) ListReports(ctx context.Context, access domain.AccessContext, filter domain.ReportFilter) ([]map[string]any, error) {
	scope := reportScope(access)
	timeWhere, timeArgs := reportFilterSQL(filter)
	scope.where += timeWhere
	scope.args = append(scope.args, timeArgs...)
	group := defaultString(filter.Group, "user")
	switch group {
	case "customer":
		return s.report(ctx, `SELECT c.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM customers c LEFT JOIN timesheets t ON t.customer_id=c.id `+scope.join+` WHERE c.workspace_id=? `+scope.where+` GROUP BY c.id ORDER BY c.name`, append([]any{access.WorkspaceID}, scope.args...)...)
	case "activity":
		return s.report(ctx, `SELECT a.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM activities a LEFT JOIN timesheets t ON t.activity_id=a.id `+scope.join+` WHERE a.workspace_id=? `+scope.where+` GROUP BY a.id ORDER BY a.name`, append([]any{access.WorkspaceID}, scope.args...)...)
	case "project":
		return s.report(ctx, `SELECT p.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM projects p LEFT JOIN timesheets t ON t.project_id=p.id `+scope.join+` WHERE p.workspace_id=? `+scope.where+` GROUP BY p.id ORDER BY p.name`, append([]any{access.WorkspaceID}, scope.args...)...)
	case "task":
		return s.report(ctx, `SELECT ta.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM tasks ta LEFT JOIN timesheets t ON t.task_id=ta.id `+scope.join+` WHERE ta.workspace_id=? `+scope.where+` GROUP BY ta.id ORDER BY ta.name`, append([]any{access.WorkspaceID}, scope.args...)...)
	case "group":
		return s.report(ctx, `SELECT g.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM groups g LEFT JOIN group_members gm ON gm.group_id=g.id LEFT JOIN timesheets t ON t.user_id=gm.user_id AND t.workspace_id=g.workspace_id WHERE g.workspace_id=? GROUP BY g.id ORDER BY g.name`, access.WorkspaceID)
	default:
		return s.report(ctx, `SELECT u.display_name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM users u LEFT JOIN timesheets t ON t.user_id=u.id `+scope.join+` WHERE (t.workspace_id=? OR t.id IS NULL) `+scope.where+` GROUP BY u.id ORDER BY u.display_name`, append([]any{access.WorkspaceID}, scope.args...)...)
	}
}

func (s *Store) Dashboard(ctx context.Context, access domain.AccessContext) (map[string]int64, error) {
	stats := map[string]int64{}
	projectIDs := accessibleProjectIDs(access)
	projectWhere := ""
	projectArgs := []any{}
	if !access.IsWorkspaceAdmin() && access.WorkspaceRole != domain.WorkspaceRoleAnalyst {
		if len(projectIDs) > 0 {
			placeholders := make([]string, len(projectIDs))
			projectArgs = append(projectArgs, access.UserID)
			for i, id := range projectIDs {
				placeholders[i] = "?"
				projectArgs = append(projectArgs, id)
			}
			projectWhere = " AND (user_id=? OR project_id IN (" + strings.Join(placeholders, ",") + "))"
		} else {
			projectWhere = " AND user_id=?"
			projectArgs = append(projectArgs, access.UserID)
		}
	}
	queries := map[string]string{
		"active_timers": "SELECT COUNT(*) FROM timesheets WHERE workspace_id=? AND ended_at IS NULL" + projectWhere,
		"today_seconds": "SELECT COALESCE(SUM(duration_seconds),0) FROM timesheets WHERE workspace_id=? AND user_id=? AND started_at>=?",
		"unexported":    "SELECT COUNT(*) FROM timesheets WHERE workspace_id=? AND exported=0 AND billable=1 AND ended_at IS NOT NULL" + projectWhere,
		"invoices":      "SELECT COUNT(*) FROM invoices WHERE workspace_id=?",
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for key, query := range queries {
		var value int64
		var err error
		if key == "today_seconds" {
			err = s.db.QueryRowContext(ctx, query, access.WorkspaceID, access.UserID, formatTime(today)).Scan(&value)
		} else if key == "active_timers" || key == "unexported" {
			err = s.db.QueryRowContext(ctx, query, append([]any{access.WorkspaceID}, projectArgs...)...).Scan(&value)
		} else if key == "invoices" && !access.IsWorkspaceAdmin() {
			value = 0
		} else {
			err = s.db.QueryRowContext(ctx, query, access.WorkspaceID).Scan(&value)
		}
		if err != nil {
			return nil, err
		}
		stats[key] = value
	}
	return stats, nil
}

func (s *Store) ProjectDashboard(ctx context.Context, access domain.AccessContext, projectID int64) (domain.ProjectDashboard, error) {
	var dashboard domain.ProjectDashboard
	project, err := s.Project(ctx, projectID)
	if err != nil || project == nil {
		return dashboard, err
	}
	if project.WorkspaceID != access.WorkspaceID || (!access.IsWorkspaceAdmin() && project.Private && !access.CanAccessProject(projectID)) {
		return dashboard, sql.ErrNoRows
	}
	dashboard.Project = *project
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(duration_seconds),0), COALESCE(SUM(rate_cents*duration_seconds/3600),0) FROM timesheets WHERE workspace_id=? AND project_id=?`, access.WorkspaceID, projectID).
		Scan(&dashboard.TrackedSeconds, &dashboard.BillableCents)
	if err != nil {
		return dashboard, err
	}
	if project.EstimateSeconds > 0 {
		dashboard.EstimatePercent = dashboard.TrackedSeconds * 100 / project.EstimateSeconds
		dashboard.OverEstimate = dashboard.TrackedSeconds > project.EstimateSeconds
	}
	if project.BudgetCents > 0 {
		dashboard.BudgetPercent = dashboard.BillableCents * 100 / project.BudgetCents
		dashboard.OverBudget = dashboard.BillableCents > project.BudgetCents
	}
	threshold := project.BudgetAlertPercent
	if threshold == 0 {
		threshold = 80
	}
	dashboard.Alert = (project.EstimateSeconds > 0 && dashboard.EstimatePercent >= threshold) || (project.BudgetCents > 0 && dashboard.BudgetPercent >= threshold)
	return dashboard, nil
}

func (s *Store) CreateWebhookEndpoint(ctx context.Context, w *domain.WebhookEndpoint) error {
	w.CreatedAt = time.Now().UTC()
	if w.WorkspaceID == 0 {
		w.WorkspaceID = 1
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO webhook_endpoints(workspace_id, name, url, secret, events, enabled, created_at) VALUES(?,?,?,?,?,?,?)`,
		w.WorkspaceID, w.Name, w.URL, w.Secret, strings.Join(w.Events, ","), boolInt(w.Enabled), formatTime(w.CreatedAt))
	if err != nil {
		return err
	}
	w.ID, err = res.LastInsertId()
	return err
}

func (s *Store) ListWebhookEndpoints(ctx context.Context, workspaceID int64) ([]domain.WebhookEndpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, name, url, secret, events, enabled, created_at FROM webhook_endpoints WHERE workspace_id=? ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hooks []domain.WebhookEndpoint
	for rows.Next() {
		var h domain.WebhookEndpoint
		var events, created string
		if err := rows.Scan(&h.ID, &h.WorkspaceID, &h.Name, &h.URL, &h.Secret, &events, &h.Enabled, &created); err != nil {
			return nil, err
		}
		if events != "" {
			h.Events = strings.Split(events, ",")
		}
		h.CreatedAt = parseTime(created)
		hooks = append(hooks, h)
	}
	return hooks, rows.Err()
}

func (s *Store) QueueWebhook(ctx context.Context, workspaceID int64, event string, payload []byte) error {
	hooks, err := s.ListWebhookEndpoints(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, hook := range hooks {
		if !hook.Enabled || !eventAllowed(hook.Events, event) {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO webhook_deliveries(endpoint_id, event, payload, status, attempts, next_attempt_at, created_at) VALUES(?,?,?,?,?,?,?)`,
			hook.ID, event, string(payload), "pending", 0, utcNow(), utcNow()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PendingWebhookDeliveries(ctx context.Context, limit int) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, `SELECT d.id, e.url, e.secret, d.event, d.payload, d.attempts FROM webhook_deliveries d JOIN webhook_endpoints e ON e.id=d.endpoint_id WHERE d.status='pending' AND d.next_attempt_at<=? ORDER BY d.id LIMIT ?`, utcNow(), limit)
}

func (s *Store) MarkWebhookDelivery(ctx context.Context, id int64, status string, attempts int, lastError string, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status=?, attempts=?, last_error=?, next_attempt_at=? WHERE id=?`, status, attempts, lastError, formatTime(next), id)
	return err
}

func (s *Store) count(ctx context.Context, table, where string, args ...any) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` `+where, args...).Scan(&total)
	return total, err
}

func (s *Store) report(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var name string
		var count, seconds, cents int64
		if err := rows.Scan(&name, &count, &seconds, &cents); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"name": name, "count": count, "seconds": seconds, "cents": cents})
	}
	return result, rows.Err()
}

type reportScopeSQL struct {
	join  string
	where string
	args  []any
}

func reportScope(access domain.AccessContext) reportScopeSQL {
	if access.IsWorkspaceAdmin() || access.WorkspaceRole == domain.WorkspaceRoleAnalyst {
		return reportScopeSQL{}
	}
	projectIDs := accessibleProjectIDs(access)
	if len(projectIDs) == 0 {
		return reportScopeSQL{where: " AND t.user_id=?", args: []any{access.UserID}}
	}
	placeholders := make([]string, len(projectIDs))
	args := []any{access.UserID}
	for i, id := range projectIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	return reportScopeSQL{where: " AND (t.user_id=? OR t.project_id IN (" + strings.Join(placeholders, ",") + "))", args: args}
}

func reportFilterSQL(filter domain.ReportFilter) (string, []any) {
	where := ""
	args := []any{}
	if filter.Begin != nil {
		where += " AND (t.started_at IS NULL OR t.started_at>=?)"
		args = append(args, formatTime(*filter.Begin))
	}
	if filter.End != nil {
		where += " AND (t.started_at IS NULL OR t.started_at<=?)"
		args = append(args, formatTime(*filter.End))
	}
	if filter.CustomerID > 0 {
		where += " AND (t.id IS NULL OR t.customer_id=?)"
		args = append(args, filter.CustomerID)
	}
	if filter.ProjectID > 0 {
		where += " AND (t.id IS NULL OR t.project_id=?)"
		args = append(args, filter.ProjectID)
	}
	if filter.ActivityID > 0 {
		where += " AND (t.id IS NULL OR t.activity_id=?)"
		args = append(args, filter.ActivityID)
	}
	if filter.TaskID > 0 {
		where += " AND (t.id IS NULL OR t.task_id=?)"
		args = append(args, filter.TaskID)
	}
	if filter.UserID > 0 {
		where += " AND (t.id IS NULL OR t.user_id=?)"
		args = append(args, filter.UserID)
	}
	if filter.GroupID > 0 {
		where += " AND (t.id IS NULL OR t.user_id IN (SELECT user_id FROM group_members WHERE group_id=?))"
		args = append(args, filter.GroupID)
	}
	return where, args
}

func accessibleProjectIDs(access domain.AccessContext) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for id := range access.ManagedProjectIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for id := range access.MemberProjectIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTimesheet(row scanner) (domain.Timesheet, error) {
	var t domain.Timesheet
	var started, created, updated string
	var ended sql.NullString
	var internal, task sql.NullInt64
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.UserID, &t.CustomerID, &t.ProjectID, &t.ActivityID, &task, &started, &ended, &t.Timezone, &t.DurationSeconds, &t.BreakSeconds, &t.RateCents, &internal, &t.Billable, &t.Exported, &t.Description, &created, &updated)
	if err != nil {
		return t, err
	}
	t.StartedAt = parseTime(started)
	if ended.Valid {
		v := parseTime(ended.String)
		t.EndedAt = &v
	}
	t.TaskID = nullableInt(task)
	t.InternalRateCents = nullableInt(internal)
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	return t, nil
}

func scanInvoice(row scanner) (domain.Invoice, error) {
	var inv domain.Invoice
	var payment, created sql.NullString
	err := row.Scan(&inv.ID, &inv.WorkspaceID, &inv.Number, &inv.CustomerID, &inv.UserID, &inv.Status, &inv.Currency, &inv.SubtotalCents, &inv.TaxCents, &inv.TotalCents, &inv.Filename, &payment, &created)
	if err != nil {
		return inv, err
	}
	if payment.Valid {
		t := parseTime(payment.String)
		inv.PaymentDate = &t
	}
	if created.Valid {
		inv.CreatedAt = parseTime(created.String)
	}
	return inv, nil
}

func searchWhere(column, term string) (string, []any) {
	term = strings.TrimSpace(term)
	if term == "" {
		return "", nil
	}
	return "WHERE lower(" + column + ") LIKE lower(?)", []any{"%" + term + "%"}
}

func scopedSearchWhere(scopeColumn string, scopeID int64, searchColumn, term string) (string, []any) {
	where := "WHERE " + scopeColumn + "=?"
	args := []any{scopeID}
	term = strings.TrimSpace(term)
	if term != "" {
		where += " AND lower(" + searchColumn + ") LIKE lower(?)"
		args = append(args, "%"+term+"%")
	}
	return where, args
}

func makePage(page, size, total int) domain.Page {
	return domain.Page{Page: page, Size: size, Total: total, HasPrev: page > 1, HasNext: page*size < total}
}

func nullableInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func organizationRoleFromLegacy(roles []domain.Role) domain.OrganizationRole {
	for _, role := range roles {
		if role == domain.RoleSuperAdmin {
			return domain.OrgRoleOwner
		}
		if role == domain.RoleAdmin {
			return domain.OrgRoleAdmin
		}
	}
	return ""
}

func workspaceRoleFromLegacy(roles []domain.Role) domain.WorkspaceRole {
	for _, role := range roles {
		if role == domain.RoleSuperAdmin || role == domain.RoleAdmin {
			return domain.WorkspaceRoleAdmin
		}
		if role == domain.RoleTeamLead {
			return domain.WorkspaceRoleAnalyst
		}
	}
	return domain.WorkspaceRoleMember
}

func utcNow() string {
	return formatTime(time.Now().UTC())
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func randomToken(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func eventAllowed(events []string, event string) bool {
	for _, item := range events {
		item = strings.TrimSpace(item)
		if item == "*" || item == event {
			return true
		}
	}
	return false
}

func (s *Store) ensureHierarchy(ctx context.Context) error {
	adds := map[string]map[string]string{
		"users": {
			"organization_id":      "INTEGER NOT NULL DEFAULT 1",
			"totp_secret":          "TEXT NOT NULL DEFAULT ''",
			"totp_enabled":         "INTEGER NOT NULL DEFAULT 0",
			"totp_recovery_hashes": "TEXT NOT NULL DEFAULT ''",
		},
		"sessions": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
		"customers": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
		"projects": {
			"workspace_id":         "INTEGER NOT NULL DEFAULT 1",
			"private":              "INTEGER NOT NULL DEFAULT 0",
			"estimate_seconds":     "INTEGER NOT NULL DEFAULT 0",
			"budget_cents":         "INTEGER NOT NULL DEFAULT 0",
			"budget_alert_percent": "INTEGER NOT NULL DEFAULT 80",
		},
		"activities": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
		"tags": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
		"rates": {
			"workspace_id":   "INTEGER NOT NULL DEFAULT 1",
			"task_id":        "INTEGER",
			"effective_from": "TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z'",
			"effective_to":   "TEXT",
		},
		"timesheets": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
			"task_id":      "INTEGER",
		},
		"invoices": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
		"webhook_endpoints": {
			"workspace_id": "INTEGER NOT NULL DEFAULT 1",
		},
	}
	for table, columns := range adds {
		for column, definition := range columns {
			if err := s.ensureColumn(ctx, table, column, definition); err != nil {
				return err
			}
		}
	}
	now := utcNow()
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO organizations(id, name, slug, created_at) VALUES(1,'Default Organization','default',?)`, now); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspaces(id, organization_id, name, slug, default_currency, timezone, created_at) VALUES(1,1,'Default Workspace','default','USD','UTC',?)`, now); err != nil {
		return err
	}
	backfills := []string{
		`UPDATE users SET organization_id=1 WHERE organization_id IS NULL OR organization_id=0`,
		`UPDATE customers SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE projects SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE activities SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE tags SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE rates SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE rates SET effective_from='1970-01-01T00:00:00Z' WHERE effective_from IS NULL OR effective_from=''`,
		`UPDATE sessions SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE timesheets SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE invoices SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`UPDATE webhook_endpoints SET workspace_id=1 WHERE workspace_id IS NULL OR workspace_id=0`,
		`INSERT OR IGNORE INTO organization_members(organization_id, user_id, role, created_at)
		 SELECT 1, u.id,
		  CASE WHEN EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name='superadmin') THEN 'owner'
		       WHEN EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name='admin') THEN 'admin'
		       ELSE 'admin' END,
		  '` + now + `' FROM users u
		 WHERE EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name IN ('superadmin','admin'))`,
		`INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at)
		 SELECT 1, u.id,
		  CASE WHEN EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name IN ('superadmin','admin')) THEN 'admin'
		       WHEN EXISTS(SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name='teamlead') THEN 'analyst'
		       ELSE 'member' END,
		  '` + now + `' FROM users u`,
	}
	for _, query := range backfills {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_workspace_members_user ON workspace_members(user_id, workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_user ON organization_members(user_id, organization_id)`,
		`CREATE INDEX IF NOT EXISTS idx_groups_workspace_name ON groups(workspace_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id, group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_project_members_user ON project_members(user_id, project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_project_groups_group ON project_groups(group_id, project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_customers_workspace_name ON customers(workspace_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_workspace_visible ON projects(workspace_id, visible)`,
		`CREATE INDEX IF NOT EXISTS idx_tags_workspace_name ON tags(workspace_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_timesheets_workspace_started ON timesheets(workspace_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_workspace_created ON invoices(workspace_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project_visible ON tasks(project_id, visible)`,
		`CREATE INDEX IF NOT EXISTS idx_timesheets_task_started ON timesheets(task_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_favorites_user_workspace ON favorites(user_id, workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_saved_reports_user_workspace ON saved_reports(user_id, workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_rates_workspace_effective ON rates(workspace_id, effective_from DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_rates_workspace_task_effective ON rates(workspace_id, task_id, effective_from DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_user_cost_rates_effective ON user_cost_rates(workspace_id, user_id, effective_from DESC)`,
	}
	for _, query := range indexes {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS settings(name TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS roles(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
INSERT OR IGNORE INTO roles(name) VALUES('user'),('teamlead'),('admin'),('superadmin');
CREATE TABLE IF NOT EXISTS role_permissions(role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE, permission TEXT NOT NULL, allowed INTEGER NOT NULL DEFAULT 1, PRIMARY KEY(role_id, permission));
CREATE TABLE IF NOT EXISTS organizations(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS workspaces(id INTEGER PRIMARY KEY AUTOINCREMENT, organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, name TEXT NOT NULL, slug TEXT NOT NULL, default_currency TEXT NOT NULL DEFAULT 'USD', timezone TEXT NOT NULL DEFAULT 'UTC', created_at TEXT NOT NULL, UNIQUE(organization_id, slug));
CREATE TABLE IF NOT EXISTS users(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	organization_id INTEGER NOT NULL DEFAULT 1 REFERENCES organizations(id) ON DELETE RESTRICT,
	email TEXT NOT NULL UNIQUE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	enabled INTEGER NOT NULL DEFAULT 1,
	totp_secret TEXT NOT NULL DEFAULT '',
	totp_enabled INTEGER NOT NULL DEFAULT 0,
	totp_recovery_hashes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_login_at TEXT
);
CREATE TABLE IF NOT EXISTS user_roles(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE, PRIMARY KEY(user_id, role_id));
CREATE TABLE IF NOT EXISTS organization_members(organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('owner','admin')), created_at TEXT NOT NULL, PRIMARY KEY(organization_id, user_id));
CREATE TABLE IF NOT EXISTS workspace_members(workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('admin','analyst','member')), created_at TEXT NOT NULL, PRIMARY KEY(workspace_id, user_id));
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE, csrf_token TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS teams(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
CREATE TABLE IF NOT EXISTS team_members(team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, lead INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(team_id, user_id));
CREATE TABLE IF NOT EXISTS groups(id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, UNIQUE(workspace_id, name));
CREATE TABLE IF NOT EXISTS group_members(group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, created_at TEXT NOT NULL, PRIMARY KEY(group_id, user_id));
CREATE TABLE IF NOT EXISTS customers(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	company TEXT NOT NULL DEFAULT '',
	contact TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	currency TEXT NOT NULL DEFAULT 'USD',
	timezone TEXT NOT NULL DEFAULT 'UTC',
	visible INTEGER NOT NULL DEFAULT 1,
	billable INTEGER NOT NULL DEFAULT 1,
	comment TEXT NOT NULL DEFAULT '',
	legacy_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	order_number TEXT NOT NULL DEFAULT '',
	visible INTEGER NOT NULL DEFAULT 1,
	private INTEGER NOT NULL DEFAULT 0,
	billable INTEGER NOT NULL DEFAULT 1,
	estimate_seconds INTEGER NOT NULL DEFAULT 0,
	budget_cents INTEGER NOT NULL DEFAULT 0,
	budget_alert_percent INTEGER NOT NULL DEFAULT 80,
	comment TEXT NOT NULL DEFAULT '',
	legacy_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS activities(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	visible INTEGER NOT NULL DEFAULT 1,
	billable INTEGER NOT NULL DEFAULT 1,
	comment TEXT NOT NULL DEFAULT '',
	legacy_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	visible INTEGER NOT NULL DEFAULT 1,
	billable INTEGER NOT NULL DEFAULT 1,
	estimate_seconds INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS customer_teams(customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(customer_id, team_id));
CREATE TABLE IF NOT EXISTS project_teams(project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(project_id, team_id));
CREATE TABLE IF NOT EXISTS activity_teams(activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(activity_id, team_id));
CREATE TABLE IF NOT EXISTS project_members(project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('manager','member')), created_at TEXT NOT NULL, PRIMARY KEY(project_id, user_id));
CREATE TABLE IF NOT EXISTS project_groups(project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, created_at TEXT NOT NULL, PRIMARY KEY(project_id, group_id));
CREATE TABLE IF NOT EXISTS rates(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	customer_id INTEGER REFERENCES customers(id) ON DELETE CASCADE,
	project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	activity_id INTEGER REFERENCES activities(id) ON DELETE CASCADE,
	task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	kind TEXT NOT NULL DEFAULT 'hourly',
	amount_cents INTEGER NOT NULL,
	internal_amount_cents INTEGER,
	fixed INTEGER NOT NULL DEFAULT 0,
	effective_from TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
	effective_to TEXT
);
CREATE TABLE IF NOT EXISTS user_cost_rates(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	amount_cents INTEGER NOT NULL,
	effective_from TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
	effective_to TEXT,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tags(id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE, name TEXT NOT NULL, visible INTEGER NOT NULL DEFAULT 1, UNIQUE(workspace_id, name));
CREATE TABLE IF NOT EXISTS timesheets(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
	task_id INTEGER REFERENCES tasks(id) ON DELETE SET NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	break_seconds INTEGER NOT NULL DEFAULT 0,
	rate_cents INTEGER NOT NULL DEFAULT 0,
	internal_rate_cents INTEGER,
	billable INTEGER NOT NULL DEFAULT 1,
	exported INTEGER NOT NULL DEFAULT 0,
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS timesheet_tags(timesheet_id INTEGER NOT NULL REFERENCES timesheets(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY(timesheet_id, tag_id));
CREATE TABLE IF NOT EXISTS favorites(id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL, customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE, project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE, task_id INTEGER REFERENCES tasks(id) ON DELETE SET NULL, description TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS invoices(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE,
	number TEXT NOT NULL UNIQUE,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'new',
	currency TEXT NOT NULL DEFAULT 'USD',
	subtotal_cents INTEGER NOT NULL DEFAULT 0,
	tax_cents INTEGER NOT NULL DEFAULT 0,
	total_cents INTEGER NOT NULL DEFAULT 0,
	filename TEXT NOT NULL DEFAULT '',
	payment_date TEXT,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS invoice_items(id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE, timesheet_id INTEGER REFERENCES timesheets(id) ON DELETE SET NULL, description TEXT NOT NULL DEFAULT '', quantity INTEGER NOT NULL, unit_cents INTEGER NOT NULL, total_cents INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS invoice_meta(invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE, name TEXT NOT NULL, value TEXT NOT NULL DEFAULT '', PRIMARY KEY(invoice_id, name));
CREATE TABLE IF NOT EXISTS saved_reports(id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL, group_by TEXT NOT NULL DEFAULT 'user', filters_json TEXT NOT NULL DEFAULT '{}', shared INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS webhook_endpoints(id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspaces(id) ON DELETE CASCADE, name TEXT NOT NULL, url TEXT NOT NULL, secret TEXT NOT NULL, events TEXT NOT NULL DEFAULT '*', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS webhook_deliveries(id INTEGER PRIMARY KEY AUTOINCREMENT, endpoint_id INTEGER NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE, event TEXT NOT NULL, payload TEXT NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', next_attempt_at TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS audit_log(id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, action TEXT NOT NULL, entity TEXT NOT NULL, entity_id INTEGER, detail TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_projects_customer_visible ON projects(customer_id, visible);
CREATE INDEX IF NOT EXISTS idx_activities_project_visible ON activities(project_id, visible);
CREATE INDEX IF NOT EXISTS idx_timesheets_user_started ON timesheets(user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_timesheets_project_started ON timesheets(project_id, started_at);
CREATE INDEX IF NOT EXISTS idx_timesheets_activity_started ON timesheets(activity_id, started_at);
CREATE INDEX IF NOT EXISTS idx_timesheets_exported_billable ON timesheets(exported, billable);
CREATE INDEX IF NOT EXISTS idx_invoices_customer_created ON invoices(customer_id, created_at DESC);
`
