package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr          string
	DataDir             string
	DatabasePath        string
	MasterKey           string
	AdminUsername       string
	AdminPassword       string
	CookieSecure        bool
	TrustedProxies      []string
	BootstrapAPIKey     string
	BootstrapBaseURL    string
	MaxRequestBodyBytes int64
}

func Load() (Config, error) {
	dataDir := env("DATA_DIR", "./data")
	maxBodyMB := envInt("MAX_REQUEST_BODY_MB", 32)
	if maxBodyMB < 1 || maxBodyMB > 256 {
		return Config{}, fmt.Errorf("MAX_REQUEST_BODY_MB must be between 1 and 256")
	}

	cfg := Config{
		ListenAddr:          env("LISTEN_ADDR", ":8080"),
		DataDir:             dataDir,
		DatabasePath:        filepath.Join(dataDir, "claude-relay.db"),
		MasterKey:           strings.TrimSpace(os.Getenv("MASTER_KEY")),
		AdminUsername:       env("ADMIN_USERNAME", "admin"),
		AdminPassword:       os.Getenv("ADMIN_PASSWORD"),
		CookieSecure:        envBool("COOKIE_SECURE", false),
		TrustedProxies:      splitCSV(os.Getenv("TRUSTED_PROXIES")),
		BootstrapAPIKey:     strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		BootstrapBaseURL:    strings.TrimRight(env("ANTHROPIC_BASE_URL", "https://api.anthropic.com"), "/"),
		MaxRequestBodyBytes: int64(maxBodyMB) * 1024 * 1024,
	}

	if cfg.AdminPassword == "" {
		cfg.AdminPassword = "change-this-password"
	}
	if strings.TrimSpace(cfg.AdminUsername) == "" {
		return Config{}, fmt.Errorf("ADMIN_USERNAME cannot be empty")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
