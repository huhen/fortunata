package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"generator/internal/store"
)

type drawPayload struct {
	DrawNo  int64 `json:"drawNo"`
	Numbers []int `json:"numbers"`
	Bonus   int   `json:"bonus"`
}

func (h *Handler) listDraws(w http.ResponseWriter, r *http.Request) {
	draws, err := h.st.List()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось получить список розыгрышей")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draws": draws})
}

func (h *Handler) createDraw(w http.ResponseWriter, r *http.Request) {
	var p drawPayload
	if !readJSON(w, r, &p) {
		return
	}
	if p.DrawNo < 1 {
		errorJSON(w, http.StatusBadRequest, "Номер розыгрыша должен быть ≥ 1")
		return
	}
	if msg := validateDraw(p.Numbers, p.Bonus); msg != "" {
		errorJSON(w, http.StatusBadRequest, msg)
		return
	}
	sort.Ints(p.Numbers)
	err := h.st.Create(store.Draw{DrawNo: p.DrawNo, Numbers: p.Numbers, Bonus: p.Bonus})
	if errors.Is(err, store.ErrDuplicate) {
		errorJSON(w, http.StatusConflict,
			"Розыгрыш № "+strconv.FormatInt(p.DrawNo, 10)+" уже есть — отредактируйте его")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось сохранить розыгрыш")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) updateDraw(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.ParseInt(r.PathValue("no"), 10, 64)
	if err != nil || no < 1 {
		errorJSON(w, http.StatusBadRequest, "Некорректный номер розыгрыша")
		return
	}
	var p drawPayload
	if !readJSON(w, r, &p) {
		return
	}
	if msg := validateDraw(p.Numbers, p.Bonus); msg != "" {
		errorJSON(w, http.StatusBadRequest, msg)
		return
	}
	sort.Ints(p.Numbers)
	if err := h.st.Update(no, p.Numbers, p.Bonus); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			errorJSON(w, http.StatusNotFound, "Розыгрыш не найден")
		default:
			errorJSON(w, http.StatusInternalServerError, "Не удалось сохранить розыгрыш")
		}
		return
	}
	p.DrawNo = no
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) deleteDraw(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.ParseInt(r.PathValue("no"), 10, 64)
	if err != nil || no < 1 {
		errorJSON(w, http.StatusBadRequest, "Некорректный номер розыгрыша")
		return
	}
	if err := h.st.Delete(no); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			errorJSON(w, http.StatusNotFound, "Розыгрыш не найден")
		default:
			errorJSON(w, http.StatusInternalServerError, "Не удалось удалить розыгрыш")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateDraw возвращает "" если комбинация корректна, иначе — сообщение пользователю.
func validateDraw(numbers []int, bonus int) string {
	if len(numbers) != 7 {
		return "Нужно ровно 7 основных чисел"
	}
	seen := make(map[int]struct{}, 7)
	for _, n := range numbers {
		if n < 1 || n > 35 {
			return "Основные числа должны быть в диапазоне 1–35"
		}
		if _, dup := seen[n]; dup {
			return "Основные числа не должны повторяться"
		}
		seen[n] = struct{}{}
	}
	if bonus < 1 || bonus > 54 {
		return "Бонусное число должно быть в диапазоне 1–54"
	}
	return ""
}
