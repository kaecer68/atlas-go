package db

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Init initializes the PostgreSQL connection pool and runs migrations.
func Init(ctx context.Context, databaseURL, migrationsPath string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL not provided")
	}

	p, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if migrationsPath != "" {
		if err := runMigrations(databaseURL, migrationsPath); err != nil {
			p.Close()
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	}

	return p, nil
}

func runMigrations(databaseURL, migrationsPath string) error {
	driverURL := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	m, err := migrate.New(
		"file://"+migrationsPath,
		driverURL,
	)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
