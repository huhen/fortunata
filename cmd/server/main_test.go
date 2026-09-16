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
	if cfg.Addr != ":8080" || cfg.DBPath != "generator.db" || cfg.CookieSecure {
		t.Fatalf("дефолты сломались: %+v", cfg)
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
