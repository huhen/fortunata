// Тесты POST /api/sync: «архив» — отдельный httptest-сервер с фикстурой.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"fortunata/internal/store"
)

// readArchiveFixture читает фикстуру из пакета timelottery — один и тот же
// HTML используется в тестах парсера и хендлера.
func readArchiveFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../timelottery/testdata/archive.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// newSyncServer поднимает api, у которого «архив» отдаёт handler.
func newSyncServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, upstream.URL))
	t.Cleanup(ts.Close)
	return ts
}

func TestSyncAddsNewDrawsAndIdempotent(t *testing.T) {
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readArchiveFixture(t)))
	})
	c := loginClient(t, ts)

	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	var res struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
		Total   int `json:"total"`
		Issues  []struct {
			DrawNo int64 `json:"drawNo"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if res.Added != 2 || res.Skipped != 0 || res.Total != 2 || len(res.Issues) != 0 {
		t.Fatalf("первый вызов: %+v", res)
	}

	// Идемпотентность: повторный вызов ничего не добавляет.
	resp2 := post(t, c, ts.URL+"/api/sync", map[string]any{})
	var res2 struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&res2); err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if res2.Added != 0 || res2.Skipped != 2 {
		t.Fatalf("повторный вызов: %+v", res2)
	}

	// Данные в базе совпадают (чтение публичное, как на фронте).
	resp3 := get(t, clientWithJar(), ts.URL+"/api/draws")
	var list struct {
		Draws []struct {
			DrawNo  int64 `json:"drawNo"`
			Numbers []int `json:"numbers"`
			Bonus   int   `json:"bonus"`
		} `json:"draws"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	var d64 *struct {
		DrawNo  int64 `json:"drawNo"`
		Numbers []int `json:"numbers"`
		Bonus   int   `json:"bonus"`
	}
	for i := range list.Draws {
		if list.Draws[i].DrawNo == 64 {
			d64 = &list.Draws[i]
		}
	}
	if d64 == nil {
		t.Fatalf("розыгрыша 64 нет в базе: %+v", list.Draws)
	}
	want := []int{5, 10, 19, 21, 24, 28, 29}
	if d64.Bonus != 18 || len(d64.Numbers) != 7 {
		t.Fatalf("d64 = %+v", d64)
	}
	for i, n := range want {
		if d64.Numbers[i] != n {
			t.Fatalf("numbers = %v", d64.Numbers)
		}
	}
}

// Смешанная страница: валидная строка добавляется, невалидная уходит в issues.
func TestSyncIssuesPassedThrough(t *testing.T) {
	upstream := `<html><body><table>
<tr><td>64</td><td>14 сент</td><td><strong>19, 28, 24, 21, 10, 29, 05 и 18</strong></td><td>10 млн</td></tr>
<tr><td>62</td><td>3 сент</td><td><strong>1, 2, 3, 4, 5, 36, 07 и 08</strong></td><td>5 млн</td></tr>
</table></body></html>`
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(upstream))
	})
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var res struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
		Total   int `json:"total"`
		Issues  []struct {
			DrawNo int64  `json:"drawNo"`
			Reason string `json:"reason"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Skipped != 0 || res.Total != 2 || len(res.Issues) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if res.Issues[0].DrawNo != 62 || res.Issues[0].Reason == "" {
		t.Fatalf("issue = %+v", res.Issues[0])
	}
}

func TestSyncRequiresSession(t *testing.T) {
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readArchiveFixture(t)))
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/sync", map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, хотим 401", resp.StatusCode)
	}
}

func TestSyncArchiveWithoutResults(t *testing.T) {
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body><p>Пусто</p></body></html>"))
	})
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Error == "" {
		t.Fatal("ожидали сообщение об ошибке")
	}
}

func TestSyncUpstreamError(t *testing.T) {
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
}

// Контракт для фронта: issues — всегда массив, никогда null.
func TestSyncIssuesAlwaysArray(t *testing.T) {
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readArchiveFixture(t)))
	})
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"issues":[]`) {
		t.Fatalf(`в ответе нет "issues":[]: %s`, raw)
	}
}

func TestSyncUpstreamUnreachable(t *testing.T) {
	// Закрытый апстрим: transport-ошибка, а не HTTP-статус.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, deadURL))
	t.Cleanup(ts.Close)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
}
