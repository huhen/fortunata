// Статус последней синки (в памяти процесса) и расписание автосинхронизации
// для админки: GET /api/sync-status.
package api

import (
	"context"
	"net/http"
	"time"
)

// syncStatus — итог последней синки (ручной или автоматической). Хранится
// в памяти: после рестарта last == nil до первой синки — догон при старте
// заполняет его сразу.
type syncStatus struct {
	At      time.Time `json:"at"`
	Added   int       `json:"added"`
	Skipped int       `json:"skipped"`
	Issues  int       `json:"issues"`
	Err     string    `json:"error,omitempty"` // пусто — успех
}

// runSyncAndRecord — синка с записью итога в статус. Её вызывает и
// POST /api/sync, и планировщик (через обёртку в newHandler). Ошибка
// возвращается наружу — по ней планировщик планирует повтор.
func (h *Handler) runSyncAndRecord(ctx context.Context) (syncResult, error) {
	res, err := h.runSync(ctx)
	st := syncStatus{At: time.Now().UTC(), Issues: len(res.Issues)}
	if err != nil {
		st.Err = err.Error()
	} else {
		st.Added, st.Skipped = res.Added, res.Skipped
	}
	h.mu.Lock()
	h.lastSync = &st
	h.mu.Unlock()
	return res, err
}

type syncStatusResponse struct {
	Enabled  bool        `json:"enabled"`
	Schedule string      `json:"schedule"`
	NextRun  *time.Time  `json:"nextRun"`
	Last     *syncStatus `json:"last"`
}

// syncStatusHandler — GET /api/sync-status (под сессией, как остальные
// админские маршруты).
func (h *Handler) syncStatusHandler(w http.ResponseWriter, r *http.Request) {
	resp := syncStatusResponse{Enabled: h.syncEnabled, Schedule: h.syncAt}
	if h.sched != nil {
		nr := h.sched.NextRun()
		resp.NextRun = &nr
	}
	h.mu.Lock()
	resp.Last = h.lastSync
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}
