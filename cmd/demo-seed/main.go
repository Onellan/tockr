package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"tockr/internal/db/sqlite"
	"tockr/internal/platform/config"
)

func main() {
	cfg := config.Load()
	dbPath := flag.String("db", cfg.DatabasePath, "path to the SQLite database")
	adminEmail := flag.String("admin-email", cfg.AdminEmail, "admin email to anchor demo favorites and workspace access")
	timezone := flag.String("timezone", cfg.DefaultTimezone, "timezone to use for seeded demo entries")
	currency := flag.String("currency", cfg.DefaultCurrency, "currency to use for seeded demo customers")
	flag.Parse()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	summary, err := store.SeedDemoData(ctx, *adminEmail, *timezone, *currency)
	if err != nil {
		log.Fatalf("seed demo data: %v", err)
	}

	fmt.Printf("Demo data ready for workspace %d\n", summary.WorkspaceID)
	fmt.Printf("Created users: %d\n", summary.UsersCreated)
	fmt.Printf("Created customers: %d\n", summary.CustomersCreated)
	fmt.Printf("Created projects: %d\n", summary.ProjectsCreated)
	fmt.Printf("Created activities: %d\n", summary.ActivitiesCreated)
	fmt.Printf("Created tasks: %d\n", summary.TasksCreated)
	fmt.Printf("Created favorites: %d\n", summary.FavoritesCreated)
	fmt.Printf("Created timesheets: %d\n", summary.TimesheetsCreated)
}
