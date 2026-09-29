package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/local/claude-relay/internal/cryptox"
)

const apiKeyColumns = `id, name, prefix, last_four, enabled, monthly_limit_usd,
	allowed_models, expires_at, request_count, input_tokens, output_tokens, spent_usd,
	last_used_at, created_at, updated_at`

func scanAPIKey(row rowScanner) (APIKey, error) {
	var key APIKey
	var enabled int
	var modelsJSON string
	var expiresAt, lastUsed sql.NullInt64
	var created, updated int64
	err := row.Scan(&key.ID, &key.Name, &key.Prefix, &key.LastFour, &enabled,
		&key.MonthlyLimitUSD, &modelsJSON, &expiresAt, &key.RequestCount,
		&key.InputTokens, &key.OutputTokens, &key.SpentUSD, &lastUsed, &created, &updated)
	if err != nil {
		return APIKey{}, err
	}
	key.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(modelsJSON), &key.AllowedModels)
	if key.AllowedModels == nil {
		key.AllowedModels = []string{}
	}
	key.ExpiresAt = epochPtr(expiresAt)
	key.LastUsedAt = epochPtr(lastUsed)
	key.CreatedAt = time.Unix(created, 0).UTC()
	key.UpdatedAt = time.Unix(updated, 0).UTC()
	return key, nil
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]APIKey, 0)
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) GetAPIKey(ctx context.Context, id int64) (APIKey, error) {
	key, err := scanAPIKey(s.db.QueryRowContext(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	return key, err
}

func (s *Store) CreateAPIKey(ctx context.Context, input APIKeyInput) (APIKeyCreated, error) {
	random, err := cryptox.RandomToken(32)
	if err != nil {
		return APIKeyCreated{}, err
	}
	secret := "crp_live_" + random
	prefixLen := min(16, len(secret))
	models, _ := json.Marshal(normalizeModels(input.AllowedModels))
	now := time.Now().Unix()
	var expires any
	if input.ExpiresAt != nil {
		expires = input.ExpiresAt.Unix()
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO api_keys(
		name, secret_hash, prefix, last_four, enabled, monthly_limit_usd,
		allowed_models, expires_at, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(input.Name), cryptox.HashSecret(secret), secret[:prefixLen], secret[len(secret)-4:],
		boolInt(input.Enabled), max(input.MonthlyLimitUSD, 0), string(models), expires, now, now)
	if err != nil {
		return APIKeyCreated{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return APIKeyCreated{}, err
	}
	key, err := s.GetAPIKey(ctx, id)
	return APIKeyCreated{APIKey: key, Secret: secret}, err
}

func (s *Store) UpdateAPIKey(ctx context.Context, id int64, input APIKeyInput) (APIKey, error) {
	models, _ := json.Marshal(normalizeModels(input.AllowedModels))
	var expires any
	if input.ExpiresAt != nil {
		expires = input.ExpiresAt.Unix()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE api_keys SET name = ?, enabled = ?,
		monthly_limit_usd = ?, allowed_models = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(input.Name), boolInt(input.Enabled), max(input.MonthlyLimitUSD, 0),
		string(models), expires, time.Now().Unix(), id)
	if err != nil {
		return APIKey{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return APIKey{}, ErrNotFound
	}
	return s.GetAPIKey(ctx, id)
}

func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AuthenticateAPIKey(ctx context.Context, secret string) (APIKey, error) {
	key, err := scanAPIKey(s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+`
		FROM api_keys WHERE secret_hash = ?`, cryptox.HashSecret(strings.TrimSpace(secret))))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!key.Enabled || (key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now())))) {
		return APIKey{}, ErrNotFound
	}
	return key, err
}

func (s *Store) APIKeyMonthSpend(ctx context.Context, id int64, now time.Time) (float64, error) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	var spend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM usage_logs WHERE api_key_id = ? AND created_at >= ?`,
		id, start).Scan(&spend)
	return spend, err
}

func IsModelAllowed(allowed []string, model string) bool {
	return modelAllowed(allowed, model)
}
