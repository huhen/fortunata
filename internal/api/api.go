// Пакет api: JSON API приложения.
package api

import (
	"encoding/json"
	"net/http"

	"fortunata/internal/auth"
	"fortunata/internal/store"
)

// maxBodyBytes — лимит тела запроса: на порядки больше реальных запросов.
const maxBodyBytes = 64 << 10

// defaultArchiveURL — страница архива результатов на timelottery.ru.
const defaultArchiveURL = "https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/"

type Handler struct {
	st           *store.Store
	auth         *auth.Manager
	cookieSecure bool
	archiveURL   string // источник синхронизации; переопределяется в тестах
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
// Пустой archiveURL заменяется на defaultArchiveURL.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string) *http.ServeMux {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	h := &Handler{st: st, auth: auth.New(password, secret), cookieSecure: cookieSecure, archiveURL: archiveURL}
	mux := http.NewServeMux()
	mux.Handle("POST /api/login", requireJSON(h.login))
	mux.Handle("POST /api/logout", requireJSON(h.logout))
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("GET /api/draws", h.listDraws)
	mux.Handle("POST /api/draws", h.session(requireJSON(h.createDraw)))
	mux.Handle("PUT /api/draws/{no}", h.session(requireJSON(h.updateDraw)))
	mux.Handle("DELETE /api/draws/{no}", h.session(h.deleteDraw))
	mux.Handle("POST /api/sync", h.session(requireJSON(h.syncDraws)))
	mux.Handle("POST /api/generate", requireJSON(h.generate))
	return mux
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
