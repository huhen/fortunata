// Пакет api: JSON API приложения.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"fortunata/internal/auth"
	"fortunata/internal/llm"
	"fortunata/internal/store"
)

// maxBodyBytes — лимит тела запроса: на порядки больше реальных запросов.
const maxBodyBytes = 64 << 10

// defaultArchiveURL — страница архива результатов на timelottery.ru.
const defaultArchiveURL = "https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/"

type Handler struct {
	st            *store.Store
	auth          *auth.Manager
	cookieSecure  bool
	archiveURL    string        // источник синхронизации; переопределяется в тестах
	archiveClient *http.Client  // клиент скачивания архива; подменяется в тестах
	llm           *llm.Client   // nil — AI-генерация не настроена
	aiSlots       chan struct{} // слот параллельного AI-запроса; ёмкость 1 — домашний LLM обрабатывает по одному
}

// newHandler собирает Handler; маршруты регистрирует register.
// Таймаут клиента должен оставаться меньше WriteTimeout HTTP-сервера
// (15 с в cmd/server/main.go), иначе вставки закоммитятся, а ответ
// до клиента не дойдёт.
func newHandler(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client) *Handler {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	return &Handler{
		st:            st,
		auth:          auth.New(password, secret),
		cookieSecure:  cookieSecure,
		archiveURL:    archiveURL,
		archiveClient: &http.Client{Timeout: 10 * time.Second},
		llm:           llmClient,
		aiSlots:       make(chan struct{}, 1),
	}
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
// Пустой archiveURL заменяется на defaultArchiveURL; llmClient == nil
// выключает AI-генерацию.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client) *http.ServeMux {
	h := newHandler(st, password, secret, cookieSecure, archiveURL, llmClient)
	mux := http.NewServeMux()
	h.register(mux)
	return mux
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
