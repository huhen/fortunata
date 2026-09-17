package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"fortunata/internal/version"
)

// TestGetVersion: публичная ручка без сессии отдаёт версию из пакета version.
func TestGetVersion(t *testing.T) {
	ts := newTestServer(t)

	resp := get(t, ts.Client(), ts.URL+"/api/version")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус: получено %d, ожидается 200", resp.StatusCode)
	}
	var data struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("декодирование ответа: %v", err)
	}
	if data.Version != version.Version {
		t.Fatalf("версия: получено %q, ожидается %q", data.Version, version.Version)
	}
}
