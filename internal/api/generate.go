package api

import (
	"net/http"

	"generator/internal/generate"
)

// maxTicketCount — верхняя граница пачки.
const maxTicketCount = 100

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Count < 1 || req.Count > maxTicketCount {
		errorJSON(w, http.StatusBadRequest, "Количество билетов — от 1 до 100")
		return
	}
	draws, err := h.st.List()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось прочитать базу розыгрышей")
		return
	}
	freq := make([]generate.DrawFreq, len(draws))
	for i, d := range draws {
		freq[i] = generate.DrawFreq{Numbers: d.Numbers, Bonus: d.Bonus}
	}
	mainFreq, bonusFreq := generate.Frequencies(freq)
	tickets := generate.GenerateBatch(mainFreq, bonusFreq, req.Count, generate.CryptoRand{})
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}
