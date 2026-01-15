package database

import (
	"blockchain-monitor/internal/config"
	"database/sql"
	"fmt"
	"time"

	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

var DB *sql.DB

//go:embed migrations/*.sql
var migrationsFS embed.FS

// InitDB initializes the database connection
func InitDB(cfg config.DatabaseConfig) error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %v", err)
	}

	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Configure connection pool for better performance
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxLifetime(5 * time.Minute)

	return nil
}

// RunMigrations runs the database migrations
func RunMigrations(cfg config.DatabaseConfig) error {
	driver, err := postgres.WithInstance(DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create database driver: %w", err)
	}

	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("could not create iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		d,
		cfg.DBName,
		driver,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("could not run up migrations: %w", err)
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
