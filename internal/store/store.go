package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/local/claude-relay/internal/cryptox"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db    *sql.DB
	vault *cryptox.Vault
}

func Open(path string, vault *cryptox.Vault) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	// _pragma applies connection-scoped settings to every pooled SQLite connection.
	dsn := path + "?_pragma=busy_timeout%3d5000&_pragma=foreign_keys%3dON&_pragma=journal_mode%3dWAL&_pragma=synchronous%3dNORMAL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}

	s := &Store{db: db, vault: vault}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS admin_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_login_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key_enc TEXT NOT NULL,
			key_hint TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 1,
			allowed_models TEXT NOT NULL DEFAULT '[]',
			monthly_limit_usd REAL NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'unknown',
			cooldown_until INTEGER,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			request_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			spent_usd REAL NOT NULL DEFAULT 0,
			last_used_at INTEGER,
			last_success_at INTEGER,
			last_error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_accounts_select ON accounts(enabled, status, cooldown_until, priority)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			secret_hash TEXT NOT NULL UNIQUE,
			prefix TEXT NOT NULL,
			last_four TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			monthly_limit_usd REAL NOT NULL DEFAULT 0,
			allowed_models TEXT NOT NULL DEFAULT '[]',
			expires_at INTEGER,
			request_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			spent_usd REAL NOT NULL DEFAULT 0,
			last_used_at INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(secret_hash)`,
		`CREATE TABLE IF NOT EXISTS usage_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT NOT NULL UNIQUE,
			api_key_id INTEGER REFERENCES api_keys(id) ON DELETE SET NULL,
			account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
			model TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
			cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			cost_usd REAL NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			time_to_first_byte_ms INTEGER NOT NULL DEFAULT 0,
			streamed INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			client_ip TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_key_created ON usage_logs(api_key_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_account_created ON usage_logs(account_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_model_created ON usage_logs(model, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS model_prices (
			match TEXT PRIMARY KEY,
			input_per_million_usd REAL NOT NULL,
			output_per_million_usd REAL NOT NULL,
			cache_read_per_million_usd REAL NOT NULL,
			cache_write_per_million_usd REAL NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER REFERENCES admin_users(id) ON DELETE SET NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}

	now := time.Now().Unix()
	defaults := map[string]string{
		"app_name":                  "Claude Relay",
		"default_anthropic_version": "2023-06-01",
		"request_timeout_seconds":   "600",
		"max_retries":               "1",
		"routing_strategy":          "least_recently_used",
		"failure_threshold":         "3",
		"cooldown_seconds":          "60",
		"log_retention_days":        "30",
		"session_ttl_hours":         "24",
	}
	for key, value := range defaults {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO settings(key, value, updated_at) VALUES(?, ?, ?)`, key, value, now); err != nil {
			return fmt.Errorf("seed setting %s: %w", key, err)
		}
	}

	prices := []ModelPrice{
		{Match: "claude-opus", InputPerMillionUSD: 15, OutputPerMillionUSD: 75, CacheReadPerMillionUSD: 1.5, CacheWritePerMillionUSD: 18.75},
		{Match: "claude-sonnet", InputPerMillionUSD: 3, OutputPerMillionUSD: 15, CacheReadPerMillionUSD: 0.3, CacheWritePerMillionUSD: 3.75},
		{Match: "claude-haiku", InputPerMillionUSD: 1, OutputPerMillionUSD: 5, CacheReadPerMillionUSD: 0.1, CacheWritePerMillionUSD: 1.25},
	}
	for _, price := range prices {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO model_prices(
			match, input_per_million_usd, output_per_million_usd, cache_read_per_million_usd,
			cache_write_per_million_usd, updated_at) VALUES(?, ?, ?, ?, ?, ?)`,
			price.Match, price.InputPerMillionUSD, price.OutputPerMillionUSD,
			price.CacheReadPerMillionUSD, price.CacheWritePerMillionUSD, now); err != nil {
			return fmt.Errorf("seed model price %s: %w", price.Match, err)
		}
	}
	return nil
}

func epochPtr(value sql.NullInt64) *time.Time {
	if !value.Valid || value.Int64 <= 0 {
		return nil
	}
	t := time.Unix(value.Int64, 0).UTC()
	return &t
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
