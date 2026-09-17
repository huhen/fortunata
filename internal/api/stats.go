package api

import "net/http"

// stats — публичная частотная статистика по архиву: основные и бонусные шары.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	main, err := h.st.MainFrequency()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось посчитать статистику")
		return
	}
	bonus, err := h.st.BonusFrequency()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось посчитать статистику")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"main": main, "bonus": bonus})
}
