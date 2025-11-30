package database

import (
	"blockchain-monitor/internal/config"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

var DB *sql.DB

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

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
