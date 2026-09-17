// Команда server: конфигурация из переменных окружения и HTTP-сервер.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"fortunata/internal/api"
	"fortunata/internal/store"
	"fortunata/web"
)

// Config — все настройки приложения, читаемые из окружения.
type Config struct {
	Addr         string // ADDR, по умолчанию ":8080"
	Password     string // ADMIN_PASSWORD, обязателен
	Secret       string // SESSION_SECRET; если пуст — случайный
	SecretRandom bool   // true, если SECRET сгенерирован при старте
	CookieSecure bool   // COOKIE_SECURE, по умолчанию false (за Traefik ставят true)
	DBPath       string // DB_PATH, по умолчанию "fortunata.db"
}

func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:   getenv("ADDR"),
		DBPath: getenv("DB_PATH"),
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "fortunata.db"
	}
	cfg.Password = getenv("ADMIN_PASSWORD")
	if cfg.Password == "" {
		return Config{}, errors.New("ADMIN_PASSWORD не задан")
	}
	cfg.Secret = getenv("SESSION_SECRET")
	if cfg.Secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return Config{}, fmt.Errorf("генерация SESSION_SECRET: %w", err)
		}
		cfg.Secret = hex.EncodeToString(buf)
		cfg.SecretRandom = true
	}
	secure := getenv("COOKIE_SECURE")
	if secure == "" {
		secure = "false"
	}
	b, err := strconv.ParseBool(secure)
	if err != nil {
		return Config{}, fmt.Errorf("COOKIE_SECURE: %w", err)
	}
	cfg.CookieSecure = b
	return cfg, nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	if cfg.SecretRandom {
		logger.Warn("SESSION_SECRET не задан — сгенерирован случайный, сессии слетят при перезапуске")
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("не удалось открыть базу", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	mux := api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure, "")

	// Статика: / — index.html, /admin — админка, остальное — файлы из web/.
	mux.Handle("GET /", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		b, err := web.Files.ReadFile("admin.html")
		if err != nil {
			http.Error(w, "admin.html не найден", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		logger.Info("слушаю", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http-сервер", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "err", err)
	}
}

// securityHeaders добавляет базовые защитные заголовки ко всем ответам.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		// Инлайн-скриптов и стилей в фронтенде нет — строгий CSP безопасен.
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}
