// Тесты GET /api/sync-status и записи статуса последней синки.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newStatusServer — api с апстримом, отдающим body, и указанным SyncConfig.
func newStatusServer(t *testing.T, syncCfg SyncConfig, body string) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	st := newTestStore(t)
	mux, _ := New(st, "pass123", "test-secret", false, upstream.URL, nil, syncCfg)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

type statusResp struct {
	Enabled  bool       `json:"enabled"`
	Schedule string     `json:"schedule"`
	NextRun  *time.Time `json:"nextRun"`
	Last     *struct {
		At      time.Time `json:"at"`
		Added   int       `json:"added"`
		Skipped int       `json:"skipped"`
		Issues  int       `json:"issues"`
		Err     string    `json:"error"`
	} `json:"last"`
}

func decodeStatus(t *testing.T, resp *http.Response) statusResp {
	t.Helper()
	var body statusResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return body
}

func TestSyncStatusRequiresSession(t *testing.T) {
	ts := newStatusServer(t, SyncConfig{Hour: 15, Enabled: true}, readArchiveFixture(t))
	resp := get(t, clientWithJar(), ts.URL+"/api/sync-status")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, хотим 401", resp.StatusCode)
	}
}

func TestSyncStatusEnabledBeforeSync(t *testing.T) {
	ts := newStatusServer(t, SyncConfig{Hour: 15, Enabled: true}, readArchiveFixture(t))
	c := loginClient(t, ts)
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if !body.Enabled || body.Schedule != "15:00 UTC" {
		t.Fatalf("body = %+v", body)
	}
	if body.NextRun == nil {
		t.Fatal("nextRun = nil при включённом планировщике")
	}
	if body.Last != nil {
		t.Fatalf("last = %+v до первой синки, хотим nil", body.Last)
	}
}

func TestSyncStatusDisabled(t *testing.T) {
	ts := newStatusServer(t, SyncConfig{}, readArchiveFixture(t))
	c := loginClient(t, ts)
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if body.Enabled {
		t.Fatalf("enabled = true при SyncConfig{}: %+v", body)
	}
	if body.NextRun != nil {
		t.Fatalf("nextRun = %v при выключенном планировщике, хотим nil", *body.NextRun)
	}
}

// Успешная ручная синка записывается в статус (он же пишет планировщик).
func TestSyncStatusRecordsSync(t *testing.T) {
	ts := newStatusServer(t, SyncConfig{Hour: 15, Enabled: true}, readArchiveFixture(t))
	c := loginClient(t, ts)
	post(t, c, ts.URL+"/api/sync", map[string]any{}).Body.Close()
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if body.Last == nil {
		t.Fatal("last = nil после синка")
	}
	if body.Last.Added != 2 || body.Last.Err != "" || body.Last.At.IsZero() {
		t.Fatalf("last = %+v", body.Last)
	}
}

// Упавшая синка записывается в статус с ошибкой — планировщик по этой же
// ошибке планирует повтор.
func TestSyncStatusRecordsError(t *testing.T) {
	ts := newStatusServer(t, SyncConfig{Hour: 15, Enabled: true}, "<html><body><p>Пусто</p></body></html>")
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("sync status = %d, хотим 502", resp.StatusCode)
	}
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if body.Last == nil || body.Last.Err == "" || body.Last.Added != 0 {
		t.Fatalf("last = %+v, ожидали ошибку", body.Last)
	}
}

// JSON-контракт поля last.issues: синка со смешанной страницей (одна
// валидная строка, одна битая) записывает количество проблем.
func TestSyncStatusRecordsIssues(t *testing.T) {
	mixed := `<html><body><table>
<tr><td>64</td><td>14 сент</td><td><strong>19, 28, 24, 21, 10, 29, 05 и 18</strong></td><td>10 млн</td></tr>
<tr><td>62</td><td>3 сент</td><td><strong>1, 2, 3, 4, 5, 36, 07 и 08</strong></td><td>5 млн</td></tr>
</table></body></html>`
	ts := newStatusServer(t, SyncConfig{Hour: 15, Enabled: true}, mixed)
	c := loginClient(t, ts)
	post(t, c, ts.URL+"/api/sync", map[string]any{}).Body.Close()
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if body.Last == nil || body.Last.Added != 1 || body.Last.Issues != 1 {
		t.Fatalf("last = %+v, ожидали added=1, issues=1", body.Last)
	}
}

// Склейка планировщика со статусом: fn планировщика — тот же
// runSyncAndRecord, его вызов напрямую (как делает планировщик) должен
// отразиться в GET /api/sync-status.
func TestSyncStatusRecordsViaSchedulerFn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readArchiveFixture(t)))
	}))
	t.Cleanup(upstream.Close)
	h := newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL, nil,
		SyncConfig{Hour: 15, Enabled: true})
	if _, err := h.runSyncAndRecord(context.Background()); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	c := loginClient(t, ts)
	body := decodeStatus(t, get(t, c, ts.URL+"/api/sync-status"))
	if body.Last == nil || body.Last.Added != 2 || body.Last.Err != "" {
		t.Fatalf("last = %+v, ожидали added=2", body.Last)
	}
}
