// Тесты клиента и добора: LLM подменяется httptest-сервером.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// respondLLM пишет ответ OpenAI-формата с фиксированным content.
func respondLLM(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
	})
}

// newTestClient подменяет LLM на httptest-сервер.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	return NewClient(upstream.URL, "test-model", "key123", nil)
}

func TestCompleteSendsChatRequest(t *testing.T) {
	var path, auth, model string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("декодирование запроса: %v", err)
		}
		model = req.Model
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Errorf("messages = %+v", req.Messages)
		}
		if req.Messages[1].Content != "дай комбинации" {
			t.Errorf("user-промт = %q", req.Messages[1].Content)
		}
		respondLLM(w, "привет")
	})
	out, err := c.Complete(context.Background(), "дай комбинации")
	if err != nil {
		t.Fatal(err)
	}
	if out != "привет" {
		t.Fatalf("out = %q", out)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer key123" {
		t.Fatalf("auth = %q", auth)
	}
	if model != "test-model" {
		t.Fatalf("model = %q", model)
	}
}

func TestCompleteWithoutAPIKeyNoAuthHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization не должен отправляться без api_key")
		}
		respondLLM(w, "ok")
	}))
	t.Cleanup(upstream.Close)
	c := NewClient(upstream.URL, "m", "", nil)
	if _, err := c.Complete(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteUnavailableOnDeadServer(t *testing.T) {
	// Порт 1 на loopback закрыт — соединение отклоняется мгновенно.
	c := NewClient("http://127.0.0.1:1", "m", "", nil)
	_, err := c.Complete(context.Background(), "hi")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, хотим ErrUnavailable", err)
	}
}

func TestCompleteHTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.Complete(context.Background(), "hi")
	if err == nil || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, хотим ошибку с кодом 500 без ErrUnavailable", err)
	}
}

func TestCompleteEmptyChoices(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	})
	_, err := c.Complete(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "пустой ответ") {
		t.Fatalf("err = %v, хотим «пустой ответ»", err)
	}
}

func TestCompleteTrimsTrailingSlash(t *testing.T) {
	var path string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		respondLLM(w, "ok")
	}))
	t.Cleanup(upstream.Close)
	c := NewClient(upstream.URL+"/", "m", "", nil)
	if _, err := c.Complete(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("path = %q, двойной слэш?", path)
	}
}

func TestProposeHappyPathSingleCall(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		respondLLM(w, `{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8}]}`)
	})
	got, err := c.Propose(context.Background(), 1, make([]int, 35), make([]int, 54), 0)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(got) != 1 || got[0].Bonus != 8 {
		t.Fatalf("calls = %d, билеты = %+v", calls, got)
	}
}

func TestProposeRefillsAndDedupes(t *testing.T) {
	calls := 0
	var secondPrompt string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("декодирование: %v", err)
		}
		if calls == 1 {
			// один валидный билет + один невалидный
			respondLLM(w, `{"combinations":[{"numbers":[1,2,3,4,5,6,7],"bonus":8},{"numbers":[1,2],"bonus":8}]}`)
			return
		}
		secondPrompt = req.Messages[1].Content
		// новый валидный + дубль первого (числа в обратном порядке)
		respondLLM(w, `{"combinations":[{"numbers":[9,10,11,12,13,14,15],"bonus":9},{"numbers":[7,6,5,4,3,2,1],"bonus":8}]}`)
	})
	got, err := c.Propose(context.Background(), 2, make([]int, 35), make([]int, 54), 5)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, хотим 2", calls)
	}
	if len(got) != 2 || got[1].Numbers[0] != 9 {
		t.Fatalf("билеты = %+v, дубль должен быть отфильтрован", got)
	}
	if !strings.Contains(secondPrompt, "do not repeat") {
		t.Fatalf("в промте добора нет exclude-списка:\n%s", secondPrompt)
	}
}

func TestProposeGivesUpAfterMaxRefills(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		respondLLM(w, `{"combinations":[{"numbers":[1,2],"bonus":8}]}`)
	})
	got, err := c.Propose(context.Background(), 2, make([]int, 35), make([]int, 54), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("билетов = %d, хотим 0", len(got))
	}
	if calls != maxRefills+1 {
		t.Fatalf("calls = %d, хотим %d (запрос + доборы)", calls, maxRefills+1)
	}
}
