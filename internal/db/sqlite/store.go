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
	ID        string
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
}

type TimesheetFilter struct {
	UserID     int64
	CustomerID int64
	ProjectID  int64
	ActivityID int64
	Begin      *time.Time
	End        *time.Time
	Exported   *bool
	Billable   *bool
	Page       int
	Size       int
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
	_, err := s.db.ExecContext(ctx, schema)
	return err
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	res, err := tx.ExecContext(ctx, `INSERT INTO users(email, username, display_name, password_hash, timezone, enabled, created_at) VALUES(?,?,?,?,?,?,?)`,
		email, "admin", "Administrator", hash, timezone, 1, now)
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
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO settings(name, value) VALUES('default_currency', ?), ('future_time_policy', ?)`, currency, "end_of_day"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, entity string, entityID *int64, detail string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_log(user_id, action, entity, entity_id, detail, created_at) VALUES(?,?,?,?,?,?)`,
		userID, action, entity, entityID, detail, utcNow())
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.scanUser(ctx, `WHERE lower(email)=lower(?)`, email)
}

func (s *Store) FindUserByID(ctx context.Context, id int64) (*domain.User, error) {
	return s.scanUser(ctx, `WHERE id=?`, id)
}

func (s *Store) scanUser(ctx context.Context, where string, args ...any) (*domain.User, error) {
	q := `SELECT id, email, username, display_name, password_hash, timezone, enabled, created_at, last_login_at FROM users ` + where
	row := s.db.QueryRowContext(ctx, q, args...)
	var u domain.User
	var created string
	var last sql.NullString
	if err := row.Scan(&u.ID, &u.Email, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Timezone, &u.Enabled, &created, &last); err != nil {
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, username, display_name, password_hash, timezone, enabled, created_at, last_login_at FROM users ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	var users []domain.User
	for rows.Next() {
		var u domain.User
		var created string
		var last sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Timezone, &u.Enabled, &created, &last); err != nil {
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
	res, err := tx.ExecContext(ctx, `INSERT INTO users(email, username, display_name, password_hash, timezone, enabled, created_at) VALUES(?,?,?,?,?,?,?)`,
		u.Email, u.Username, u.DisplayName, hash, defaultString(u.Timezone, "UTC"), boolInt(u.Enabled), utcNow())
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
	return tx.Commit()
}

func (s *Store) TouchLogin(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, utcNow(), userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (*Session, error) {
	session := &Session{ID: randomToken(32), UserID: userID, CSRFToken: randomToken(32), ExpiresAt: time.Now().UTC().Add(ttl)}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id, user_id, csrf_token, expires_at, created_at) VALUES(?,?,?,?,?)`,
		session.ID, session.UserID, session.CSRFToken, formatTime(session.ExpiresAt), utcNow())
	return session, err
}

func (s *Store) FindSession(ctx context.Context, id string) (*Session, error) {
	var session Session
	var expires string
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, csrf_token, expires_at FROM sessions WHERE id=?`, id).Scan(&session.ID, &session.UserID, &session.CSRFToken, &expires)
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
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO customers(name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Number, c.Company, c.Contact, c.Email, defaultString(c.Currency, "USD"), defaultString(c.Timezone, "UTC"), boolInt(c.Visible), boolInt(c.Billable), c.Comment, c.LegacyJSON, now)
		if err != nil {
			return err
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE customers SET name=?, number=?, company=?, contact=?, email=?, currency=?, timezone=?, visible=?, billable=?, comment=?, legacy_json=? WHERE id=?`,
		c.Name, c.Number, c.Company, c.Contact, c.Email, c.Currency, c.Timezone, boolInt(c.Visible), boolInt(c.Billable), c.Comment, c.LegacyJSON, c.ID)
	return err
}

func (s *Store) ListCustomers(ctx context.Context, term string, page, size int) ([]domain.Customer, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := searchWhere("name", term)
	total, err := s.count(ctx, "customers", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at FROM customers `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	items := []domain.Customer{}
	for rows.Next() {
		var c domain.Customer
		var created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Number, &c.Company, &c.Contact, &c.Email, &c.Currency, &c.Timezone, &c.Visible, &c.Billable, &c.Comment, &c.LegacyJSON, &created); err != nil {
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
	err := s.db.QueryRowContext(ctx, `SELECT id, name, number, company, contact, email, currency, timezone, visible, billable, comment, legacy_json, created_at FROM customers WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Number, &c.Company, &c.Contact, &c.Email, &c.Currency, &c.Timezone, &c.Visible, &c.Billable, &c.Comment, &c.LegacyJSON, &created)
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
	if p.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO projects(customer_id, name, number, order_number, visible, billable, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			p.CustomerID, p.Name, p.Number, p.OrderNo, boolInt(p.Visible), boolInt(p.Billable), p.Comment, p.LegacyJSON, now)
		if err != nil {
			return err
		}
		p.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET customer_id=?, name=?, number=?, order_number=?, visible=?, billable=?, comment=?, legacy_json=? WHERE id=?`,
		p.CustomerID, p.Name, p.Number, p.OrderNo, boolInt(p.Visible), boolInt(p.Billable), p.Comment, p.LegacyJSON, p.ID)
	return err
}

func (s *Store) ListProjects(ctx context.Context, customerID int64, term string, page, size int) ([]domain.Project, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := searchWhere("name", term)
	if customerID > 0 {
		if where == "" {
			where = "WHERE customer_id=?"
		} else {
			where += " AND customer_id=?"
		}
		args = append(args, customerID)
	}
	total, err := s.count(ctx, "projects", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, customer_id, name, number, order_number, visible, billable, comment, legacy_json, created_at FROM projects `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var items []domain.Project
	for rows.Next() {
		var p domain.Project
		var created string
		if err := rows.Scan(&p.ID, &p.CustomerID, &p.Name, &p.Number, &p.OrderNo, &p.Visible, &p.Billable, &p.Comment, &p.LegacyJSON, &created); err != nil {
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
	err := s.db.QueryRowContext(ctx, `SELECT id, customer_id, name, number, order_number, visible, billable, comment, legacy_json, created_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.CustomerID, &p.Name, &p.Number, &p.OrderNo, &p.Visible, &p.Billable, &p.Comment, &p.LegacyJSON, &created)
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
	if a.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO activities(project_id, name, number, visible, billable, comment, legacy_json, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			a.ProjectID, a.Name, a.Number, boolInt(a.Visible), boolInt(a.Billable), a.Comment, a.LegacyJSON, now)
		if err != nil {
			return err
		}
		a.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE activities SET project_id=?, name=?, number=?, visible=?, billable=?, comment=?, legacy_json=? WHERE id=?`,
		a.ProjectID, a.Name, a.Number, boolInt(a.Visible), boolInt(a.Billable), a.Comment, a.LegacyJSON, a.ID)
	return err
}

func (s *Store) ListActivities(ctx context.Context, projectID int64, term string, page, size int) ([]domain.Activity, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where, args := searchWhere("name", term)
	if projectID > 0 {
		if where == "" {
			where = "WHERE project_id=?"
		} else {
			where += " AND project_id=?"
		}
		args = append(args, projectID)
	}
	total, err := s.count(ctx, "activities", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, number, visible, billable, comment, legacy_json, created_at FROM activities `+where+` ORDER BY name LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	defer rows.Close()
	var items []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var project sql.NullInt64
		var created string
		if err := rows.Scan(&a.ID, &project, &a.Name, &a.Number, &a.Visible, &a.Billable, &a.Comment, &a.LegacyJSON, &created); err != nil {
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
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, name, number, visible, billable, comment, legacy_json, created_at FROM activities WHERE id=?`, id).
		Scan(&a.ID, &project, &a.Name, &a.Number, &a.Visible, &a.Billable, &a.Comment, &a.LegacyJSON, &created)
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

func (s *Store) UpsertTag(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("empty tag")
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name, visible) VALUES(?,1)`, name)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM tags WHERE name=?`, name).Scan(&id)
	return id, err
}

func (s *Store) ListTags(ctx context.Context) ([]domain.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, visible FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []domain.Tag
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Visible); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *Store) UpsertRate(ctx context.Context, r *domain.Rate) error {
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO rates(customer_id, project_id, activity_id, user_id, kind, amount_cents, internal_amount_cents, fixed) VALUES(?,?,?,?,?,?,?,?)`,
			r.CustomerID, r.ProjectID, r.ActivityID, r.UserID, defaultString(r.Kind, "hourly"), r.AmountCents, r.InternalAmountCents, boolInt(r.Fixed))
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE rates SET customer_id=?, project_id=?, activity_id=?, user_id=?, kind=?, amount_cents=?, internal_amount_cents=?, fixed=? WHERE id=?`,
		r.CustomerID, r.ProjectID, r.ActivityID, r.UserID, r.Kind, r.AmountCents, r.InternalAmountCents, boolInt(r.Fixed), r.ID)
	return err
}

func (s *Store) ListRates(ctx context.Context) ([]domain.Rate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, customer_id, project_id, activity_id, user_id, kind, amount_cents, internal_amount_cents, fixed FROM rates ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rates []domain.Rate
	for rows.Next() {
		var r domain.Rate
		var customer, project, activity, user, internal sql.NullInt64
		if err := rows.Scan(&r.ID, &customer, &project, &activity, &user, &r.Kind, &r.AmountCents, &internal, &r.Fixed); err != nil {
			return nil, err
		}
		r.CustomerID = nullableInt(customer)
		r.ProjectID = nullableInt(project)
		r.ActivityID = nullableInt(activity)
		r.UserID = nullableInt(user)
		r.InternalAmountCents = nullableInt(internal)
		rates = append(rates, r)
	}
	return rates, rows.Err()
}

func (s *Store) ResolveRate(ctx context.Context, userID, customerID, projectID, activityID int64) (int64, *int64, error) {
	candidates := []struct {
		where string
		args  []any
	}{
		{"activity_id=? AND user_id=?", []any{activityID, userID}},
		{"activity_id=? AND user_id IS NULL", []any{activityID}},
		{"project_id=? AND user_id=?", []any{projectID, userID}},
		{"project_id=? AND user_id IS NULL", []any{projectID}},
		{"customer_id=? AND user_id=?", []any{customerID, userID}},
		{"customer_id=? AND user_id IS NULL", []any{customerID}},
		{"customer_id IS NULL AND project_id IS NULL AND activity_id IS NULL AND user_id=?", []any{userID}},
		{"customer_id IS NULL AND project_id IS NULL AND activity_id IS NULL AND user_id IS NULL", nil},
	}
	for _, candidate := range candidates {
		var amount int64
		var internal sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT amount_cents, internal_amount_cents FROM rates WHERE `+candidate.where+` ORDER BY id DESC LIMIT 1`, candidate.args...).Scan(&amount, &internal)
		if err == nil {
			return amount, nullableInt(internal), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, nil, err
		}
	}
	return 0, nil, nil
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
	rate, internal, err := s.ResolveRate(ctx, t.UserID, t.CustomerID, t.ProjectID, t.ActivityID)
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
	rate, internal, err := s.ResolveRate(ctx, t.UserID, t.CustomerID, t.ProjectID, t.ActivityID)
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
	res, err := tx.ExecContext(ctx, `INSERT INTO timesheets(user_id, customer_id, project_id, activity_id, started_at, ended_at, timezone, duration_seconds, break_seconds, rate_cents, internal_rate_cents, billable, exported, description, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.UserID, t.CustomerID, t.ProjectID, t.ActivityID, formatTime(t.StartedAt), ended, defaultString(t.Timezone, "UTC"), t.DurationSeconds, t.BreakSeconds, t.RateCents, t.InternalRateCents, boolInt(t.Billable), boolInt(t.Exported), t.Description, now, now)
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
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name, visible) VALUES(?,1)`, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO timesheet_tags(timesheet_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, t.ID, name); err != nil {
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
	if f.ActivityID > 0 {
		where = append(where, "activity_id=?")
		args = append(args, f.ActivityID)
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, customer_id, project_id, activity_id, started_at, ended_at, timezone, duration_seconds, break_seconds, rate_cents, internal_rate_cents, billable, exported, description, created_at, updated_at FROM timesheets `+whereSQL+` ORDER BY started_at DESC LIMIT ? OFFSET ?`,
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

func (s *Store) CreateInvoice(ctx context.Context, userID, customerID int64, begin, end time.Time, taxBasisPoints int64) (*domain.Invoice, error) {
	billable := true
	exported := false
	items, _, err := s.ListTimesheets(ctx, TimesheetFilter{CustomerID: customerID, Begin: &begin, End: &end, Billable: &billable, Exported: &exported, Page: 1, Size: 100})
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
	res, err := tx.ExecContext(ctx, `INSERT INTO invoices(number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		invoice.Number, invoice.CustomerID, invoice.UserID, invoice.Status, invoice.Currency, invoice.SubtotalCents, invoice.TaxCents, invoice.TotalCents, invoice.Filename, formatTime(invoice.CreatedAt))
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

func (s *Store) ListInvoices(ctx context.Context, customerID int64, page, size int) ([]domain.Invoice, domain.Page, error) {
	page, size = domain.NormalizePage(page, size)
	where := ""
	args := []any{}
	if customerID > 0 {
		where = "WHERE customer_id=?"
		args = append(args, customerID)
	}
	total, err := s.count(ctx, "invoices", where, args...)
	if err != nil {
		return nil, domain.Page{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, payment_date, created_at FROM invoices `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
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
	row := s.db.QueryRowContext(ctx, `SELECT id, number, customer_id, user_id, status, currency, subtotal_cents, tax_cents, total_cents, filename, payment_date, created_at FROM invoices WHERE id=?`, id)
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

func (s *Store) ListReports(ctx context.Context, group string) ([]map[string]any, error) {
	switch group {
	case "customer":
		return s.report(ctx, `SELECT c.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM customers c LEFT JOIN timesheets t ON t.customer_id=c.id GROUP BY c.id ORDER BY c.name`)
	case "activity":
		return s.report(ctx, `SELECT a.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM activities a LEFT JOIN timesheets t ON t.activity_id=a.id GROUP BY a.id ORDER BY a.name`)
	case "project":
		return s.report(ctx, `SELECT p.name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM projects p LEFT JOIN timesheets t ON t.project_id=p.id GROUP BY p.id ORDER BY p.name`)
	default:
		return s.report(ctx, `SELECT u.display_name, COUNT(t.id), COALESCE(SUM(t.duration_seconds),0), COALESCE(SUM(t.rate_cents*t.duration_seconds/3600),0) FROM users u LEFT JOIN timesheets t ON t.user_id=u.id GROUP BY u.id ORDER BY u.display_name`)
	}
}

func (s *Store) Dashboard(ctx context.Context, userID int64) (map[string]int64, error) {
	stats := map[string]int64{}
	queries := map[string]string{
		"active_timers": "SELECT COUNT(*) FROM timesheets WHERE ended_at IS NULL",
		"today_seconds": "SELECT COALESCE(SUM(duration_seconds),0) FROM timesheets WHERE user_id=? AND started_at>=?",
		"unexported":    "SELECT COUNT(*) FROM timesheets WHERE exported=0 AND billable=1 AND ended_at IS NOT NULL",
		"invoices":      "SELECT COUNT(*) FROM invoices",
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for key, query := range queries {
		var value int64
		var err error
		if key == "today_seconds" {
			err = s.db.QueryRowContext(ctx, query, userID, formatTime(today)).Scan(&value)
		} else {
			err = s.db.QueryRowContext(ctx, query).Scan(&value)
		}
		if err != nil {
			return nil, err
		}
		stats[key] = value
	}
	return stats, nil
}

func (s *Store) CreateWebhookEndpoint(ctx context.Context, w *domain.WebhookEndpoint) error {
	w.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO webhook_endpoints(name, url, secret, events, enabled, created_at) VALUES(?,?,?,?,?,?)`,
		w.Name, w.URL, w.Secret, strings.Join(w.Events, ","), boolInt(w.Enabled), formatTime(w.CreatedAt))
	if err != nil {
		return err
	}
	w.ID, err = res.LastInsertId()
	return err
}

func (s *Store) ListWebhookEndpoints(ctx context.Context) ([]domain.WebhookEndpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, secret, events, enabled, created_at FROM webhook_endpoints ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hooks []domain.WebhookEndpoint
	for rows.Next() {
		var h domain.WebhookEndpoint
		var events, created string
		if err := rows.Scan(&h.ID, &h.Name, &h.URL, &h.Secret, &events, &h.Enabled, &created); err != nil {
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

func (s *Store) QueueWebhook(ctx context.Context, event string, payload []byte) error {
	hooks, err := s.ListWebhookEndpoints(ctx)
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

func (s *Store) report(ctx context.Context, query string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, query)
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

type scanner interface {
	Scan(dest ...any) error
}

func scanTimesheet(row scanner) (domain.Timesheet, error) {
	var t domain.Timesheet
	var started, created, updated string
	var ended sql.NullString
	var internal sql.NullInt64
	err := row.Scan(&t.ID, &t.UserID, &t.CustomerID, &t.ProjectID, &t.ActivityID, &started, &ended, &t.Timezone, &t.DurationSeconds, &t.BreakSeconds, &t.RateCents, &internal, &t.Billable, &t.Exported, &t.Description, &created, &updated)
	if err != nil {
		return t, err
	}
	t.StartedAt = parseTime(started)
	if ended.Valid {
		v := parseTime(ended.String)
		t.EndedAt = &v
	}
	t.InternalRateCents = nullableInt(internal)
	t.CreatedAt = parseTime(created)
	t.UpdatedAt = parseTime(updated)
	return t, nil
}

func scanInvoice(row scanner) (domain.Invoice, error) {
	var inv domain.Invoice
	var payment, created sql.NullString
	err := row.Scan(&inv.ID, &inv.Number, &inv.CustomerID, &inv.UserID, &inv.Status, &inv.Currency, &inv.SubtotalCents, &inv.TaxCents, &inv.TotalCents, &inv.Filename, &payment, &created)
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

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS settings(name TEXT PRIMARY KEY, value TEXT);
CREATE TABLE IF NOT EXISTS roles(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
INSERT OR IGNORE INTO roles(name) VALUES('user'),('teamlead'),('admin'),('superadmin');
CREATE TABLE IF NOT EXISTS role_permissions(role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE, permission TEXT NOT NULL, allowed INTEGER NOT NULL DEFAULT 1, PRIMARY KEY(role_id, permission));
CREATE TABLE IF NOT EXISTS users(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	email TEXT NOT NULL UNIQUE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	last_login_at TEXT
);
CREATE TABLE IF NOT EXISTS user_roles(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE, PRIMARY KEY(user_id, role_id));
CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, csrf_token TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS teams(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE);
CREATE TABLE IF NOT EXISTS team_members(team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, lead INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(team_id, user_id));
CREATE TABLE IF NOT EXISTS customers(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	order_number TEXT NOT NULL DEFAULT '',
	visible INTEGER NOT NULL DEFAULT 1,
	billable INTEGER NOT NULL DEFAULT 1,
	comment TEXT NOT NULL DEFAULT '',
	legacy_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS activities(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	number TEXT NOT NULL DEFAULT '',
	visible INTEGER NOT NULL DEFAULT 1,
	billable INTEGER NOT NULL DEFAULT 1,
	comment TEXT NOT NULL DEFAULT '',
	legacy_json TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS customer_teams(customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(customer_id, team_id));
CREATE TABLE IF NOT EXISTS project_teams(project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(project_id, team_id));
CREATE TABLE IF NOT EXISTS activity_teams(activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE, team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE, PRIMARY KEY(activity_id, team_id));
CREATE TABLE IF NOT EXISTS rates(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER REFERENCES customers(id) ON DELETE CASCADE,
	project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	activity_id INTEGER REFERENCES activities(id) ON DELETE CASCADE,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	kind TEXT NOT NULL DEFAULT 'hourly',
	amount_cents INTEGER NOT NULL,
	internal_amount_cents INTEGER,
	fixed INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS tags(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, visible INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS timesheets(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	activity_id INTEGER NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
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
CREATE TABLE IF NOT EXISTS invoices(
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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
CREATE TABLE IF NOT EXISTS webhook_endpoints(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, url TEXT NOT NULL, secret TEXT NOT NULL, events TEXT NOT NULL DEFAULT '*', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL);
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
