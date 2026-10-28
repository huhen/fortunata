package api

import (
	"net/http"

	"fortunata/internal/generate"
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
	mainFreq, bonusFreq, _, err := h.archiveFrequencies()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось прочитать базу розыгрышей")
		return
	}
	tickets := generate.GenerateBatch(mainFreq, bonusFreq, req.Count, generate.CryptoRand{})
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

// archiveFrequencies читает архив и считает частоты чисел: общий шаг
// обычной и AI-генерации.
func (h *Handler) archiveFrequencies() (main, bonus []int, total int, err error) {
	draws, err := h.st.List()
	if err != nil {
		return nil, nil, 0, err
	}
	freq := make([]generate.DrawFreq, len(draws))
	for i, d := range draws {
		freq[i] = generate.DrawFreq{Numbers: d.Numbers, Bonus: d.Bonus}
	}
	main, bonus = generate.Frequencies(freq)
	return main, bonus, len(draws), nil
}
