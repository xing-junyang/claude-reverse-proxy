package httpapi

import (
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

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	hours := parseQueryInt(r, "hours", 24)
	data, err := s.store.Dashboard(r.Context(), hours)
	if err != nil {
		s.logger.Error("load dashboard", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取概览数据失败")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.store.ListAccounts(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取账号失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": accounts})
}

func validateAccountInput(input store.AccountInput, creating bool) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		return errors.New("账号名称长度应为 1–80 个字符")
	}
	if creating && strings.TrimSpace(input.APIKey) == "" {
		return errors.New("API Key 不能为空")
	}
	if len(input.APIKey) > 1024 {
		return errors.New("API Key 过长")
	}
	parsed, err := url.Parse(strings.TrimSpace(input.BaseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("Base URL 必须是有效的 HTTP(S) 地址")
	}
	if input.Weight < 0 || input.Weight > 1000 || input.Priority < -100 || input.Priority > 100 {
		return errors.New("权重或优先级超出允许范围")
	}
	if input.MonthlyLimitUSD < 0 || input.MonthlyLimitUSD > 10_000_000 {
		return errors.New("月度限额超出允许范围")
	}
	if len(input.AllowedModels) > 100 {
		return errors.New("模型规则过多")
	}
	return nil
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var input store.AccountInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if input.Weight == 0 {
		input.Weight = 1
	}
	if err := validateAccountInput(input, true); err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	account, err := s.store.CreateAccount(r.Context(), input)
	if err != nil {
		s.logger.Error("create account", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "添加账号失败")
		return
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "account.create", "account", strconv.FormatInt(account.ID, 10), account.Name, s.clientIP(r))
	writeJSON(w, http.StatusCreated, account)
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "账号 ID 无效")
		return
	}
	var input store.AccountInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if input.Weight == 0 {
		input.Weight = 1
	}
	if err := validateAccountInput(input, false); err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	account, err := s.store.UpdateAccount(r.Context(), id, input)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "账号不存在")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "更新账号失败")
		return
	}
	if strings.TrimSpace(input.APIKey) != "" {
		_ = s.store.ResetAccountHealth(r.Context(), id)
		account, _ = s.store.GetAccount(r.Context(), id)
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "account.update", "account", strconv.FormatInt(id, 10), account.Name, s.clientIP(r))
	writeJSON(w, http.StatusOK, account)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "账号 ID 无效")
		return
	}
	account, _ := s.store.GetAccount(r.Context(), id)
	if err := s.store.DeleteAccount(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "账号不存在")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "删除账号失败")
		return
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "account.delete", "account", strconv.FormatInt(id, 10), account.Name, s.clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) toggleAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "账号 ID 无效")
		return
	}
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if err := s.store.SetAccountEnabled(r.Context(), id, input.Enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "账号不存在")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "更新账号状态失败")
		return
	}
	account, _ := s.store.GetAccount(r.Context(), id)
	writeJSON(w, http.StatusOK, account)
}

func (s *Server) resetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "账号 ID 无效")
		return
	}
	if err := s.store.ResetAccountHealth(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "账号不存在")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "重置账号状态失败")
		return
	}
	account, _ := s.store.GetAccount(r.Context(), id)
	writeJSON(w, http.StatusOK, account)
}

func (s *Server) testAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "账号 ID 无效")
		return
	}
	account, err := s.store.GetAccountSecret(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "账号不存在")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取账号失败")
		return
	}
	settings, _ := s.store.GetSettings(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	upstreamURL, err := upstreamURL(account.BaseURL, "/v1/models", "")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_upstream", "上游地址无效")
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	req.Header.Set("x-api-key", account.APIKey)
	req.Header.Set("anthropic-version", settings.DefaultAnthropicVersion)
	req.Header.Set("User-Agent", "claude-relay/"+s.version)
	started := time.Now()
	response, err := s.client.Do(req)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		s.store.MarkAccountFailure(r.Context(), id, err.Error(), time.Minute, false)
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "latency_ms": latency, "message": "无法连接上游：" + err.Error()})
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := upstreamErrorMessage(body, response.Status)
		s.store.MarkAccountFailure(r.Context(), id, message, time.Minute, response.StatusCode == http.StatusUnauthorized)
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "status_code": response.StatusCode, "latency_ms": latency, "message": message})
		return
	}
	s.store.MarkAccountSuccess(r.Context(), id)
	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &payload)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status_code": response.StatusCode, "latency_ms": latency, "models": len(payload.Data), "message": "连接正常"})
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取 API Key 失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": keys})
}

func validateKeyInput(input store.APIKeyInput) error {
	if name := strings.TrimSpace(input.Name); name == "" || len(name) > 80 {
		return errors.New("名称长度应为 1–80 个字符")
	}
	if input.MonthlyLimitUSD < 0 || input.MonthlyLimitUSD > 10_000_000 {
		return errors.New("月度限额超出允许范围")
	}
	if input.ExpiresAt != nil && input.ExpiresAt.Before(time.Now().Add(-time.Minute)) {
		return errors.New("过期时间不能早于当前时间")
	}
	return nil
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var input store.APIKeyInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if err := validateKeyInput(input); err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	created, err := s.store.CreateAPIKey(r.Context(), input)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "创建 API Key 失败")
		return
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "key.create", "api_key", strconv.FormatInt(created.ID, 10), created.Name, s.clientIP(r))
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "API Key ID 无效")
		return
	}
	var input store.APIKeyInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if err := validateKeyInput(input); err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	key, err := s.store.UpdateAPIKey(r.Context(), id, input)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "API Key 不存在")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "更新 API Key 失败")
		return
	}
	writeJSON(w, http.StatusOK, key)
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "API Key ID 无效")
		return
	}
	if err := s.store.DeleteAPIKey(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "API Key 不存在")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "删除 API Key 失败")
		return
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "key.delete", "api_key", strconv.FormatInt(id, 10), "", s.clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func usageFilterFromRequest(r *http.Request) store.UsageFilter {
	query := r.URL.Query()
	filter := store.UsageFilter{
		APIKeyID: parseInt64(query.Get("key_id")), AccountID: parseInt64(query.Get("account_id")),
		Model: query.Get("model"), Status: query.Get("status"), Search: query.Get("search"),
		Limit: parseQueryInt(r, "limit", 50), Offset: parseQueryInt(r, "offset", 0),
	}
	if raw := query.Get("from"); raw != "" {
		filter.From, _ = time.Parse(time.RFC3339, raw)
	}
	if raw := query.Get("to"); raw != "" {
		filter.To, _ = time.Parse(time.RFC3339, raw)
	}
	return filter
}

func (s *Server) listUsage(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListUsage(r.Context(), usageFilterFromRequest(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取调用记录失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) usageSummary(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.UsageSummary(r.Context(), usageFilterFromRequest(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取用量统计失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) usageModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.store.DistinctModels(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取模型列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": models})
}

func (s *Server) purgeUsage(w http.ResponseWriter, r *http.Request) {
	days := parseQueryInt(r, "older_than_days", 30)
	if days < 1 || days > 3650 {
		writeProblem(w, http.StatusBadRequest, "validation_error", "清理天数超出允许范围")
		return
	}
	deleted, err := s.store.PurgeUsageBefore(r.Context(), time.Now().AddDate(0, 0, -days))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "清理调用记录失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取设置失败")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var input store.Settings
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	settings, err := s.store.UpdateSettings(r.Context(), input)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	user := adminUser(r.Context())
	s.store.AddAudit(r.Context(), user.ID, "settings.update", "settings", "", "更新网关设置", s.clientIP(r))
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) listPrices(w http.ResponseWriter, r *http.Request) {
	prices, err := s.store.ListModelPrices(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取计价规则失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": prices})
}

func (s *Server) upsertPrice(w http.ResponseWriter, r *http.Request) {
	match := strings.TrimSpace(r.PathValue("match"))
	var price store.ModelPrice
	if err := decodeJSON(r, &price); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	price.Match = match
	updated, err := s.store.UpsertModelPrice(r.Context(), price)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deletePrice(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteModelPrice(r.Context(), r.PathValue("match")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "not_found", "计价规则不存在")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "删除计价规则失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseQueryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func upstreamErrorMessage(body []byte, fallback string) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
		return truncatePublic(payload.Error.Message, 300)
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return truncatePublic(text, 300)
	}
	return fallback
}

func truncatePublic(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func jsonDetail(value any) string {
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("%s", encoded)
}
