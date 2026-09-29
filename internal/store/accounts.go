package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const accountColumns = `id, name, base_url, key_hint, enabled, priority, weight,
	allowed_models, monthly_limit_usd, notes, status, cooldown_until,
	consecutive_failures, request_count, input_tokens, output_tokens, spent_usd,
	last_used_at, last_success_at, last_error, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (Account, error) {
	var account Account
	var enabled int
	var modelsJSON string
	var cooldown, lastUsed, lastSuccess sql.NullInt64
	var created, updated int64
	err := row.Scan(
		&account.ID, &account.Name, &account.BaseURL, &account.KeyHint, &enabled,
		&account.Priority, &account.Weight, &modelsJSON, &account.MonthlyLimitUSD,
		&account.Notes, &account.Status, &cooldown, &account.ConsecutiveFailures,
		&account.RequestCount, &account.InputTokens, &account.OutputTokens, &account.SpentUSD,
		&lastUsed, &lastSuccess, &account.LastError, &created, &updated,
	)
	if err != nil {
		return Account{}, err
	}
	account.Enabled = enabled == 1
	_ = json.Unmarshal([]byte(modelsJSON), &account.AllowedModels)
	if account.AllowedModels == nil {
		account.AllowedModels = []string{}
	}
	account.CooldownUntil = epochPtr(cooldown)
	account.LastUsedAt = epochPtr(lastUsed)
	account.LastSuccessAt = epochPtr(lastSuccess)
	account.CreatedAt = time.Unix(created, 0).UTC()
	account.UpdatedAt = time.Unix(updated, 0).UTC()
	return account, nil
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountColumns+` FROM accounts ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]Account, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) GetAccount(ctx context.Context, id int64) (Account, error) {
	account, err := scanAccount(s.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

func (s *Store) GetAccountSecret(ctx context.Context, id int64) (AccountSecret, error) {
	account, err := s.GetAccount(ctx, id)
	if err != nil {
		return AccountSecret{}, err
	}
	var encrypted string
	if err := s.db.QueryRowContext(ctx, `SELECT api_key_enc FROM accounts WHERE id = ?`, id).Scan(&encrypted); err != nil {
		return AccountSecret{}, err
	}
	secret, err := s.vault.Decrypt(encrypted)
	if err != nil {
		return AccountSecret{}, err
	}
	return AccountSecret{Account: account, APIKey: secret}, nil
}

func (s *Store) CreateAccount(ctx context.Context, input AccountInput) (Account, error) {
	models, _ := json.Marshal(normalizeModels(input.AllowedModels))
	encrypted, err := s.vault.Encrypt(strings.TrimSpace(input.APIKey))
	if err != nil {
		return Account{}, err
	}
	now := time.Now().Unix()
	result, err := s.db.ExecContext(ctx, `INSERT INTO accounts(
		name, base_url, api_key_enc, key_hint, enabled, priority, weight,
		allowed_models, monthly_limit_usd, notes, status, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'unknown', ?, ?)`,
		strings.TrimSpace(input.Name), normalizeBaseURL(input.BaseURL), encrypted, keyHint(input.APIKey),
		boolInt(input.Enabled), input.Priority, max(input.Weight, 1), string(models),
		max(input.MonthlyLimitUSD, 0), strings.TrimSpace(input.Notes), now, now)
	if err != nil {
		return Account{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, id)
}

func (s *Store) UpdateAccount(ctx context.Context, id int64, input AccountInput) (Account, error) {
	var encrypted, hint string
	if err := s.db.QueryRowContext(ctx, `SELECT api_key_enc, key_hint FROM accounts WHERE id = ?`, id).Scan(&encrypted, &hint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Account{}, ErrNotFound
		}
		return Account{}, err
	}
	if strings.TrimSpace(input.APIKey) != "" {
		var err error
		encrypted, err = s.vault.Encrypt(strings.TrimSpace(input.APIKey))
		if err != nil {
			return Account{}, err
		}
		hint = keyHint(input.APIKey)
	}
	models, _ := json.Marshal(normalizeModels(input.AllowedModels))
	result, err := s.db.ExecContext(ctx, `UPDATE accounts SET
		name = ?, base_url = ?, api_key_enc = ?, key_hint = ?, enabled = ?,
		priority = ?, weight = ?, allowed_models = ?, monthly_limit_usd = ?,
		notes = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(input.Name), normalizeBaseURL(input.BaseURL), encrypted, hint,
		boolInt(input.Enabled), input.Priority, max(input.Weight, 1), string(models),
		max(input.MonthlyLimitUSD, 0), strings.TrimSpace(input.Notes), time.Now().Unix(), id)
	if err != nil {
		return Account{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Account{}, ErrNotFound
	}
	return s.GetAccount(ctx, id)
}

func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetAccountEnabled(ctx context.Context, id int64, enabled bool) error {
	status := "disabled"
	if enabled {
		status = "unknown"
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET enabled = ?, status = ?, cooldown_until = NULL, updated_at = ? WHERE id = ?`,
		boolInt(enabled), status, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAccountSuccess(ctx context.Context, id int64) {
	now := time.Now().Unix()
	_, _ = s.db.ExecContext(ctx, `UPDATE accounts SET status = 'healthy',
		cooldown_until = NULL, consecutive_failures = 0, last_success_at = ?,
		last_error = '', updated_at = ? WHERE id = ?`, now, now, id)
}

func (s *Store) MarkAccountFailure(ctx context.Context, id int64, message string, cooldown time.Duration, disable bool) {
	status := "degraded"
	var cooldownUntil any
	if disable {
		status = "disabled"
	} else if cooldown > 0 {
		status = "cooldown"
		cooldownUntil = time.Now().Add(cooldown).Unix()
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE accounts SET status = ?,
		cooldown_until = ?, consecutive_failures = consecutive_failures + 1,
		last_error = ?, updated_at = ? WHERE id = ?`,
		status, cooldownUntil, truncate(message, 500), time.Now().Unix(), id)
}

func (s *Store) CandidateAccounts(ctx context.Context, model string) ([]AccountSecret, error) {
	accounts, err := s.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	candidates := make([]AccountSecret, 0, len(accounts))
	for _, account := range accounts {
		if !account.Enabled || account.Status == "disabled" || (account.CooldownUntil != nil && account.CooldownUntil.After(now)) {
			continue
		}
		if !modelAllowed(account.AllowedModels, model) {
			continue
		}
		if account.MonthlyLimitUSD > 0 {
			spent, err := s.AccountMonthSpend(ctx, account.ID, now)
			if err != nil {
				return nil, err
			}
			if spent >= account.MonthlyLimitUSD {
				continue
			}
		}
		secret, err := s.GetAccountSecret(ctx, account.ID)
		if err != nil {
			continue
		}
		candidates = append(candidates, secret)
	}
	return candidates, nil
}

func (s *Store) AccountMonthSpend(ctx context.Context, id int64, now time.Time) (float64, error) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	var spend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM usage_logs WHERE account_id = ? AND created_at >= ?`,
		id, start).Scan(&spend)
	return spend, err
}

func (s *Store) BootstrapAccount(ctx context.Context, apiKey, baseURL string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 || strings.TrimSpace(apiKey) == "" {
		return false, nil
	}
	_, err := s.CreateAccount(ctx, AccountInput{
		Name: "Anthropic", BaseURL: baseURL, APIKey: apiKey, Enabled: true, Weight: 1,
	})
	return err == nil, err
}

func normalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "https://api.anthropic.com"
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func modelAllowed(allowed []string, model string) bool {
	if len(allowed) == 0 || model == "" {
		return true
	}
	for _, pattern := range allowed {
		if pattern == "*" || pattern == model || (strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))) {
			return true
		}
	}
	return false
}

func keyHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func (s *Store) ResetAccountHealth(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE accounts SET status = 'unknown', cooldown_until = NULL,
		consecutive_failures = 0, last_error = '', updated_at = ? WHERE id = ?`, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("%w: account", ErrNotFound)
	}
	return nil
}
