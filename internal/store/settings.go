package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, err
		}
		values[key] = value
	}
	return Settings{
		AppName:                 values["app_name"],
		DefaultAnthropicVersion: values["default_anthropic_version"],
		RequestTimeoutSeconds:   parseInt(values["request_timeout_seconds"], 600),
		MaxRetries:              parseInt(values["max_retries"], 1),
		RoutingStrategy:         values["routing_strategy"],
		FailureThreshold:        parseInt(values["failure_threshold"], 3),
		CooldownSeconds:         parseInt(values["cooldown_seconds"], 60),
		LogRetentionDays:        parseInt(values["log_retention_days"], 30),
		SessionTTLHours:         parseInt(values["session_ttl_hours"], 24),
	}, rows.Err()
}

func (s *Store) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	if strings.TrimSpace(settings.AppName) == "" {
		return Settings{}, errors.New("app name cannot be empty")
	}
	if strings.TrimSpace(settings.DefaultAnthropicVersion) == "" {
		return Settings{}, errors.New("anthropic version cannot be empty")
	}
	if settings.RequestTimeoutSeconds < 10 || settings.RequestTimeoutSeconds > 3600 {
		return Settings{}, errors.New("request timeout must be between 10 and 3600 seconds")
	}
	if settings.MaxRetries < 0 || settings.MaxRetries > 5 {
		return Settings{}, errors.New("max retries must be between 0 and 5")
	}
	if settings.RoutingStrategy != "least_recently_used" && settings.RoutingStrategy != "priority" && settings.RoutingStrategy != "weighted" {
		return Settings{}, errors.New("unsupported routing strategy")
	}
	if settings.FailureThreshold < 1 || settings.FailureThreshold > 20 {
		return Settings{}, errors.New("failure threshold must be between 1 and 20")
	}
	if settings.CooldownSeconds < 5 || settings.CooldownSeconds > 86400 {
		return Settings{}, errors.New("cooldown must be between 5 and 86400 seconds")
	}
	if settings.LogRetentionDays < 1 || settings.LogRetentionDays > 3650 {
		return Settings{}, errors.New("log retention must be between 1 and 3650 days")
	}
	if settings.SessionTTLHours < 1 || settings.SessionTTLHours > 720 {
		return Settings{}, errors.New("session TTL must be between 1 and 720 hours")
	}

	values := map[string]string{
		"app_name":                  strings.TrimSpace(settings.AppName),
		"default_anthropic_version": strings.TrimSpace(settings.DefaultAnthropicVersion),
		"request_timeout_seconds":   strconv.Itoa(settings.RequestTimeoutSeconds),
		"max_retries":               strconv.Itoa(settings.MaxRetries),
		"routing_strategy":          settings.RoutingStrategy,
		"failure_threshold":         strconv.Itoa(settings.FailureThreshold),
		"cooldown_seconds":          strconv.Itoa(settings.CooldownSeconds),
		"log_retention_days":        strconv.Itoa(settings.LogRetentionDays),
		"session_ttl_hours":         strconv.Itoa(settings.SessionTTLHours),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value, time.Now().Unix()); err != nil {
			return Settings{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, err
	}
	return s.GetSettings(ctx)
}

func (s *Store) ListModelPrices(ctx context.Context) ([]ModelPrice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT match, input_per_million_usd,
		output_per_million_usd, cache_read_per_million_usd,
		cache_write_per_million_usd, updated_at FROM model_prices ORDER BY match`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := make([]ModelPrice, 0)
	for rows.Next() {
		var price ModelPrice
		var updated int64
		if err := rows.Scan(&price.Match, &price.InputPerMillionUSD, &price.OutputPerMillionUSD,
			&price.CacheReadPerMillionUSD, &price.CacheWritePerMillionUSD, &updated); err != nil {
			return nil, err
		}
		price.UpdatedAt = time.Unix(updated, 0).UTC()
		prices = append(prices, price)
	}
	return prices, rows.Err()
}

func (s *Store) UpsertModelPrice(ctx context.Context, price ModelPrice) (ModelPrice, error) {
	price.Match = strings.TrimSpace(price.Match)
	if price.Match == "" || strings.ContainsAny(price.Match, " \t\r\n") {
		return ModelPrice{}, errors.New("model match must be a non-empty prefix without spaces")
	}
	if price.InputPerMillionUSD < 0 || price.OutputPerMillionUSD < 0 ||
		price.CacheReadPerMillionUSD < 0 || price.CacheWritePerMillionUSD < 0 {
		return ModelPrice{}, errors.New("prices cannot be negative")
	}
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO model_prices(match, input_per_million_usd,
		output_per_million_usd, cache_read_per_million_usd, cache_write_per_million_usd, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(match) DO UPDATE SET input_per_million_usd = excluded.input_per_million_usd,
		output_per_million_usd = excluded.output_per_million_usd,
		cache_read_per_million_usd = excluded.cache_read_per_million_usd,
		cache_write_per_million_usd = excluded.cache_write_per_million_usd,
		updated_at = excluded.updated_at`, price.Match, price.InputPerMillionUSD,
		price.OutputPerMillionUSD, price.CacheReadPerMillionUSD, price.CacheWritePerMillionUSD, now)
	if err != nil {
		return ModelPrice{}, err
	}
	price.UpdatedAt = time.Unix(now, 0).UTC()
	return price, nil
}

func (s *Store) DeleteModelPrice(ctx context.Context, match string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM model_prices WHERE match = ?`, match)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *Store) DatabaseStats(ctx context.Context) (map[string]any, error) {
	stats := map[string]any{}
	for name, query := range map[string]string{
		"accounts":   `SELECT COUNT(*) FROM accounts`,
		"api_keys":   `SELECT COUNT(*) FROM api_keys`,
		"usage_logs": `SELECT COUNT(*) FROM usage_logs`,
	} {
		var value int64
		if err := s.db.QueryRowContext(ctx, query).Scan(&value); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("count %s: %w", name, err)
		}
		stats[name] = value
	}
	return stats, nil
}
