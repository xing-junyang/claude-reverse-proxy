export type AdminUser = {
  id: number
  username: string
}

export type Account = {
  id: number
  name: string
  base_url: string
  key_hint: string
  enabled: boolean
  priority: number
  weight: number
  allowed_models: string[]
  monthly_limit_usd: number
  notes: string
  status: 'unknown' | 'healthy' | 'degraded' | 'cooldown' | 'disabled'
  cooldown_until?: string
  consecutive_failures: number
  request_count: number
  input_tokens: number
  output_tokens: number
  spent_usd: number
  last_used_at?: string
  last_success_at?: string
  last_error?: string
  created_at: string
  updated_at: string
}

export type AccountInput = {
  name: string
  base_url: string
  api_key: string
  enabled: boolean
  priority: number
  weight: number
  allowed_models: string[]
  monthly_limit_usd: number
  notes: string
}

export type APIKey = {
  id: number
  name: string
  prefix: string
  last_four: string
  enabled: boolean
  monthly_limit_usd: number
  allowed_models: string[]
  expires_at?: string
  request_count: number
  input_tokens: number
  output_tokens: number
  spent_usd: number
  last_used_at?: string
  created_at: string
  updated_at: string
}

export type APIKeyInput = {
  name: string
  enabled: boolean
  monthly_limit_usd: number
  allowed_models: string[]
  expires_at: string | null
}

export type APIKeyCreated = APIKey & { secret: string }

export type UsageLog = {
  id: number
  request_id: string
  api_key_id: number
  api_key_name: string
  account_id: number
  account_name: string
  model: string
  endpoint: string
  status_code: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  cost_usd: number
  duration_ms: number
  time_to_first_byte_ms: number
  streamed: boolean
  error?: string
  client_ip: string
  created_at: string
}

export type MetricDelta = {
  label: string
  value: number
  previous: number
  change_pct: number
  has_previous: boolean
}

export type SeriesPoint = {
  bucket: string
  requests: number
  input_tokens: number
  output_tokens: number
  cost_usd: number
  errors: number
}

export type ModelBreakdown = {
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cost_usd: number
}

export type DashboardData = {
  period_hours: number
  requests: MetricDelta
  tokens: MetricDelta
  cost: MetricDelta
  success_rate: MetricDelta
  average_latency_ms: number
  active_accounts: number
  total_accounts: number
  active_keys: number
  series: SeriesPoint[]
  models: ModelBreakdown[]
  recent: UsageLog[]
}

export type Settings = {
  app_name: string
  default_anthropic_version: string
  request_timeout_seconds: number
  max_retries: number
  routing_strategy: 'least_recently_used' | 'priority' | 'weighted'
  failure_threshold: number
  cooldown_seconds: number
  log_retention_days: number
  session_ttl_hours: number
}

export type ModelPrice = {
  match: string
  input_per_million_usd: number
  output_per_million_usd: number
  cache_read_per_million_usd: number
  cache_write_per_million_usd: number
  updated_at?: string
}

export type SystemInfo = {
  version: string
  go_version: string
  started_at: string
  database_path: string
  stats: Record<string, number>
}

export type ToastMessage = {
  id: number
  type: 'success' | 'error' | 'info'
  title: string
  description?: string
}
