package api

import (
	"net/http"

	"fortunata/internal/version"
)

// getVersion — публичная ручка с версией сборки для футера фронтенда.
func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version.Version})
}
