package store

import "time"

type AdminUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Account struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	BaseURL             string     `json:"base_url"`
	KeyHint             string     `json:"key_hint"`
	Enabled             bool       `json:"enabled"`
	Priority            int        `json:"priority"`
	Weight              int        `json:"weight"`
	AllowedModels       []string   `json:"allowed_models"`
	MonthlyLimitUSD     float64    `json:"monthly_limit_usd"`
	Notes               string     `json:"notes"`
	Status              string     `json:"status"`
	CooldownUntil       *time.Time `json:"cooldown_until,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	RequestCount        int64      `json:"request_count"`
	InputTokens         int64      `json:"input_tokens"`
	OutputTokens        int64      `json:"output_tokens"`
	SpentUSD            float64    `json:"spent_usd"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type AccountInput struct {
	Name            string   `json:"name"`
	BaseURL         string   `json:"base_url"`
	APIKey          string   `json:"api_key"`
	Enabled         bool     `json:"enabled"`
	Priority        int      `json:"priority"`
	Weight          int      `json:"weight"`
	AllowedModels   []string `json:"allowed_models"`
	MonthlyLimitUSD float64  `json:"monthly_limit_usd"`
	Notes           string   `json:"notes"`
}

type AccountSecret struct {
	Account
	APIKey string `json:"-"`
}

type APIKey struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Prefix          string     `json:"prefix"`
	LastFour        string     `json:"last_four"`
	Enabled         bool       `json:"enabled"`
	MonthlyLimitUSD float64    `json:"monthly_limit_usd"`
	AllowedModels   []string   `json:"allowed_models"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RequestCount    int64      `json:"request_count"`
	InputTokens     int64      `json:"input_tokens"`
	OutputTokens    int64      `json:"output_tokens"`
	SpentUSD        float64    `json:"spent_usd"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type APIKeyInput struct {
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	MonthlyLimitUSD float64    `json:"monthly_limit_usd"`
	AllowedModels   []string   `json:"allowed_models"`
	ExpiresAt       *time.Time `json:"expires_at"`
}

type APIKeyCreated struct {
	APIKey
	Secret string `json:"secret"`
}

type UsageLog struct {
	ID                  int64     `json:"id"`
	RequestID           string    `json:"request_id"`
	APIKeyID            int64     `json:"api_key_id"`
	APIKeyName          string    `json:"api_key_name"`
	AccountID           int64     `json:"account_id"`
	AccountName         string    `json:"account_name"`
	Model               string    `json:"model"`
	Endpoint            string    `json:"endpoint"`
	StatusCode          int       `json:"status_code"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CostUSD             float64   `json:"cost_usd"`
	DurationMS          int64     `json:"duration_ms"`
	TimeToFirstByteMS   int64     `json:"time_to_first_byte_ms"`
	Streamed            bool      `json:"streamed"`
	Error               string    `json:"error,omitempty"`
	ClientIP            string    `json:"client_ip"`
	CreatedAt           time.Time `json:"created_at"`
}

type UsageInsert struct {
	RequestID           string
	APIKeyID            int64
	AccountID           int64
	Model               string
	Endpoint            string
	StatusCode          int
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	CostUSD             float64
	DurationMS          int64
	TimeToFirstByteMS   int64
	Streamed            bool
	Error               string
	ClientIP            string
	CreatedAt           time.Time
}

type UsageFilter struct {
	APIKeyID  int64
	AccountID int64
	Model     string
	Status    string
	Search    string
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

type UsageList struct {
	Items []UsageLog `json:"items"`
	Total int64      `json:"total"`
}

type MetricDelta struct {
	Label       string  `json:"label"`
	Value       float64 `json:"value"`
	Previous    float64 `json:"previous"`
	ChangePct   float64 `json:"change_pct"`
	HasPrevious bool    `json:"has_previous"`
}

type SeriesPoint struct {
	Bucket       time.Time `json:"bucket"`
	Requests     int64     `json:"requests"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Errors       int64     `json:"errors"`
}

type ModelBreakdown struct {
	Model        string  `json:"model"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type Dashboard struct {
	PeriodHours      int              `json:"period_hours"`
	Requests         MetricDelta      `json:"requests"`
	Tokens           MetricDelta      `json:"tokens"`
	Cost             MetricDelta      `json:"cost"`
	SuccessRate      MetricDelta      `json:"success_rate"`
	AverageLatencyMS int64            `json:"average_latency_ms"`
	ActiveAccounts   int64            `json:"active_accounts"`
	TotalAccounts    int64            `json:"total_accounts"`
	ActiveKeys       int64            `json:"active_keys"`
	Series           []SeriesPoint    `json:"series"`
	Models           []ModelBreakdown `json:"models"`
	Recent           []UsageLog       `json:"recent"`
}

type ModelPrice struct {
	Match                   string    `json:"match"`
	InputPerMillionUSD      float64   `json:"input_per_million_usd"`
	OutputPerMillionUSD     float64   `json:"output_per_million_usd"`
	CacheReadPerMillionUSD  float64   `json:"cache_read_per_million_usd"`
	CacheWritePerMillionUSD float64   `json:"cache_write_per_million_usd"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type Settings struct {
	AppName                 string `json:"app_name"`
	DefaultAnthropicVersion string `json:"default_anthropic_version"`
	RequestTimeoutSeconds   int    `json:"request_timeout_seconds"`
	MaxRetries              int    `json:"max_retries"`
	RoutingStrategy         string `json:"routing_strategy"`
	FailureThreshold        int    `json:"failure_threshold"`
	CooldownSeconds         int    `json:"cooldown_seconds"`
	LogRetentionDays        int    `json:"log_retention_days"`
	SessionTTLHours         int    `json:"session_ttl_hours"`
}

type SystemInfo struct {
	Version      string    `json:"version"`
	GoVersion    string    `json:"go_version"`
	DatabasePath string    `json:"database_path"`
	StartedAt    time.Time `json:"started_at"`
}
