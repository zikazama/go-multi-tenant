package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

func NewConnection(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func RunMigrations(db *sql.DB) error {
	migrations := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,

		`CREATE TABLE IF NOT EXISTS tenants (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,

		`CREATE TABLE IF NOT EXISTS messages (
			id UUID NOT NULL DEFAULT uuid_generate_v4(),
			tenant_id UUID NOT NULL,
			payload JSONB,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (id, tenant_id)
		) PARTITION BY LIST (tenant_id);`,

		`CREATE TABLE IF NOT EXISTS tenant_configs (
			tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
			workers INTEGER NOT NULL DEFAULT 3,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	return nil
}

func CreateTenantPartition(db *sql.DB, tenantID string) error {
	safeTenantID := strings.ReplaceAll(tenantID, "-", "_")
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS messages_%s 
		PARTITION OF messages 
		FOR VALUES IN ('%s');
	`, safeTenantID, tenantID)

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create partition for tenant %s: %w", tenantID, err)
	}

	return nil
}

func DropTenantPartition(db *sql.DB, tenantID string) error {
	safeTenantID := strings.ReplaceAll(tenantID, "-", "_")
	query := fmt.Sprintf(`DROP TABLE IF EXISTS messages_%s;`, safeTenantID)
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to drop partition for tenant %s: %w", tenantID, err)
	}

	return nil
}
