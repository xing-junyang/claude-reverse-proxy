package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/local/claude-relay/internal/config"
	"github.com/local/claude-relay/internal/store"
	webassets "github.com/local/claude-relay/internal/web"
)

type Server struct {
	cfg       config.Config
	store     *store.Store
	logger    *slog.Logger
	client    *http.Client
	startedAt time.Time
	version   string
	selector  *accountSelector
	limiter   *loginLimiter
	static    fs.FS
}

func New(cfg config.Config, data *store.Store, logger *slog.Logger, version string) (*Server, error) {
	static, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded frontend: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	return &Server{
		cfg: cfg, store: data, logger: logger, version: version,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		startedAt: time.Now().UTC(),
		selector:  newAccountSelector(),
		limiter:   newLoginLimiter(),
		static:    static,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/public/config", s.publicConfig)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.Handle("GET /api/auth/me", s.requireAdmin(http.HandlerFunc(s.me)))
	mux.Handle("PUT /api/auth/password", s.requireAdmin(http.HandlerFunc(s.changePassword)))

	mux.Handle("GET /api/dashboard", s.requireAdmin(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /api/accounts", s.requireAdmin(http.HandlerFunc(s.listAccounts)))
	mux.Handle("POST /api/accounts", s.requireAdmin(http.HandlerFunc(s.createAccount)))
	mux.Handle("PUT /api/accounts/{id}", s.requireAdmin(http.HandlerFunc(s.updateAccount)))
	mux.Handle("DELETE /api/accounts/{id}", s.requireAdmin(http.HandlerFunc(s.deleteAccount)))
	mux.Handle("POST /api/accounts/{id}/toggle", s.requireAdmin(http.HandlerFunc(s.toggleAccount)))
	mux.Handle("POST /api/accounts/{id}/test", s.requireAdmin(http.HandlerFunc(s.testAccount)))
	mux.Handle("POST /api/accounts/{id}/reset", s.requireAdmin(http.HandlerFunc(s.resetAccount)))

	mux.Handle("GET /api/keys", s.requireAdmin(http.HandlerFunc(s.listKeys)))
	mux.Handle("POST /api/keys", s.requireAdmin(http.HandlerFunc(s.createKey)))
	mux.Handle("PUT /api/keys/{id}", s.requireAdmin(http.HandlerFunc(s.updateKey)))
	mux.Handle("DELETE /api/keys/{id}", s.requireAdmin(http.HandlerFunc(s.deleteKey)))

	mux.Handle("GET /api/usage", s.requireAdmin(http.HandlerFunc(s.listUsage)))
	mux.Handle("GET /api/usage/summary", s.requireAdmin(http.HandlerFunc(s.usageSummary)))
	mux.Handle("GET /api/usage/models", s.requireAdmin(http.HandlerFunc(s.usageModels)))
	mux.Handle("DELETE /api/usage", s.requireAdmin(http.HandlerFunc(s.purgeUsage)))

	mux.Handle("GET /api/settings", s.requireAdmin(http.HandlerFunc(s.getSettings)))
	mux.Handle("PUT /api/settings", s.requireAdmin(http.HandlerFunc(s.updateSettings)))
	mux.Handle("GET /api/prices", s.requireAdmin(http.HandlerFunc(s.listPrices)))
	mux.Handle("PUT /api/prices/{match}", s.requireAdmin(http.HandlerFunc(s.upsertPrice)))
	mux.Handle("DELETE /api/prices/{match}", s.requireAdmin(http.HandlerFunc(s.deletePrice)))
	mux.Handle("GET /api/system", s.requireAdmin(http.HandlerFunc(s.systemInfo)))

	mux.HandleFunc("OPTIONS /v1/{path...}", s.proxyPreflight)
	mux.HandleFunc("POST /v1/messages", s.proxyMessages)
	mux.HandleFunc("POST /v1/messages/count_tokens", s.proxyMessages)
	mux.HandleFunc("GET /v1/models", s.proxyModels)
	mux.HandleFunc("GET /v1/models/{model}", s.proxyModels)

	mux.HandleFunc("/", s.frontend)

	return s.recoverMiddleware(s.requestMiddleware(s.securityMiddleware(mux)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy", "service": "claude-relay", "error": "database unavailable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "service": "claude-relay", "version": s.version,
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取服务配置失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_name": settings.AppName,
		"version":  s.version,
	})
}

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
		writeProblem(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
		return
	}

	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	if info, err := fs.Stat(s.static, requested); err == nil && !info.IsDir() {
		if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		http.FileServer(http.FS(s.static)).ServeHTTP(w, r)
		return
	}
	index, err := fs.ReadFile(s.static, "index.html")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "frontend_not_built",
			"message": "Frontend assets are not available. Run npm run build in frontend/.",
		})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DatabaseStats(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "internal_error", "读取系统状态失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.version, "go_version": runtime.Version(), "started_at": s.startedAt,
		"database_path": s.cfg.DatabasePath, "stats": stats,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nilResponseWriter{}, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) == nil {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// nilResponseWriter is only used to let MaxBytesReader cap admin JSON bodies.
// It is never written to for valid requests.
type nilResponseWriter struct{}

func (nilResponseWriter) Header() http.Header       { return make(http.Header) }
func (nilResponseWriter) Write([]byte) (int, error) { return 0, http.ErrBodyNotAllowed }
func (nilResponseWriter) WriteHeader(int)           {}

func pathID(r *http.Request) (int64, error) {
	var id int64
	_, err := fmt.Sscan(r.PathValue("id"), &id)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
