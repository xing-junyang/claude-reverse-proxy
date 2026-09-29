package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/local/claude-relay/internal/store"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if allowed, wait := s.limiter.allow(ip); !allowed {
		w.Header().Set("Retry-After", formatRetryAfter(wait))
		writeProblem(w, http.StatusTooManyRequests, "too_many_attempts", "登录失败次数过多，请稍后再试")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请输入用户名和密码")
		return
	}
	user, err := s.store.AuthenticateAdmin(r.Context(), strings.TrimSpace(input.Username), input.Password)
	if err != nil {
		s.limiter.failed(ip)
		time.Sleep(250 * time.Millisecond)
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "登录失败")
		return
	}
	token, expires, err := s.store.CreateSession(r.Context(), user.ID, time.Duration(settings.SessionTTLHours)*time.Hour)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "登录失败")
		return
	}
	s.limiter.success(ip)
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: token, Path: "/", Expires: expires,
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	s.store.AddAudit(r.Context(), user.ID, "login", "session", "", "管理员登录", ip)
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "expires_at": expires})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookieName); err == nil && cookie.Value != "" {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	clearSessionCookie(w, s.cfg.CookieSecure)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.GetSettings(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"user":     adminUser(r.Context()),
		"app_name": settings.AppName,
		"version":  s.version,
	})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "请求内容格式错误")
		return
	}
	if len(input.NewPassword) < 12 || len(input.NewPassword) > 256 {
		writeProblem(w, http.StatusBadRequest, "weak_password", "新密码至少需要 12 个字符")
		return
	}
	user := adminUser(r.Context())
	if err := s.store.ChangePassword(r.Context(), user.ID, input.CurrentPassword, input.NewPassword); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusBadRequest, "invalid_password", "当前密码错误")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "internal_error", "修改密码失败")
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), user.ID)
	clearSessionCookie(w, s.cfg.CookieSecure)
	s.store.AddAudit(r.Context(), user.ID, "password.change", "user", "", "管理员修改密码", s.clientIP(r))
	writeJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func formatRetryAfter(wait time.Duration) string {
	seconds := int(wait.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
