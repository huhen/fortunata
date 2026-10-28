package main

import (
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(func(string) string { return "" })
	if err == nil {
		t.Fatal("ожидали ошибку: ADMIN_PASSWORD не задан")
	}
	if !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("не та ошибка: %v", err)
	}
	_ = cfg
}

func TestLoadConfigFull(t *testing.T) {
	env := map[string]string{
		"ADDR":           ":9000",
		"ADMIN_PASSWORD": "pass",
		"SESSION_SECRET": "s3cret",
		"COOKIE_SECURE":  "true",
		"DB_PATH":        "/tmp/x.db",
	}
	getenv := func(k string) string { return env[k] }
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.Password != "pass" || cfg.Secret != "s3cret" ||
		!cfg.CookieSecure || cfg.DBPath != "/tmp/x.db" || cfg.SecretRandom {
		t.Fatalf("неожиданный конфиг: %+v", cfg)
	}
}

func TestLoadConfigRandomSecret(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		return ""
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Secret) != 64 || !cfg.SecretRandom {
		t.Fatalf("ожидали случайный 64-символьный hex-секрет, got %q", cfg.Secret)
	}
	if cfg.Addr != ":8080" || cfg.DBPath != "fortunata.db" || cfg.CookieSecure {
		t.Fatalf("дефолты сломались: %+v", cfg)
	}
}

func TestLoadConfigLLMPair(t *testing.T) {
	env := map[string]string{
		"ADMIN_PASSWORD": "pass",
		"LLM_BASE_URL":   "http://192.168.1.128:18020",
		"LLM_MODEL":      "qwen3.8-27b",
		"LLM_API_KEY":    "secret",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMBaseURL != "http://192.168.1.128:18020" || cfg.LLMModel != "qwen3.8-27b" || cfg.LLMAPIKey != "secret" {
		t.Fatalf("LLM-конфиг: %+v", cfg)
	}
}

func TestLoadConfigLLMHalfConfigured(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		if k == "LLM_BASE_URL" {
			return "http://x"
		}
		return ""
	}
	_, err := loadConfig(getenv)
	if err == nil || !strings.Contains(err.Error(), "LLM_BASE_URL") {
		t.Fatalf("ожидали ошибку пары LLM_*, got %v", err)
	}
}

func TestLoadConfigLLMOffByDefault(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		return ""
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMBaseURL != "" || cfg.LLMModel != "" || cfg.LLMAPIKey != "" {
		t.Fatalf("LLM должен быть выключен: %+v", cfg)
	}
}

func TestLoadConfigInvalidCookieSecure(t *testing.T) {
	getenv := func(k string) string {
		if k == "COOKIE_SECURE" {
			return "yes"
		}
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		return ""
	}
	_, err := loadConfig(getenv)
	if err == nil || !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("ожидали ошибку COOKIE_SECURE, got %v", err)
	}
}

func TestLoadConfigSyncDefaults(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		return ""
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyncAtHour != 15 || cfg.SyncAtMin != 0 || !cfg.SyncEnabled {
		t.Fatalf("дефолты SYNC_*: %+v", cfg)
	}
}

func TestLoadConfigSyncCustom(t *testing.T) {
	env := map[string]string{
		"ADMIN_PASSWORD": "pass",
		"SYNC_AT":        "07:30",
		"SYNC_ENABLED":   "false",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyncAtHour != 7 || cfg.SyncAtMin != 30 || cfg.SyncEnabled {
		t.Fatalf("свой SYNC_*: %+v", cfg)
	}
}

func TestLoadConfigSyncBoundary(t *testing.T) {
	cases := []struct {
		at   string
		hour int
		min  int
	}{
		{"00:00", 0, 0},
		{"23:59", 23, 59},
	}
	for _, tc := range cases {
		t.Run(tc.at, func(t *testing.T) {
			env := map[string]string{"ADMIN_PASSWORD": "pass", "SYNC_AT": tc.at}
			cfg, err := loadConfig(func(k string) string { return env[k] })
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SyncAtHour != tc.hour || cfg.SyncAtMin != tc.min {
				t.Fatalf("SYNC_AT=%q: ожидали %d:%02d, got %d:%02d", tc.at, tc.hour, tc.min, cfg.SyncAtHour, cfg.SyncAtMin)
			}
		})
	}
}

func TestLoadConfigSyncBadAt(t *testing.T) {
	// «7:30» и «15:00:00» отвергаем: формат строго HH:MM, две цифры каждого поля.
	for _, at := range []string{"25:00", "7:30", "15:60", "abc", "15:00:00", "-1:00", "ab:cd"} {
		t.Run(at, func(t *testing.T) {
			env := map[string]string{"ADMIN_PASSWORD": "pass", "SYNC_AT": at}
			_, err := loadConfig(func(k string) string { return env[k] })
			if err == nil || !strings.Contains(err.Error(), "SYNC_AT") {
				t.Fatalf("SYNC_AT=%q: ожидали ошибку, got %v", at, err)
			}
		})
	}
}

func TestLoadConfigSyncBadEnabled(t *testing.T) {
	env := map[string]string{"ADMIN_PASSWORD": "pass", "SYNC_ENABLED": "maybe"}
	_, err := loadConfig(func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "SYNC_ENABLED") {
		t.Fatalf("ожидали ошибку SYNC_ENABLED, got %v", err)
	}
}
