// Синхронизация розыгрышей с архивом timelottery.ru: скачиваем страницу,
// разбираем и вставляем только новые розыгрыши. Существующие не трогаем.
package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"fortunata/internal/store"
	"fortunata/internal/timelottery"
)

// maxArchiveBytes — лимит размера страницы архива (реальная ~0,2 МБ).
const maxArchiveBytes = 5 << 20

// errArchiveTooBig — страница архива превысила лимит; после оборачивания
// в html.Parse распознаётся через errors.Is.
var errArchiveTooBig = errors.New("страница архива больше 5 МБ")

// limitedReader читает не более n байт, дальше — errArchiveTooBig:
// io.LimitReader тихо обрезал бы страницу, давая частичный парс с 200.
type limitedReader struct {
	r io.Reader
	n int64 // сколько байтов осталось прочитать
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errArchiveTooBig
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

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
	draws, issues, err := h.fetchArchive(h.archiveURL)
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
func (h *Handler) fetchArchive(url string) ([]timelottery.Draw, []timelottery.Issue, error) {
	resp, err := h.archiveClient.Get(url)
	if err != nil {
		slog.Warn("sync: загрузка архива", "url", url, "err", err)
		return nil, nil, errors.New("не удалось загрузить страницу архива")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("сервер архива ответил %d", resp.StatusCode)
	}
	draws, issues, err := timelottery.Parse(&limitedReader{r: resp.Body, n: maxArchiveBytes})
	if err != nil {
		if errors.Is(err, errArchiveTooBig) {
			return nil, nil, errArchiveTooBig // без префикса «разбор html:»
		}
		return nil, nil, err
	}
	return draws, issues, nil
}
