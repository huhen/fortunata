// Пакет api: JSON API приложения.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"fortunata/internal/auth"
	"fortunata/internal/llm"
	"fortunata/internal/scheduler"
	"fortunata/internal/store"
)

// maxBodyBytes — лимит тела запроса: на порядки больше реальных запросов.
const maxBodyBytes = 64 << 10

// WriteTimeout — таймаут записи HTTP-сервера: cmd/server ставит его на
// http.Server. Худший случай — proposeBudget в internal/llm (125 с, запас
// 5 с), иначе ответ AI-генерации умрёт при записи. Таймаут скачивания
// архива обязан быть меньше него (см. newHandler).
const WriteTimeout = 130 * time.Second

// defaultArchiveURL — страница архива результатов на timelottery.ru.
const defaultArchiveURL = "https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/"

// SyncConfig — настройки автосинхронизации из env (см. cmd/server).
type SyncConfig struct {
	Hour, Min int
	Enabled   bool
}

// FormatSchedule рендерит время расписания («15:00 UTC») — одно место для
// лога cmd/server и поля schedule в статусе.
func FormatSchedule(hour, min int) string {
	return fmt.Sprintf("%02d:%02d UTC", hour, min)
}

type Handler struct {
	st            *store.Store
	auth          *auth.Manager
	cookieSecure  bool
	archiveURL    string               // источник синхронизации; переопределяется в тестах
	archiveClient *http.Client         // клиент скачивания архива; подменяется в тестах
	llm           *llm.Client          // nil — AI-генерация не настроена
	aiSlots       chan struct{}        // слот параллельного AI-запроса; ёмкость 1 — домашний LLM обрабатывает по одному
	syncAt        string               // «15:00 UTC» — для статуса
	sched         *scheduler.Scheduler // nil — автосинхронизация выключена
	mu            sync.Mutex           // защищает lastSync
	lastSync      *syncStatus
}

// newHandler собирает Handler; маршруты регистрирует register.
// Таймаут клиента должен оставаться меньше WriteTimeout, иначе вставки
// закоммитятся, а ответ до клиента не дойдёт (инвариант проверяет
// TestSyncArchiveClientTimeoutBelowWriteTimeout).
func newHandler(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client, syncCfg SyncConfig) *Handler {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	h := &Handler{
		st:            st,
		auth:          auth.New(password, secret),
		cookieSecure:  cookieSecure,
		archiveURL:    archiveURL,
		archiveClient: &http.Client{Timeout: 10 * time.Second},
		llm:           llmClient,
		aiSlots:       make(chan struct{}, 1),
		syncAt:        FormatSchedule(syncCfg.Hour, syncCfg.Min),
	}
	if syncCfg.Enabled {
		h.sched = scheduler.New(syncCfg.Hour, syncCfg.Min, func(ctx context.Context) error {
			_, err := h.runSyncAndRecord(ctx)
			return err
		})
	}
	return h
}

// New собирает все /api-маршруты; возвращает mux и планировщик автосинки
// (nil — выключена; main запускает его: go sched.Run(ctx)). Пустой
// archiveURL заменяется на defaultArchiveURL; llmClient == nil выключает
// AI-генерацию.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client, syncCfg SyncConfig) (*http.ServeMux, *scheduler.Scheduler) {
	h := newHandler(st, password, secret, cookieSecure, archiveURL, llmClient, syncCfg)
	mux := http.NewServeMux()
	h.register(mux)
	return mux, h.sched
}

// register регистрирует все /api-маршруты на mux.
func (h *Handler) register(mux *http.ServeMux) {
	mux.Handle("POST /api/login", requireJSON(h.login))
	mux.Handle("POST /api/logout", requireJSON(h.logout))
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("GET /api/draws", h.listDraws)
	mux.HandleFunc("GET /api/stats", h.stats)
	mux.HandleFunc("GET /api/version", h.getVersion)
	mux.Handle("POST /api/draws", h.session(requireJSON(h.createDraw)))
	mux.Handle("PUT /api/draws/{no}", h.session(requireJSON(h.updateDraw)))
	mux.Handle("DELETE /api/draws/{no}", h.session(h.deleteDraw))
	mux.Handle("POST /api/sync", h.session(requireJSON(h.syncDraws)))
	mux.HandleFunc("GET /api/sync-status", h.session(h.syncStatusGet))
	mux.Handle("POST /api/generate", requireJSON(h.generate))
	mux.Handle("POST /api/generate/ai", requireJSON(h.generateAI))
}

// requireJSON защищает мутации: CSRF-запрос из формы со стороннего сайта
// не сможет поставить Content-Type: application/json.
func requireJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			errorJSON(w, http.StatusUnsupportedMediaType, "Ожидается Content-Type: application/json")
			return
		}
		next(w, r)
	}
}

// session пропускает дальше только запросы с валидной cookie-сессией.
func (h *Handler) session(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName)
		if err != nil || !h.auth.Verify(c.Value) {
			errorJSON(w, http.StatusUnauthorized, "Требуется вход")
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON декодирует тело запроса; при ошибке сам пишет 400 и возвращает false.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		errorJSON(w, http.StatusBadRequest, "Некорректный JSON")
		return false
	}
	return true
}
