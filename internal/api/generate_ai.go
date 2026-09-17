// AI-генерация: LLM предлагает комбинации по статистике архива.
package api

import (
	"errors"
	"log/slog"
	"net/http"

	"fortunata/internal/generate"
	"fortunata/internal/llm"
)

// maxAICount — верхняя граница для AI: длинные ответы LLM ломаются чаще.
const maxAICount = 20

func (h *Handler) generateAI(w http.ResponseWriter, r *http.Request) {
	if h.llm == nil {
		errorJSON(w, http.StatusServiceUnavailable, "AI генерация не настроена")
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Count < 1 || req.Count > maxAICount {
		errorJSON(w, http.StatusBadRequest, "Количество билетов — от 1 до 20")
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
	tickets, err := h.llm.Propose(r.Context(), req.Count, mainFreq, bonusFreq, len(draws))
	if err != nil {
		if errors.Is(err, llm.ErrUnavailable) {
			errorJSON(w, http.StatusBadGateway, "LLM-сервер недоступен")
			return
		}
		errorJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(tickets) == 0 {
		errorJSON(w, http.StatusBadGateway, "LLM не смог предложить корректные комбинации")
		return
	}
	if len(tickets) < req.Count {
		slog.Warn("generate-ai: недобор комбинаций", "запрошено", req.Count, "получено", len(tickets))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}
