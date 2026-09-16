package api

import (
	"net/http"
	"time"

	"fortunata/internal/auth"
)

// wrongPasswordPause — пауза перед ответом 401, замедляет перебор пароля.
const wrongPasswordPause = time.Second

// setSessionCookie ставит (или при maxAge<0 удаляет) cookie-сессию.
func (h *Handler) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.cookieSecure,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !h.auth.CheckPassword(req.Password) {
		time.Sleep(wrongPasswordPause)
		errorJSON(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}
	h.setSessionCookie(w, h.auth.NewToken(), int(auth.SessionTTL.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// me — есть ли у запроса валидная сессия (для показа админ-панели).
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	authed := false
	if c, err := r.Cookie(auth.CookieName); err == nil {
		authed = h.auth.Verify(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": authed})
}
