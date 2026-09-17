// Тесты POST /api/generate/ai: LLM — httptest-заглушка через инъекцию клиента.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fortunata/internal/llm"
	"fortunata/internal/store"
)

// respondLLM пишет ответ OpenAI-формата с фиксированным content.
func respondLLM(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
	})
}

// newAIServer поднимает api с LLM-заглушкой handler.
func newAIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, "",
		llm.NewClient(upstream.URL, "test-model", "", nil)))
	t.Cleanup(ts.Close)
	return ts
}

func TestGenerateAIBusy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("LLM не должен вызываться, когда слот занят")
	}))
	t.Cleanup(upstream.Close)
	h := newHandler(newTestStore(t), "pass123", "test-secret", false, "",
		llm.NewClient(upstream.URL, "test-model", "", nil))
	h.aiSlots <- struct{}{} // слот занят
	mux := http.NewServeMux()
	h.register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, хотим 503", resp.StatusCode)
	}
}

func TestGenerateAIHappyPath(t *testing.T) {
	// Публичный доступ: клиент без логина.
	calls := 0
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		respondLLM(w, `{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8}]}`)
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("вызовов LLM = %d, хотим 1", calls)
	}
	var body struct {
		Tickets []struct {
			Numbers []int `json:"numbers"`
			Bonus   int   `json:"bonus"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tickets) != 1 || body.Tickets[0].Bonus != 8 {
		t.Fatalf("tickets = %+v", body.Tickets)
	}
}

func TestGenerateAINotConfigured(t *testing.T) {
	ts := newTestServer(t) // без LLM-клиента
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, хотим 503", resp.StatusCode)
	}
}

func TestGenerateAIEmptyArchive(t *testing.T) {
	// Пустой архив — валидный режим: нулевые частоты, totalDraws=0.
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		respondLLM(w, `{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8}]}`)
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("пустая база: status = %d, хотим 200", resp.StatusCode)
	}
}

func TestGenerateAIValidation(t *testing.T) {
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("LLM не должен вызываться при невалидном count")
	})
	for _, count := range []int{0, -1, 21} {
		resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": count})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("count=%d: status = %d, хотим 400", count, resp.StatusCode)
		}
	}
}

func TestGenerateAIUpstreamError(t *testing.T) {
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body.Error, "500") {
		t.Fatalf("в ошибке нет кода upstream: %q", body.Error)
	}
}

func TestGenerateAINoValidCombinations(t *testing.T) {
	calls := 0
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		respondLLM(w, `{"combinations":[{"numbers":[1,2],"bonus":8}]}`)
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 1})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
	if calls != 3 { // запрос + 2 добора
		t.Fatalf("вызовов LLM = %d, хотим 3", calls)
	}
}

func TestGenerateAIPartialResult(t *testing.T) {
	calls := 0
	ts := newAIServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// один валидный + один невалидный
			respondLLM(w, `{"combinations":[{"numbers":[1,2,3,4,5,6,7],"bonus":8},{"numbers":[99],"bonus":8}]}`)
			return
		}
		// добор приносит только дубль
		respondLLM(w, `{"combinations":[{"numbers":[7,6,5,4,3,2,1],"bonus":8}]}`)
	})
	resp := post(t, clientWithJar(), ts.URL+"/api/generate/ai", map[string]any{"count": 2})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, недобор ≥1 билета — это 200", resp.StatusCode)
	}
	var body struct {
		Tickets []json.RawMessage `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tickets) != 1 {
		t.Fatalf("билетов = %d, хотим 1 (частичный результат)", len(body.Tickets))
	}
}
