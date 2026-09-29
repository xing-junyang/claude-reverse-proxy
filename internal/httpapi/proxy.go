package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/local/claude-relay/internal/store"
)

type anthropicUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
}

type proxyMeta struct {
	clientKey store.APIKey
	account   store.AccountSecret
	model     string
	endpoint  string
	streamed  bool
	started   time.Time
	status    int
	usage     anthropicUsage
	ttfb      int64
	errorText string
}

func (s *Server) proxyPreflight(w http.ResponseWriter, r *http.Request) {
	setProxyCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) proxyMessages(w http.ResponseWriter, r *http.Request) {
	setProxyCORS(w)
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAnthropicError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large", requestID(r.Context()))
			return
		}
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Unable to read request body", requestID(r.Context()))
		return
	}
	var envelope struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Request body must be valid JSON", requestID(r.Context()))
		return
	}
	if strings.TrimSpace(envelope.Model) == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "model is required", requestID(r.Context()))
		return
	}
	endpoint := "messages"
	streamed := envelope.Stream
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		endpoint = "count_tokens"
		streamed = false
	}
	s.handleProxy(w, r, body, envelope.Model, streamed, endpoint)
}

func (s *Server) proxyModels(w http.ResponseWriter, r *http.Request) {
	setProxyCORS(w)
	s.handleProxy(w, r, nil, "", false, "models")
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request, body []byte, model string, streamed bool, endpoint string) {
	started := time.Now()
	clientKey, ok := s.authenticateClient(w, r)
	if !ok {
		return
	}
	if model != "" && !store.IsModelAllowed(clientKey.AllowedModels, model) {
		s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusForbidden, "model not allowed")
		writeAnthropicError(w, http.StatusForbidden, "permission_error", "This API key is not allowed to use the requested model", requestID(r.Context()))
		return
	}
	if clientKey.MonthlyLimitUSD > 0 {
		spent, err := s.store.APIKeyMonthSpend(r.Context(), clientKey.ID, time.Now().UTC())
		if err != nil {
			s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusInternalServerError, "quota check failed")
			writeAnthropicError(w, http.StatusInternalServerError, "api_error", "Unable to verify API key quota", requestID(r.Context()))
			return
		}
		if spent >= clientKey.MonthlyLimitUSD {
			s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusTooManyRequests, "monthly spending limit reached")
			writeAnthropicError(w, http.StatusTooManyRequests, "rate_limit_error", "This API key has reached its monthly spending limit", requestID(r.Context()))
			return
		}
	}

	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusInternalServerError, "gateway settings unavailable")
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "Gateway settings are unavailable", requestID(r.Context()))
		return
	}
	candidates, err := s.store.CandidateAccounts(r.Context(), model)
	if err != nil {
		s.logger.Error("select upstream accounts", "error", err, "request_id", requestID(r.Context()))
		s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusServiceUnavailable, "account selection failed")
		writeAnthropicError(w, http.StatusServiceUnavailable, "overloaded_error", "No upstream account is currently available", requestID(r.Context()))
		return
	}
	if len(candidates) == 0 {
		s.recordGatewayFailure(r, clientKey, 0, model, endpoint, streamed, started, http.StatusServiceUnavailable, "no upstream account available")
		writeAnthropicError(w, http.StatusServiceUnavailable, "overloaded_error", "No upstream account is currently available", requestID(r.Context()))
		return
	}

	maxAttempts := min(len(candidates), settings.MaxRetries+1)
	excluded := make(map[int64]bool, maxAttempts)
	var lastNetworkError error
	var lastAccountID int64
	for attempt := 0; attempt < maxAttempts; attempt++ {
		account, release, found := s.selector.acquire(candidates, settings.RoutingStrategy, excluded)
		if !found {
			break
		}
		excluded[account.ID] = true
		lastAccountID = account.ID
		response, err := s.callUpstream(r, body, account, settings)
		if err != nil {
			release()
			lastNetworkError = err
			cooldown := failureCooldown(account.ConsecutiveFailures, settings.CooldownSeconds, settings.FailureThreshold)
			s.store.MarkAccountFailure(context.Background(), account.ID, err.Error(), cooldown, false)
			continue
		}

		if isRetryableStatus(response.StatusCode) && attempt+1 < maxAttempts {
			errorBody, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			response.Body.Close()
			release()
			message := upstreamErrorMessage(errorBody, response.Status)
			cooldown := retryCooldown(response, account.ConsecutiveFailures, settings)
			s.store.MarkAccountFailure(context.Background(), account.ID, message, cooldown, false)
			continue
		}

		meta := proxyMeta{
			clientKey: clientKey, account: account, model: model, endpoint: endpoint,
			streamed: streamed, started: started, status: response.StatusCode,
			ttfb: time.Since(started).Milliseconds(),
		}
		if response.StatusCode >= 200 && response.StatusCode < 400 {
			s.store.MarkAccountSuccess(context.Background(), account.ID)
		}
		if streamed && response.StatusCode >= 200 && response.StatusCode < 300 {
			s.proxyStream(w, r, response, &meta)
			release()
			s.recordProxy(meta, r)
			return
		}
		s.proxyBuffered(w, r, response, &meta)
		release()
		s.recordProxy(meta, r)
		return
	}

	message := "Unable to connect to an upstream account"
	if lastNetworkError != nil {
		message = "Upstream connection failed"
		s.logger.Warn("all upstream attempts failed", "error", lastNetworkError, "request_id", requestID(r.Context()))
	}
	s.recordGatewayFailure(r, clientKey, lastAccountID, model, endpoint, streamed, started, http.StatusBadGateway, message)
	writeAnthropicError(w, http.StatusBadGateway, "api_error", message, requestID(r.Context()))
}

func (s *Server) authenticateClient(w http.ResponseWriter, r *http.Request) (store.APIKey, bool) {
	secret := strings.TrimSpace(r.Header.Get("x-api-key"))
	if secret == "" {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			secret = strings.TrimSpace(authorization[7:])
		}
	}
	if secret == "" {
		writeAnthropicError(w, http.StatusUnauthorized, "authentication_error", "A valid x-api-key or Bearer token is required", requestID(r.Context()))
		return store.APIKey{}, false
	}
	key, err := s.store.AuthenticateAPIKey(r.Context(), secret)
	if err != nil {
		writeAnthropicError(w, http.StatusUnauthorized, "authentication_error", "Invalid API key", requestID(r.Context()))
		return store.APIKey{}, false
	}
	return key, true
}

func (s *Server) callUpstream(r *http.Request, body []byte, account store.AccountSecret, settings store.Settings) (*http.Response, error) {
	target, err := upstreamURL(account.BaseURL, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(settings.RequestTimeoutSeconds)*time.Second)
	// The response body owns the request context. Cancel it when Body.Close is called.
	reader := io.Reader(nil)
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, r.Method, target, reader)
	if err != nil {
		cancel()
		return nil, err
	}
	request.Header.Set("x-api-key", account.APIKey)
	request.Header.Set("anthropic-version", firstNonEmpty(r.Header.Get("anthropic-version"), settings.DefaultAnthropicVersion))
	request.Header.Set("User-Agent", "claude-relay/"+s.version)
	if value := r.Header.Get("anthropic-beta"); value != "" {
		request.Header.Set("anthropic-beta", value)
	}
	if value := r.Header.Get("Accept"); value != "" {
		request.Header.Set("Accept", value)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &cancelOnClose{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func (s *Server) proxyBuffered(w http.ResponseWriter, r *http.Request, response *http.Response, meta *proxyMeta) {
	defer response.Body.Close()
	copyUpstreamHeaders(w.Header(), response.Header)
	setProxyCORS(w)
	data, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		meta.status = http.StatusBadGateway
		meta.errorText = "failed reading upstream response: " + err.Error()
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "Failed to read upstream response", requestID(r.Context()))
		return
	}
	if meta.endpoint == "models" && response.StatusCode >= 200 && response.StatusCode < 300 {
		data = filterModelResponse(data, meta.clientKey.AllowedModels)
	}
	meta.usage = parseUsage(data)
	if meta.endpoint == "count_tokens" {
		meta.usage.OutputTokens = 0
	}
	if response.StatusCode >= 400 {
		meta.errorText = upstreamErrorMessage(data, response.Status)
		if response.StatusCode == http.StatusUnauthorized {
			s.store.MarkAccountFailure(context.Background(), meta.account.ID, meta.errorText, 0, true)
		} else if isRetryableStatus(response.StatusCode) {
			settings, _ := s.store.GetSettings(context.Background())
			s.store.MarkAccountFailure(context.Background(), meta.account.ID, meta.errorText, retryCooldown(response, meta.account.ConsecutiveFailures, settings), false)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(data)
}

func (s *Server) proxyStream(w http.ResponseWriter, r *http.Request, response *http.Response, meta *proxyMeta) {
	defer response.Body.Close()
	copyUpstreamHeaders(w.Header(), response.Header)
	setProxyCORS(w)
	w.WriteHeader(response.StatusCode)
	controller := http.NewResponseController(w)
	reader := bufio.NewReaderSize(response.Body, 64*1024)
	var eventData strings.Builder
	firstWrite := true
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if firstWrite {
				meta.ttfb = time.Since(meta.started).Milliseconds()
				firstWrite = false
			}
			if _, writeErr := io.WriteString(w, line); writeErr != nil {
				meta.status = 499
				meta.errorText = "client disconnected"
				return
			}
			_ = controller.Flush()
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				if eventData.Len() < 4<<20 {
					eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
				}
			} else if trimmed == "" && eventData.Len() > 0 {
				mergeUsage(&meta.usage, parseUsage([]byte(eventData.String())))
				eventData.Reset()
			}
		}
		if err != nil {
			if eventData.Len() > 0 {
				mergeUsage(&meta.usage, parseUsage([]byte(eventData.String())))
			}
			if !errors.Is(err, io.EOF) {
				meta.errorText = "upstream stream interrupted: " + err.Error()
			}
			return
		}
	}
}

func (s *Server) recordProxy(meta proxyMeta, r *http.Request) {
	cost := 0.0
	if meta.endpoint == "messages" && meta.status >= 200 && meta.status < 300 {
		cost = s.store.EstimateCost(context.Background(), meta.model, meta.usage.InputTokens,
			meta.usage.OutputTokens, meta.usage.CacheReadTokens, meta.usage.CacheCreationTokens)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.store.RecordUsage(ctx, store.UsageInsert{
		RequestID: requestID(r.Context()), APIKeyID: meta.clientKey.ID, AccountID: meta.account.ID,
		Model: meta.model, Endpoint: meta.endpoint, StatusCode: meta.status,
		InputTokens: meta.usage.InputTokens, OutputTokens: meta.usage.OutputTokens,
		CacheReadTokens: meta.usage.CacheReadTokens, CacheCreationTokens: meta.usage.CacheCreationTokens,
		CostUSD: cost, DurationMS: time.Since(meta.started).Milliseconds(), TimeToFirstByteMS: meta.ttfb,
		Streamed: meta.streamed, Error: meta.errorText, ClientIP: s.clientIP(r), CreatedAt: meta.started.UTC(),
	})
	if err != nil {
		s.logger.Error("record proxy usage", "error", err, "request_id", requestID(r.Context()))
	}
}

func (s *Server) recordGatewayFailure(r *http.Request, key store.APIKey, accountID int64, model, endpoint string, streamed bool, started time.Time, status int, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.RecordUsage(ctx, store.UsageInsert{
		RequestID: requestID(r.Context()), APIKeyID: key.ID, AccountID: accountID,
		Model: model, Endpoint: endpoint, StatusCode: status, DurationMS: time.Since(started).Milliseconds(),
		Streamed: streamed, Error: message, ClientIP: s.clientIP(r), CreatedAt: started.UTC(),
	}); err != nil {
		s.logger.Error("record rejected proxy request", "error", err, "request_id", requestID(r.Context()))
	}
}

func upstreamURL(baseURL, requestPath, rawQuery string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid upstream URL")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(requestPath, "/v1/") {
		requestPath = strings.TrimPrefix(requestPath, "/v1")
	}
	parsed.Path = basePath + "/" + strings.TrimLeft(requestPath, "/")
	parsed.RawPath = ""
	parsed.RawQuery = rawQuery
	parsed.Fragment = ""
	return parsed.String(), nil
}

func copyUpstreamHeaders(destination, source http.Header) {
	hopByHop := map[string]bool{
		"connection": true, "keep-alive": true, "proxy-authenticate": true,
		"proxy-authorization": true, "te": true, "trailer": true,
		"transfer-encoding": true, "upgrade": true, "set-cookie": true,
		"content-length": true, "access-control-allow-origin": true,
	}
	for key, values := range source {
		if hopByHop[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func setProxyCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-api-key, anthropic-version, anthropic-beta")
	w.Header().Set("Access-Control-Expose-Headers", "request-id, x-relay-request-id, retry-after")
	w.Header().Set("Vary", "Origin")
}

func writeAnthropicError(w http.ResponseWriter, status int, errorType, message, id string) {
	setProxyCORS(w)
	writeJSON(w, status, map[string]any{
		"type":       "error",
		"error":      map[string]string{"type": errorType, "message": message},
		"request_id": id,
	})
}

func parseUsage(data []byte) anthropicUsage {
	var payload struct {
		Usage       anthropicUsage `json:"usage"`
		InputTokens int64          `json:"input_tokens"`
		Message     struct {
			Usage anthropicUsage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return anthropicUsage{}
	}
	result := payload.Usage
	mergeUsage(&result, payload.Message.Usage)
	if result.InputTokens == 0 && payload.InputTokens > 0 {
		result.InputTokens = payload.InputTokens
	}
	return result
}

func mergeUsage(destination *anthropicUsage, value anthropicUsage) {
	destination.InputTokens = max(destination.InputTokens, value.InputTokens)
	destination.OutputTokens = max(destination.OutputTokens, value.OutputTokens)
	destination.CacheReadTokens = max(destination.CacheReadTokens, value.CacheReadTokens)
	destination.CacheCreationTokens = max(destination.CacheCreationTokens, value.CacheCreationTokens)
}

func filterModelResponse(data []byte, allowed []string) []byte {
	if len(allowed) == 0 {
		return data
	}
	var payload struct {
		Data    []json.RawMessage `json:"data"`
		HasMore bool              `json:"has_more"`
		FirstID string            `json:"first_id,omitempty"`
		LastID  string            `json:"last_id,omitempty"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return data
	}
	filtered := make([]json.RawMessage, 0, len(payload.Data))
	for _, raw := range payload.Data {
		var item struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &item) == nil && store.IsModelAllowed(allowed, item.ID) {
			filtered = append(filtered, raw)
		}
	}
	payload.Data = filtered
	encoded, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return encoded
}

func isRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests ||
		status == http.StatusInternalServerError || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout || status == 529
}

func retryCooldown(response *http.Response, failures int, settings store.Settings) time.Duration {
	if raw := response.Header.Get("Retry-After"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return min(time.Duration(seconds)*time.Second, 24*time.Hour)
		}
		if deadline, err := http.ParseTime(raw); err == nil && deadline.After(time.Now()) {
			return min(time.Until(deadline), 24*time.Hour)
		}
	}
	return failureCooldown(failures, settings.CooldownSeconds, settings.FailureThreshold)
}

func failureCooldown(failures, baseSeconds, threshold int) time.Duration {
	if baseSeconds <= 0 {
		baseSeconds = 60
	}
	exponent := max(failures+1-threshold, 0)
	if exponent > 6 {
		exponent = 6
	}
	return min(time.Duration(baseSeconds*(1<<exponent))*time.Second, time.Hour)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func proxyDebug(meta proxyMeta) string {
	return fmt.Sprintf("account=%d endpoint=%s status=%d", meta.account.ID, meta.endpoint, meta.status)
}
