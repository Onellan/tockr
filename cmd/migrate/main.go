package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"tockr/internal/db/legacy"
	"tockr/internal/db/sqlite"
)

func main() {
	sourcePath := flag.String("legacy-sqlite", "", "path to legacy Kimai SQLite database/export")
	destPath := flag.String("dest", "data/tockr.db", "path to Tockr SQLite database")
	flag.Parse()
	if *sourcePath == "" {
		log.Fatal("-legacy-sqlite is required")
	}
	ctx := context.Background()
	dest, err := sqlite.Open(ctx, *destPath)
	if err != nil {
		log.Fatal(err)
	}
	defer dest.Close()
	source, err := sql.Open("sqlite", *sourcePath)
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()
	if err := migrate(ctx, source, dest.DB()); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "migration completed")
}

func migrate(ctx context.Context, src, dst *sql.DB) error {
	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := migrateUsers(ctx, src, tx); err != nil {
		return err
	}
	if err := copyTable(ctx, src, tx, "kimai2_customers", `INSERT OR IGNORE INTO customers(id,workspace_id,name,number,company,contact,email,currency,timezone,visible,billable,comment,legacy_json,created_at) VALUES(?,1,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		[]string{"id", "name", "number", "company", "contact", "email", "currency", "timezone", "visible", "billable", "comment"}); err != nil {
		return err
	}
	if err := copyTable(ctx, src, tx, "kimai2_projects", `INSERT OR IGNORE INTO projects(id,workspace_id,customer_id,name,number,order_number,visible,billable,comment,legacy_json,created_at) VALUES(?,1,?,?,?,?,?,?,?,?,?)`,
		[]string{"id", "customer_id", "name", "number", "order_number", "visible", "billable", "comment"}); err != nil {
		return err
	}
	if err := copyTable(ctx, src, tx, "kimai2_activities", `INSERT OR IGNORE INTO activities(id,workspace_id,project_id,name,number,visible,billable,comment,legacy_json,created_at) VALUES(?,1,?,?,?,?,?,?,?,?)`,
		[]string{"id", "project_id", "name", "number", "visible", "billable", "comment"}); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log(action, entity, detail, created_at) VALUES('legacy_migration','database','kimai core import',?)`, now())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func migrateUsers(ctx context.Context, src *sql.DB, tx *sql.Tx) error {
	rows, err := src.QueryContext(ctx, `SELECT id,email,username,alias,password,timezone,enabled,roles,registration_date FROM kimai2_users`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var email, username, alias, password, timezoneValue, rolesRaw string
		var enabled int
		var registered sql.NullString
		if err := rows.Scan(&id, &email, &username, &alias, &password, &timezoneValue, &enabled, &rolesRaw, &registered); err != nil {
			return err
		}
		if timezoneValue == "" {
			timezoneValue = "UTC"
		}
		if alias == "" {
			alias = username
		}
		created := fallbackTime(registered)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO users(id,organization_id,email,username,display_name,password_hash,timezone,enabled,created_at) VALUES(?,1,?,?,?,?,?,?,?)`, id, email, username, alias, password, timezoneValue, enabled, created); err != nil {
			return err
		}
		roles := legacy.ParseRoles(rolesRaw)
		for _, role := range roles {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_roles(user_id, role_id) SELECT ?, id FROM roles WHERE name=?`, id, role); err != nil {
				return err
			}
		}
		orgRole, workspaceRole := scopedRoles(roles)
		if orgRole != "" {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organization_members(organization_id, user_id, role, created_at) VALUES(1,?,?,?)`, id, orgRole, created); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_members(workspace_id, user_id, role, created_at) VALUES(1,?,?,?)`, id, workspaceRole, created); err != nil {
			return err
		}
	}
	return rows.Err()
}

func scopedRoles(roles []string) (string, string) {
	for _, role := range roles {
		if role == "superadmin" {
			return "owner", "admin"
		}
		if role == "admin" {
			return "admin", "admin"
		}
		if role == "teamlead" {
			return "", "analyst"
		}
	}
	return "", "member"
}

func copyTable(ctx context.Context, src *sql.DB, tx *sql.Tx, table, insert string, columns []string) error {
	rows, err := src.QueryContext(ctx, "SELECT "+join(columns)+" FROM "+table) // #nosec G202
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		values = append(values, "{}", now())
		if _, err := tx.ExecContext(ctx, insert, values...); err != nil {
			return err
		}
	}
	return rows.Err()
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}

func fallbackTime(value sql.NullString) string {
	if value.Valid && value.String != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", value.String); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return now()
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
