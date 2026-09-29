package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

func (s *Store) RecordUsage(ctx context.Context, usage UsageInsert) error {
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var keyID, accountID any
	if usage.APIKeyID > 0 {
		keyID = usage.APIKeyID
	}
	if usage.AccountID > 0 {
		accountID = usage.AccountID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(
		request_id, api_key_id, account_id, model, endpoint, status_code,
		input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		cost_usd, duration_ms, time_to_first_byte_ms, streamed, error, client_ip, created_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		usage.RequestID, keyID, accountID, usage.Model, usage.Endpoint, usage.StatusCode,
		usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreationTokens,
		usage.CostUSD, usage.DurationMS, usage.TimeToFirstByteMS, boolInt(usage.Streamed),
		truncate(usage.Error, 1000), usage.ClientIP, usage.CreatedAt.Unix())
	if err != nil {
		return err
	}
	if usage.APIKeyID > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE api_keys SET request_count = request_count + 1,
			input_tokens = input_tokens + ?, output_tokens = output_tokens + ?,
			spent_usd = spent_usd + ?, last_used_at = ?, updated_at = ? WHERE id = ?`,
			usage.InputTokens, usage.OutputTokens, usage.CostUSD, usage.CreatedAt.Unix(),
			usage.CreatedAt.Unix(), usage.APIKeyID)
		if err != nil {
			return err
		}
	}
	if usage.AccountID > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE accounts SET request_count = request_count + 1,
			input_tokens = input_tokens + ?, output_tokens = output_tokens + ?,
			spent_usd = spent_usd + ?, last_used_at = ?, updated_at = ? WHERE id = ?`,
			usage.InputTokens, usage.OutputTokens, usage.CostUSD, usage.CreatedAt.Unix(),
			usage.CreatedAt.Unix(), usage.AccountID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListUsage(ctx context.Context, filter UsageFilter) (UsageList, error) {
	where, args := usageWhere(filter)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs u `+where, args...).Scan(&total); err != nil {
		return UsageList{}, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	query := `SELECT u.id, u.request_id, COALESCE(u.api_key_id, 0), COALESCE(k.name, ''),
		COALESCE(u.account_id, 0), COALESCE(a.name, ''), u.model, u.endpoint,
		u.status_code, u.input_tokens, u.output_tokens, u.cache_read_tokens,
		u.cache_creation_tokens, u.cost_usd, u.duration_ms, u.time_to_first_byte_ms,
		u.streamed, u.error, u.client_ip, u.created_at
		FROM usage_logs u
		LEFT JOIN api_keys k ON k.id = u.api_key_id
		LEFT JOIN accounts a ON a.id = u.account_id ` + where + `
		ORDER BY u.created_at DESC, u.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return UsageList{}, err
	}
	defer rows.Close()
	items := make([]UsageLog, 0, limit)
	for rows.Next() {
		item, err := scanUsage(rows)
		if err != nil {
			return UsageList{}, err
		}
		items = append(items, item)
	}
	return UsageList{Items: items, Total: total}, rows.Err()
}

func scanUsage(row rowScanner) (UsageLog, error) {
	var item UsageLog
	var streamed int
	var created int64
	err := row.Scan(&item.ID, &item.RequestID, &item.APIKeyID, &item.APIKeyName,
		&item.AccountID, &item.AccountName, &item.Model, &item.Endpoint,
		&item.StatusCode, &item.InputTokens, &item.OutputTokens, &item.CacheReadTokens,
		&item.CacheCreationTokens, &item.CostUSD, &item.DurationMS, &item.TimeToFirstByteMS,
		&streamed, &item.Error, &item.ClientIP, &created)
	if err != nil {
		return UsageLog{}, err
	}
	item.Streamed = streamed == 1
	item.CreatedAt = time.Unix(created, 0).UTC()
	return item, nil
}

func usageWhere(filter UsageFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0)
	if filter.APIKeyID > 0 {
		clauses = append(clauses, "u.api_key_id = ?")
		args = append(args, filter.APIKeyID)
	}
	if filter.AccountID > 0 {
		clauses = append(clauses, "u.account_id = ?")
		args = append(args, filter.AccountID)
	}
	if filter.Model != "" {
		clauses = append(clauses, "u.model = ?")
		args = append(args, filter.Model)
	}
	switch filter.Status {
	case "success":
		clauses = append(clauses, "u.status_code >= 200 AND u.status_code < 400")
	case "error":
		clauses = append(clauses, "u.status_code >= 400")
	}
	if filter.Search != "" {
		needle := "%" + strings.TrimSpace(filter.Search) + "%"
		clauses = append(clauses, "(u.request_id LIKE ? OR u.model LIKE ? OR u.error LIKE ? OR u.client_ip LIKE ?)")
		args = append(args, needle, needle, needle, needle)
	}
	if !filter.From.IsZero() {
		clauses = append(clauses, "u.created_at >= ?")
		args = append(args, filter.From.Unix())
	}
	if !filter.To.IsZero() {
		clauses = append(clauses, "u.created_at < ?")
		args = append(args, filter.To.Unix())
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

type aggregate struct {
	requests int64
	tokens   int64
	cost     float64
	success  float64
	latency  int64
}

func (s *Store) aggregatePeriod(ctx context.Context, from, to time.Time) (aggregate, error) {
	var a aggregate
	var successes int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(input_tokens + output_tokens), 0), COALESCE(SUM(cost_usd), 0),
		COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(duration_ms), 0)
		FROM usage_logs WHERE created_at >= ? AND created_at < ?`, from.Unix(), to.Unix()).
		Scan(&a.requests, &a.tokens, &a.cost, &successes, &a.latency)
	if a.requests > 0 {
		a.success = float64(successes) / float64(a.requests) * 100
	}
	return a, err
}

func metric(label string, current, previous float64) MetricDelta {
	m := MetricDelta{Label: label, Value: current, Previous: previous, HasPrevious: previous != 0}
	if previous != 0 {
		m.ChangePct = (current - previous) / previous * 100
	}
	return m
}

func (s *Store) Dashboard(ctx context.Context, hours int) (Dashboard, error) {
	if hours != 24 && hours != 168 && hours != 720 {
		hours = 24
	}
	// SQLite stores second precision timestamps. Move the exclusive upper bound one
	// second ahead so requests written in the current second appear immediately.
	now := time.Now().UTC().Add(time.Second)
	start := now.Add(-time.Duration(hours) * time.Hour)
	previousStart := start.Add(-time.Duration(hours) * time.Hour)
	current, err := s.aggregatePeriod(ctx, start, now)
	if err != nil {
		return Dashboard{}, err
	}
	previous, err := s.aggregatePeriod(ctx, previousStart, start)
	if err != nil {
		return Dashboard{}, err
	}

	dashboard := Dashboard{
		PeriodHours:      hours,
		Requests:         metric("requests", float64(current.requests), float64(previous.requests)),
		Tokens:           metric("tokens", float64(current.tokens), float64(previous.tokens)),
		Cost:             metric("cost", current.cost, previous.cost),
		SuccessRate:      metric("success_rate", current.success, previous.success),
		AverageLatencyMS: current.latency,
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0) FROM accounts`).
		Scan(&dashboard.TotalAccounts, &dashboard.ActiveAccounts)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE enabled = 1 AND (expires_at IS NULL OR expires_at > ?)`, now.Unix()).
		Scan(&dashboard.ActiveKeys)

	dashboard.Series, err = s.usageSeries(ctx, start, now, hours)
	if err != nil {
		return Dashboard{}, err
	}
	dashboard.Models, err = s.modelBreakdown(ctx, start, now)
	if err != nil {
		return Dashboard{}, err
	}
	recent, err := s.ListUsage(ctx, UsageFilter{From: start, To: now, Limit: 8})
	if err != nil {
		return Dashboard{}, err
	}
	dashboard.Recent = recent.Items
	return dashboard, nil
}

func (s *Store) usageSeries(ctx context.Context, from, to time.Time, hours int) ([]SeriesPoint, error) {
	format := "%Y-%m-%dT%H:00:00Z"
	step := time.Hour
	if hours > 168 {
		format = "%Y-%m-%dT00:00:00Z"
		step = 24 * time.Hour
	}
	rows, err := s.db.QueryContext(ctx, `SELECT strftime(?, created_at, 'unixepoch') AS bucket,
		COUNT(*), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cost_usd), 0),
		COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0)
		FROM usage_logs WHERE created_at >= ? AND created_at < ?
		GROUP BY bucket ORDER BY bucket`, format, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[int64]SeriesPoint{}
	for rows.Next() {
		var raw string
		var point SeriesPoint
		if err := rows.Scan(&raw, &point.Requests, &point.InputTokens, &point.OutputTokens, &point.CostUSD, &point.Errors); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}
		point.Bucket = parsed
		values[parsed.Unix()] = point
	}

	cursor := from.Truncate(step)
	points := make([]SeriesPoint, 0, int(to.Sub(cursor)/step)+1)
	for cursor.Before(to) || cursor.Equal(to) {
		if point, ok := values[cursor.Unix()]; ok {
			points = append(points, point)
		} else {
			points = append(points, SeriesPoint{Bucket: cursor})
		}
		cursor = cursor.Add(step)
	}
	return points, rows.Err()
}

func (s *Store) modelBreakdown(ctx context.Context, from, to time.Time) ([]ModelBreakdown, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT model, COUNT(*),
		COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_usd), 0)
		FROM usage_logs WHERE created_at >= ? AND created_at < ?
		GROUP BY model ORDER BY COUNT(*) DESC LIMIT 8`, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]ModelBreakdown, 0)
	for rows.Next() {
		var model ModelBreakdown
		if err := rows.Scan(&model.Model, &model.Requests, &model.InputTokens, &model.OutputTokens, &model.CostUSD); err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *Store) EstimateCost(ctx context.Context, model string, input, output, cacheRead, cacheWrite int64) float64 {
	var price ModelPrice
	err := s.db.QueryRowContext(ctx, `SELECT match, input_per_million_usd,
		output_per_million_usd, cache_read_per_million_usd, cache_write_per_million_usd, updated_at
		FROM model_prices WHERE ? LIKE match || '%' ORDER BY LENGTH(match) DESC LIMIT 1`, model).
		Scan(&price.Match, &price.InputPerMillionUSD, &price.OutputPerMillionUSD,
			&price.CacheReadPerMillionUSD, &price.CacheWritePerMillionUSD, new(int64))
	if err != nil {
		return 0
	}
	cost := (float64(input)*price.InputPerMillionUSD +
		float64(output)*price.OutputPerMillionUSD +
		float64(cacheRead)*price.CacheReadPerMillionUSD +
		float64(cacheWrite)*price.CacheWritePerMillionUSD) / 1_000_000
	return math.Round(cost*1e8) / 1e8
}

func (s *Store) PurgeUsageBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM usage_logs WHERE created_at < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) DistinctModels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT model FROM usage_logs WHERE model != '' ORDER BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]string, 0)
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *Store) UsageSummary(ctx context.Context, filter UsageFilter) (map[string]any, error) {
	where, args := usageWhere(filter)
	var requests, input, output, errorsCount, avgLatency int64
	var cost float64
	err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_usd), 0),
		COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(duration_ms), 0) FROM usage_logs u %s`, where), args...).
		Scan(&requests, &input, &output, &cost, &errorsCount, &avgLatency)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return map[string]any{
		"requests": requests, "input_tokens": input, "output_tokens": output,
		"cost_usd": cost, "errors": errorsCount, "average_latency_ms": avgLatency,
	}, nil
}
