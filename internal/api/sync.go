// Синхронизация розыгрышей с архивом timelottery.ru: скачиваем страницу,
// разбираем и вставляем только новые розыгрыши. Существующие не трогаем.
package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"fortunata/internal/store"
	"fortunata/internal/timelottery"
)

// archiveClient — клиент скачивания архива; таймаут меньше WriteTimeout
// сервера, чтобы ответ успел уйти клиенту.
var archiveClient = &http.Client{Timeout: 20 * time.Second}

// maxArchiveBytes — лимит размера страницы архива (реальная ~0,2 МБ).
const maxArchiveBytes = 5 << 20

type syncIssue struct {
	DrawNo int64  `json:"drawNo"`
	Reason string `json:"reason"`
}

type syncResult struct {
	Added   int         `json:"added"`
	Skipped int         `json:"skipped"`
	Total   int         `json:"total"` // added + skipped + len(issues)
	Issues  []syncIssue `json:"issues"`
}

func (h *Handler) syncDraws(w http.ResponseWriter, r *http.Request) {
	draws, issues, err := fetchArchive(h.archiveURL)
	if err != nil {
		errorJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	res := syncResult{Total: len(draws) + len(issues), Issues: []syncIssue{}}
	for _, is := range issues {
		res.Issues = append(res.Issues, syncIssue{DrawNo: is.DrawNo, Reason: is.Reason})
	}
	for _, d := range draws {
		err := h.st.Create(store.Draw{DrawNo: d.No, Numbers: d.Numbers, Bonus: d.Bonus})
		switch {
		case err == nil:
			res.Added++
		case errors.Is(err, store.ErrDuplicate):
			res.Skipped++
		default:
			slog.Error("sync: вставка розыгрыша", "no", d.No, "err", err)
			res.Issues = append(res.Issues, syncIssue{DrawNo: d.No, Reason: "ошибка сохранения"})
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// fetchArchive скачивает страницу архива и разбирает её.
func fetchArchive(url string) ([]timelottery.Draw, []timelottery.Issue, error) {
	resp, err := archiveClient.Get(url)
	if err != nil {
		slog.Warn("sync: загрузка архива", "url", url, "err", err)
		return nil, nil, errors.New("не удалось загрузить страницу архива")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("сервер архива ответил %d", resp.StatusCode)
	}
	return timelottery.Parse(io.LimitReader(resp.Body, maxArchiveBytes))
}
