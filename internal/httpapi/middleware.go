package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/local/claude-relay/internal/cryptox"
	"github.com/local/claude-relay/internal/store"
)

type contextKey string

const (
	contextUserKey      contextKey = "admin_user"
	contextRequestIDKey contextKey = "request_id"
	adminCookieName                = "claude_relay_session"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID, err := cryptox.RandomToken(12)
		if err != nil {
			requestID = fmt.Sprintf("fallback-%d", started.UnixNano())
		}
		requestID = "crp_" + requestID
		ctx := context.WithValue(r.Context(), contextRequestIDKey, requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Relay-Request-Id", requestID)

		if strings.HasPrefix(r.URL.Path, "/api/") && isMutation(r.Method) && !sameOrigin(r) {
			writeProblem(w, http.StatusForbidden, "origin_rejected", "请求来源校验失败")
			return
		}

		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if r.URL.Path != "/healthz" {
			s.logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(started).Milliseconds(),
				"bytes", recorder.bytes,
				"request_id", requestID,
				"client_ip", s.clientIP(r),
			)
		}
	})
}

func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if s.cfg.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic recovered", "panic", recovered, "path", r.URL.Path, "request_id", requestID(r.Context()))
				writeProblem(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(adminCookieName)
		if err != nil || cookie.Value == "" {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		user, err := s.store.SessionUser(r.Context(), cookie.Value)
		if err != nil {
			clearSessionCookie(w, s.cfg.CookieSecure)
			writeProblem(w, http.StatusUnauthorized, "session_expired", "登录状态已过期")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextUserKey, user)))
	})
}

func adminUser(ctx context.Context) store.AdminUser {
	user, _ := ctx.Value(contextUserKey).(store.AdminUser)
	return user
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(contextRequestIDKey).(string)
	return value
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.isTrustedProxy(host) {
		return host
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if candidate := strings.TrimSpace(parts[0]); net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	if candidate := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(candidate) != nil {
		return candidate
	}
	return host
}

func (s *Server) isTrustedProxy(ip string) bool {
	parsedIP := net.ParseIP(ip)
	for _, configured := range s.cfg.TrustedProxies {
		if configured == ip {
			return true
		}
		if _, cidr, err := net.ParseCIDR(configured); err == nil && parsedIP != nil && cidr.Contains(parsedIP) {
			return true
		}
	}
	return false
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

type loginAttempt struct {
	count       int
	windowStart time.Time
	blockedTill time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt)}
}

func (l *loginLimiter) allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	attempt := l.attempts[ip]
	if attempt.blockedTill.After(now) {
		return false, time.Until(attempt.blockedTill)
	}
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) > 10*time.Minute {
		l.attempts[ip] = loginAttempt{windowStart: now}
	}
	return true, 0
}

func (l *loginLimiter) failed(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	attempt := l.attempts[ip]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) > 10*time.Minute {
		attempt = loginAttempt{windowStart: now}
	}
	attempt.count++
	if attempt.count >= 5 {
		attempt.blockedTill = now.Add(15 * time.Minute)
	}
	l.attempts[ip] = attempt
}

func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	delete(l.attempts, ip)
	l.mu.Unlock()
}
