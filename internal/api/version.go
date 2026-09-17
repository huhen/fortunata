package api

import (
	"net/http"

	"fortunata/internal/version"
)

// getVersion — публичная ручка: версия сборки для футера и флаг
// настроенности AI-генерации для главной страницы.
func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": version.Version, "ai": h.llm != nil})
}
