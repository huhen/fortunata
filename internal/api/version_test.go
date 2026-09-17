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
		AI      bool   `json:"ai"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("декодирование ответа: %v", err)
	}
	if data.Version != version.Version {
		t.Fatalf("версия: получено %q, ожидается %q", data.Version, version.Version)
	}
	if data.AI {
		t.Fatal("без LLM-конфига флаг ai должен быть false")
	}
}

// TestGetVersionAIEnabled: при настроенном LLM флаг ai = true.
func TestGetVersionAIEnabled(t *testing.T) {
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("LLM не должен вызываться из /api/version")
	})
	resp := get(t, clientWithJar(), ts.URL+"/api/version")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус: получено %d, ожидается 200", resp.StatusCode)
	}
	var data struct {
		AI bool `json:"ai"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("декодирование ответа: %v", err)
	}
	if !data.AI {
		t.Fatal("с LLM-клиентом флаг ai должен быть true")
	}
}
