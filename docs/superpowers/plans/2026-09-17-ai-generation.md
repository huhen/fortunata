# AI-генерация комбинаций через llama.cpp — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Кнопка «AI генерация»: LLM (llama.cpp, OpenAI-совместимый API) предлагает до 20 комбинаций по статистике архива; невалидные добираются повторными запросами.

**Architecture:** Новый пакет `internal/llm` (клиент `/v1/chat/completions`, промт EN, парсинг с устойчивостью к `<think>`/заборам, валидация, до 2 доборов). Тонкий хендлер `POST /api/generate/ai` в `internal/api`, тот же формат ответа, что у `/api/generate`. Кнопка фронтенда видна только при настроенном LLM (флаг `ai` в `GET /api/version`).

**Tech Stack:** Go 1.22+ (net/http ServeMux), vanilla JS, `httptest` для тестов.

**Спека:** `docs/superpowers/specs/2026-09-17-ai-generation-design.md`

---

## Структура файлов

| Файл | Действие | Ответственность |
|---|---|---|
| `internal/generate/generate.go` | Modify | `ticketKey` → экспортируемый `Key` (для дедупликации в llm) |
| `internal/llm/llm.go` | Create | `Client`, `NewClient`, `Complete`, `Propose`, `ErrUnavailable`, константы |
| `internal/llm/prompt.go` | Create | `systemPrompt`, `ComposePrompt` |
| `internal/llm/parse.go` | Create | `ExtractTickets`, `validTicket` |
| `internal/llm/*_test.go` | Create | тесты парсера, промта, клиента, добора |
| `internal/api/api.go` | Modify | поле `Handler.llm`, параметр `New`/`newHandler`, маршрут |
| `internal/api/generate_ai.go` | Create | хендлер `generateAI` |
| `internal/api/ai_generate_test.go` | Create | тесты хендлера с httptest-заглушкой LLM |
| `internal/api/version.go` | Modify | поле `ai` в ответе |
| `internal/api/version_test.go` | Modify | проверка `ai: false` без конфига |
| `internal/api/api_test.go`, `internal/api/sync_test.go` | Modify | `, nil` в вызовах `New`/`newHandler`; `writeTimeout` 15с → 130с |
| `cmd/server/main.go` | Modify | `Config.LLM*`, `loadConfig`, сборка клиента, `WriteTimeout` 130с |
| `cmd/server/main_test.go` | Modify | тесты `LLM_*` |
| `web/index.html` | Modify | кнопка `#btn-generate-ai` (hidden) |
| `web/app.js` | Modify | `renderTickets`, видимость по `/api/version`, обработчик AI-клипа |
| `compose.yaml`, `compose.local.yaml`, `.env.example` | Modify | passthrough `LLM_*` |
| `README.md`, `CLAUDE.md` | Modify | документация |

---

### Task 1: экспорт `generate.Key`

**Files:**
- Modify: `internal/generate/generate.go:120-122`
- Test: `internal/generate/generate_test.go`

- [ ] **Step 1: Пишем падающий тест**

В конец `internal/generate/generate_test.go` добавить:

```go
func TestKey(t *testing.T) {
	k := Key(Ticket{Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8})
	if k != "[1 2 3 4 5 6 7]+8" {
		t.Fatalf("Key = %q", k)
	}
}
```

- [ ] **Step 2: Убеждаемся, что тест падает**

Run: `go test ./internal/generate/ -run TestKey -v`
Expected: FAIL — `undefined: Key`

- [ ] **Step 3: Переименовываем ticketKey → Key**

В `internal/generate/generate.go` заменить:

```go
func ticketKey(t Ticket) string {
	return fmt.Sprintf("%v+%d", t.Numbers, t.Bonus)
}
```

на:

```go
// Key — строковый ключ билета для дедупликации пачки (числа + бонус).
// Экспортирован: используется также пакетом llm.
func Key(t Ticket) string {
	return fmt.Sprintf("%v+%d", t.Numbers, t.Bonus)
}
```

В `GenerateBatch` заменить оба использования `ticketKey(t)` на `Key(t)` (строки с `key := ticketKey(t)`).

- [ ] **Step 4: Все тесты пакета зелёные**

Run: `go test ./internal/generate/`
Expected: PASS (включая существующие тесты GenerateBatch)

- [ ] **Step 5: Коммит**

```bash
git add internal/generate/generate.go internal/generate/generate_test.go
git commit -m "refactor: экспорт generate.Key для дедупликации билетов"
```

---

### Task 2: `internal/llm` — парсер ответа

**Files:**
- Create: `internal/llm/parse.go`
- Test: `internal/llm/parse_test.go`

- [ ] **Step 1: Пишем падающие тесты**

`internal/llm/parse_test.go`:

```go
// Тесты парсинга ответа LLM: чистый JSON, <think>-блоки qwen3, заборы,
// мусор вокруг, невалидные комбинации.
package llm

import (
	"reflect"
	"testing"

	"fortunata/internal/generate"
)

func TestExtractTicketsCleanJSON(t *testing.T) {
	content := `{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8},{"numbers":[1,2,3,4,5,6,7],"bonus":54}]}`
	got := ExtractTickets(content)
	if len(got) != 2 {
		t.Fatalf("билетов = %d, хотим 2", len(got))
	}
	if !reflect.DeepEqual(got[0].Numbers, []int{3, 7, 12, 19, 25, 31, 34}) || got[0].Bonus != 8 {
		t.Fatalf("билет 0 = %+v", got[0])
	}
}

func TestExtractTicketsThinkAndFences(t *testing.T) {
	content := "<think>Мне нужно выбрать числа… {\"fake\": true}</think>\n```json\n" +
		`{"combinations":[{"numbers":[34,31,25,19,12,7,3],"bonus":8}]}` + "\n```\nВот ваши числа!"
	got := ExtractTickets(content)
	if len(got) != 1 {
		t.Fatalf("билетов = %d, хотим 1", len(got))
	}
	// Числа пришли в обратном порядке — сортировка применена.
	if !reflect.DeepEqual(got[0].Numbers, []int{3, 7, 12, 19, 25, 31, 34}) {
		t.Fatalf("numbers = %v", got[0].Numbers)
	}
}

func TestExtractTicketsGarbageAroundJSON(t *testing.T) {
	content := `Sure! Here are your numbers: {"combinations":[{"numbers":[1,2,3,4,5,6,7],"bonus":8}]} Hope this helps!`
	got := ExtractTickets(content)
	if len(got) != 1 {
		t.Fatalf("билетов = %d, хотим 1", len(got))
	}
}

func TestExtractTicketsDropsInvalid(t *testing.T) {
	content := `{"combinations":[
		{"numbers":[1,2,3],"bonus":8},
		{"numbers":[1,2,3,4,5,6,36],"bonus":8},
		{"numbers":[1,2,3,4,5,6,7],"bonus":55},
		{"numbers":[1,1,3,4,5,6,7],"bonus":8},
		{"numbers":[1,2,3,4,5,6,7],"bonus":8}
	]}`
	got := ExtractTickets(content)
	if len(got) != 1 || got[0].Bonus != 8 {
		t.Fatalf("билеты = %+v, хотим 1 валидный", got)
	}
}

func TestExtractTicketsNoJSON(t *testing.T) {
	if got := ExtractTickets("Извините, не могу помочь."); len(got) != 0 {
		t.Fatalf("билеты = %+v, хотим пусто", got)
	}
	if got := ExtractTickets("<think>думаю"); len(got) != 0 {
		t.Fatalf("незакрытый think: %+v, хотим пусто", got)
	}
}
```

- [ ] **Step 2: Убеждаемся, что тесты падают**

Run: `go test ./internal/llm/ -v`
Expected: FAIL — `undefined: ExtractTickets` (пакета ещё нет; если go ругается на отсутствие пакетов — это и есть ожидаемый сбой)

- [ ] **Step 3: Пишем реализацию**

`internal/llm/parse.go`:

```go
// Парсинг ответа LLM: извлечение JSON с комбинациями и валидация билетов.
package llm

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"fortunata/internal/generate"
)

// thinkRe срезает парные <think>-блоки (qwen3 рассуждает перед ответом),
// unclosedThinkRe — незакрытый <think> до конца ответа.
var (
	thinkRe         = regexp.MustCompile(`(?s)<think>.*?</think>`)
	unclosedThinkRe = regexp.MustCompile(`(?s)<think>.*`)
)

// combinationsPayload — ожидаемая структура ответа модели.
type combinationsPayload struct {
	Combinations []struct {
		Numbers []int `json:"numbers"`
		Bonus   int   `json:"bonus"`
	} `json:"combinations"`
}

// ExtractTickets вытаскивает из ответа модели валидные билеты. Ответ может
// содержать <think>-рассуждения, ```-заборы и текст вокруг JSON. Невалидные
// комбинации отбрасываются; у валидных числа сортируются по возрастанию.
func ExtractTickets(content string) []generate.Ticket {
	cleaned := unclosedThinkRe.ReplaceAllString(thinkRe.ReplaceAllString(content, ""), "")
	start := strings.Index(cleaned, "{")
	if start < 0 {
		return nil
	}
	var payload combinationsPayload
	// Decode читает первый JSON-объект и игнорирует мусор после него.
	if err := json.NewDecoder(strings.NewReader(cleaned[start:])).Decode(&payload); err != nil {
		return nil
	}
	tickets := make([]generate.Ticket, 0, len(payload.Combinations))
	for _, c := range payload.Combinations {
		if t, ok := validTicket(c.Numbers, c.Bonus); ok {
			tickets = append(tickets, t)
		}
	}
	return tickets
}

// validTicket проверяет комбинацию: ровно 7 уникальных чисел 1–35, бонус 1–54.
func validTicket(numbers []int, bonus int) (generate.Ticket, bool) {
	if len(numbers) != 7 || bonus < 1 || bonus > 54 {
		return generate.Ticket{}, false
	}
	seen := make(map[int]struct{}, 7)
	for _, n := range numbers {
		if n < 1 || n > 35 {
			return generate.Ticket{}, false
		}
		if _, dup := seen[n]; dup {
			return generate.Ticket{}, false
		}
		seen[n] = struct{}{}
	}
	sorted := append([]int(nil), numbers...)
	sort.Ints(sorted)
	return generate.Ticket{Numbers: sorted, Bonus: bonus}, true
}
```

- [ ] **Step 4: Тесты зелёные**

Run: `go test ./internal/llm/ -v`
Expected: PASS, все 5 тестов

- [ ] **Step 5: Коммит**

```bash
git add internal/llm/parse.go internal/llm/parse_test.go
git commit -m "feat: парсер ответа LLM с валидацией комбинаций (internal/llm)"
```

---

### Task 3: `internal/llm` — промт

**Files:**
- Create: `internal/llm/prompt.go`
- Test: `internal/llm/prompt_test.go`

- [ ] **Step 1: Пишем падающие тесты**

`internal/llm/prompt_test.go`:

```go
// Тесты промта: правила, статистика, exclude-список.
package llm

import (
	"strings"
	"testing"

	"fortunata/internal/generate"
)

func TestComposePromptContainsRulesAndStats(t *testing.T) {
	main := make([]int, 35)
	main[0] = 5  // число 1 выпадало 5 раз
	bonus := make([]int, 54)
	bonus[7] = 2 // бонус 8 выпадал 2 раза
	p := ComposePrompt(10, main, bonus, 123, nil)
	for _, want := range []string{
		"Past draws analyzed: 123",
		"Propose 10 distinct combinations",
		"1:5", "8:2",
		"7 distinct main numbers", "ascending", "bonus number from 1 to 54",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("в промте нет %q\nпромт:\n%s", want, p)
		}
	}
}

func TestComposePromptExcludeList(t *testing.T) {
	exclude := []generate.Ticket{{Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}}
	p := ComposePrompt(3, make([]int, 35), make([]int, 54), 1, exclude)
	for _, want := range []string{"do not repeat", "[1 2 3 4 5 6 7] bonus 8", "Propose 3 distinct"} {
		if !strings.Contains(p, want) {
			t.Fatalf("в промте нет %q\nпромт:\n%s", want, p)
		}
	}
}
```

- [ ] **Step 2: Убеждаемся, что тесты падают**

Run: `go test ./internal/llm/ -run TestComposePrompt -v`
Expected: FAIL — `undefined: ComposePrompt`

- [ ] **Step 3: Пишем реализацию**

`internal/llm/prompt.go`:

```go
// Сборка промтов для LLM на английском: правила игры, статистика, формат ответа.
package llm

import (
	"fmt"
	"strings"

	"fortunata/internal/generate"
)

// systemPrompt — правила игры и требование к формату ответа.
const systemPrompt = `You are a lottery combination proposer for the Fortunata lottery.
Game rules:
- A ticket is 7 distinct main numbers from 1 to 35 in ascending order, plus 1 bonus number from 1 to 54.
- All tickets in your answer must be distinct from each other.

Respond with ONLY a JSON object in exactly this format, no markdown fences, no explanations:
{"combinations":[{"numbers":[3,7,12,19,25,31,34],"bonus":8}]}`

// ComposePrompt строит пользовательский промт: статистика выпадений,
// количество прошедших тиражей и просьба предложить count комбинаций.
// exclude — уже найденные комбинации: их повторять нельзя (добор).
func ComposePrompt(count int, main, bonus []int, totalDraws int, exclude []generate.Ticket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Past draws analyzed: %d.\n", totalDraws)
	b.WriteString("Main number frequency (number:times drawn):\n")
	b.WriteString(frequencyList(main))
	b.WriteString("Bonus number frequency (number:times drawn):\n")
	b.WriteString(frequencyList(bonus))
	if len(exclude) > 0 {
		b.WriteString("Already proposed combinations (do not repeat them):\n")
		for _, t := range exclude {
			fmt.Fprintf(&b, "- %v bonus %d\n", t.Numbers, t.Bonus)
		}
	}
	fmt.Fprintf(&b, "Propose %d distinct combinations for the next draw.\n", count)
	return b.String()
}

// frequencyList форматирует частоты как "1:5, 2:0, ...".
func frequencyList(freq []int) string {
	parts := make([]string, len(freq))
	for i, c := range freq {
		parts[i] = fmt.Sprintf("%d:%d", i+1, c)
	}
	return strings.Join(parts, ", ") + "\n"
}
```

- [ ] **Step 4: Тесты зелёные**

Run: `go test ./internal/llm/ -v`
Expected: PASS — тесты парсера и промта

- [ ] **Step 5: Коммит**

```bash
git add internal/llm/prompt.go internal/llm/prompt_test.go
git commit -m "feat: сборка англоязычного промта для LLM (правила, статистика, exclude)"
```

---

### Task 4: `internal/llm` — клиент и добор

**Files:**
- Create: `internal/llm/llm.go`
- Test: `internal/llm/client_test.go`

- [ ] **Step 1: Пишем падающие тесты**

`internal/llm/client_test.go`:

```go
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

	"fortunata/internal/generate"
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
```

- [ ] **Step 2: Убеждаемся, что тесты падают**

Run: `go test ./internal/llm/ -run "TestComplete|TestPropose" -v`
Expected: FAIL — `undefined: NewClient`

- [ ] **Step 3: Пишем реализацию**

`internal/llm/llm.go`:

```go
// Пакет llm: клиент llama.cpp-сервера (OpenAI-совместимый
// /v1/chat/completions), сборка промта, парсинг ответа и добор невалидных
// комбинаций повторными запросами.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fortunata/internal/generate"
)

const (
	// requestTimeout — лимит одного запроса к LLM; должен оставаться меньше
	// WriteTimeout HTTP-сервера в cmd/server/main.go, иначе ответ
	// AI-генерации закоммитится, а до клиента не дойдёт.
	requestTimeout = 120 * time.Second
	// maxTokens — запас на <think>-рассуждения qwen3 и 20 комбинаций.
	maxTokens = 4096
	// temperature — умеренная случайность предложений.
	temperature = 0.8
	// maxRefills — число доборов после первого запроса.
	maxRefills = 2
)

// ErrUnavailable — LLM-сервер недоступен (сеть, таймаут, отказ в соединении).
var ErrUnavailable = errors.New("LLM-сервер недоступен")

// Client — клиент llama.cpp-сервера; http.Client подменяемый для тестов.
type Client struct {
	baseURL string // без пути, напр. http://192.168.1.128:18020
	model   string
	apiKey  string
	hc      *http.Client
}

// NewClient собирает клиент; hc == nil заменяется на клиент с requestTimeout.
func NewClient(baseURL, model, apiKey string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		hc:      hc,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete отправляет system+user промт и возвращает текст ответа модели.
func (c *Client) Complete(ctx context.Context, userPrompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("кодирование запроса: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("сервер LLM ответил %d", resp.StatusCode)
	}
	var cr chatResponse
	// Лимит тела — гигиена: ожидаемый ответ на 20 комбинаций — десятки КБ.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&cr); err != nil {
		return "", fmt.Errorf("ответ LLM не JSON: %w", err)
	}
	if len(cr.Choices) == 0 || cr.Choices[0].Message.Content == "" {
		return "", errors.New("пустой ответ LLM")
	}
	return cr.Choices[0].Message.Content, nil
}

// Propose запрашивает count валидных комбинаций; если валидных меньше,
// добирает недостающее повторными запросами (максимум maxRefills) с
// exclude-списком уже найденных. Сетевые ошибки пробрасываются сразу.
// Может вернуть меньше count (вплоть до нуля) — решение об ошибке
// принимает вызывающий.
func (c *Client) Propose(ctx context.Context, count int, main, bonus []int, totalDraws int) ([]generate.Ticket, error) {
	seen := make(map[string]struct{}, count)
	tickets := make([]generate.Ticket, 0, count)
	for attempt := 0; len(tickets) < count && attempt <= maxRefills; attempt++ {
		var exclude []generate.Ticket
		if attempt > 0 {
			exclude = tickets
		}
		content, err := c.Complete(ctx, ComposePrompt(count-len(tickets), main, bonus, totalDraws, exclude))
		if err != nil {
			return nil, err
		}
		for _, t := range ExtractTickets(content) {
			key := generate.Key(t)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			tickets = append(tickets, t)
		}
	}
	return tickets, nil
}
```

- [ ] **Step 4: Тесты зелёные**

Run: `go test ./internal/llm/ -v`
Expected: PASS — все тесты пакета (парсер, промт, клиент, добор)

- [ ] **Step 5: Коммит**

```bash
git add internal/llm/llm.go internal/llm/client_test.go
git commit -m "feat: клиент llama.cpp с добором невалидных комбинаций"
```

---

### Task 5: API — сигнатура `New` с LLM-клиентом

Смена сигнатуры отдельным коммитом: сборка остаётся зелёной, хендлер добавляется в Task 6.

**Files:**
- Modify: `internal/api/api.go`
- Modify: `internal/api/api_test.go:26,455`, `internal/api/sync_test.go:45,227,251,271,328`
- Modify: `cmd/server/main.go:87`

- [ ] **Step 1: Меняем Handler, newHandler и New**

В `internal/api/api.go`:

Импорты дополнить `"fortunata/internal/llm"`. Структуру заменить на:

```go
type Handler struct {
	st            *store.Store
	auth          *auth.Manager
	cookieSecure  bool
	archiveURL    string       // источник синхронизации; переопределяется в тестах
	archiveClient *http.Client // клиент скачивания архива; подменяется в тестах
	llm           *llm.Client  // nil — AI-генерация не настроена
}
```

`newHandler` и `New` — добавить последний параметр `llmClient *llm.Client`:

```go
func newHandler(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client) *Handler {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	return &Handler{
		st:            st,
		auth:          auth.New(password, secret),
		cookieSecure:  cookieSecure,
		archiveURL:    archiveURL,
		archiveClient: &http.Client{Timeout: 10 * time.Second},
		llm:           llmClient,
	}
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
// Пустой archiveURL заменяется на defaultArchiveURL; llmClient == nil
// выключает AI-генерацию.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client) *http.ServeMux {
	h := newHandler(st, password, secret, cookieSecure, archiveURL, llmClient)
	mux := http.NewServeMux()
	h.register(mux)
	return mux
}
```

- [ ] **Step 2: Добавляем `, nil` во все вызовы**

Каждый вызов `New(...)`/`newHandler(...)` дополнить последним аргументом `nil`:

- `internal/api/api_test.go:26` — `New(st, "pass123", "test-secret", false, "", nil)`
- `internal/api/api_test.go:455` — то же
- `internal/api/sync_test.go:45` — `New(st, "pass123", "test-secret", false, upstream.URL, nil)`
- `internal/api/sync_test.go:227` — `New(st, "pass123", "test-secret", false, deadURL, nil)`
- `internal/api/sync_test.go:251` — `newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL, nil)`
- `internal/api/sync_test.go:271` — `newHandler(newTestStore(t), "p", "s", false, "", nil)`
- `internal/api/sync_test.go:328` — `newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL, nil)`
- `cmd/server/main.go:87` — `api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure, "", nil)` (в Task 7 сюда придёт настоящий клиент)

- [ ] **Step 3: Сборка и тесты зелёные**

Run: `go build ./... && go test ./internal/...`
Expected: сборка OK, тесты PASS

- [ ] **Step 4: Коммит**

```bash
git add internal/api/ cmd/server/main.go
git commit -m "refactor: api.New принимает LLM-клиент (nil = выключен)"
```

---

### Task 6: API — хендлер `POST /api/generate/ai`

**Files:**
- Create: `internal/api/generate_ai.go`
- Test: `internal/api/ai_generate_test.go`

- [ ] **Step 1: Пишем падающие тесты**

`internal/api/ai_generate_test.go`:

```go
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
```

- [ ] **Step 2: Убеждаемся, что тесты падают**

Run: `go test ./internal/api/ -run TestGenerateAI -v`
Expected: FAIL — 404 на `/api/generate/ai` (маршрут не зарегистрирован)

- [ ] **Step 3: Регистрируем маршрут**

В `internal/api/api.go` в `register` добавить строку рядом с `/api/generate`:

```go
	mux.Handle("POST /api/generate", requireJSON(h.generate))
	mux.Handle("POST /api/generate/ai", requireJSON(h.generateAI))
```

- [ ] **Step 4: Пишем хендлер**

`internal/api/generate_ai.go`:

```go
// AI-генерация: LLM предлагает комбинации по статистике архива.
package api

import (
	"errors"
	"log/slog"
	"net/http"

	"fortunata/internal/generate"
	"fortunata/internal/llm"
)

// maxAICount — верхняя граница для AI: длинные ответы LLM ломаются чаще.
const maxAICount = 20

func (h *Handler) generateAI(w http.ResponseWriter, r *http.Request) {
	if h.llm == nil {
		errorJSON(w, http.StatusServiceUnavailable, "AI генерация не настроена")
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Count < 1 || req.Count > maxAICount {
		errorJSON(w, http.StatusBadRequest, "Количество билетов — от 1 до 20")
		return
	}
	draws, err := h.st.List()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось прочитать базу розыгрышей")
		return
	}
	freq := make([]generate.DrawFreq, len(draws))
	for i, d := range draws {
		freq[i] = generate.DrawFreq{Numbers: d.Numbers, Bonus: d.Bonus}
	}
	mainFreq, bonusFreq := generate.Frequencies(freq)
	tickets, err := h.llm.Propose(r.Context(), req.Count, mainFreq, bonusFreq, len(draws))
	if err != nil {
		if errors.Is(err, llm.ErrUnavailable) {
			errorJSON(w, http.StatusBadGateway, "LLM-сервер недоступен")
			return
		}
		errorJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(tickets) == 0 {
		errorJSON(w, http.StatusBadGateway, "LLM не смог предложить корректные комбинации")
		return
	}
	if len(tickets) < req.Count {
		slog.Warn("generate-ai: недобор комбинаций", "запрошено", req.Count, "получено", len(tickets))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}
```

- [ ] **Step 5: Тесты зелёные**

Run: `go test ./internal/api/ -v`
Expected: PASS — новые и все существующие тесты API

- [ ] **Step 6: Коммит**

```bash
git add internal/api/generate_ai.go internal/api/ai_generate_test.go internal/api/api.go
git commit -m "feat: POST /api/generate/ai — комбинации от LLM с добором"
```

---

### Task 7: `GET /api/version` — флаг `ai`

**Files:**
- Modify: `internal/api/version.go`
- Test: `internal/api/version_test.go`

- [ ] **Step 1: Дополняем тест**

В `internal/api/version_test.go` заменить декодирование и проверки на:

```go
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
```

- [ ] **Step 2: Убеждаемся, что тест падает**

Run: `go test ./internal/api/ -run TestGetVersion -v`
Expected: FAIL — `ai` отсутствует в ответе, декодируется как false… поле `data.AI` будет false → тест может пройти. Проверка: `curl`-подобно убедиться, что поле реально появилось, нельзя. Поэтому шаг 2 — запустить тест, зафиксировать PASS, и реализацию проверить код-ревью флага в JSON (Step 3 добавляет поле — после этого тест осмыслен). Дополнительно изменить тест: временно `if _, ok := ...` нельзя на декодированной структуре. Оставляем как есть: тест фиксирует контракт «ai: false без конфига», реализация добавляет поле.

Run: `go test ./internal/api/ -run TestGetVersion -v`
Expected: PASS (текущая реализация отдаёт только version — тест это и проверяет)

- [ ] **Step 3: Добавляем поле в ответ**

`internal/api/version.go`:

```go
// getVersion — публичная ручка: версия сборки для футера и флаг
// настроенности AI-генерации для главной страницы.
func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": version.Version, "ai": h.llm != nil})
}
```

- [ ] **Step 4: Тесты зелёные**

Run: `go test ./internal/api/ -run TestGetVersion -v`
Expected: PASS

- [ ] **Step 5: Коммит**

```bash
git add internal/api/version.go internal/api/version_test.go
git commit -m "feat: флаг ai в /api/version — настроен ли LLM"
```

---

### Task 8: `cmd/server` — конфиг и таймауты

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/server/main_test.go`
- Modify: `internal/api/sync_test.go:270` (константа инварианта)

- [ ] **Step 1: Пишем падающие тесты**

В `cmd/server/main_test.go` добавить:

```go
func TestLoadConfigLLMPair(t *testing.T) {
	env := map[string]string{
		"ADMIN_PASSWORD": "pass",
		"LLM_BASE_URL":   "http://192.168.1.128:18020",
		"LLM_MODEL":      "qwen3.8-27b",
		"LLM_API_KEY":    "secret",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMBaseURL != "http://192.168.1.128:18020" || cfg.LLMModel != "qwen3.8-27b" || cfg.LLMAPIKey != "secret" {
		t.Fatalf("LLM-конфиг: %+v", cfg)
	}
}

func TestLoadConfigLLMHalfConfigured(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		if k == "LLM_BASE_URL" {
			return "http://x"
		}
		return ""
	}
	_, err := loadConfig(getenv)
	if err == nil || !strings.Contains(err.Error(), "LLM_BASE_URL") {
		t.Fatalf("ожидали ошибку пары LLM_*, got %v", err)
	}
}

func TestLoadConfigLLMOffByDefault(t *testing.T) {
	getenv := func(k string) string {
		if k == "ADMIN_PASSWORD" {
			return "pass"
		}
		return ""
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMBaseURL != "" || cfg.LLMModel != "" || cfg.LLMAPIKey != "" {
		t.Fatalf("LLM должен быть выключен: %+v", cfg)
	}
}
```

- [ ] **Step 2: Убеждаемся, что тесты падают**

Run: `go test ./cmd/server/ -v`
Expected: FAIL — `cfg.LLMBaseURL undefined`

- [ ] **Step 3: Конфиг**

В `cmd/server/main.go` дополнить структуру `Config`:

```go
// Config — все настройки приложения, читаемые из окружения.
type Config struct {
	Addr         string // ADDR, по умолчанию ":8080"
	Password     string // ADMIN_PASSWORD, обязателен
	Secret       string // SESSION_SECRET; если пуст — случайный
	SecretRandom bool   // true, если SECRET сгенерирован при старте
	CookieSecure bool   // COOKIE_SECURE, по умолчанию false (за Traefik ставят true)
	DBPath       string // DB_PATH, по умолчанию "fortunata.db"
	LLMBaseURL   string // LLM_BASE_URL; задаются с LLMModel вместе, пусто = AI выключена
	LLMModel     string // LLM_MODEL
	LLMAPIKey    string // LLM_API_KEY, опционален
}
```

В конец `loadConfig` (после блока `cfg.CookieSecure = b`) добавить:

```go
	cfg.LLMBaseURL = getenv("LLM_BASE_URL")
	cfg.LLMModel = getenv("LLM_MODEL")
	cfg.LLMAPIKey = getenv("LLM_API_KEY")
	if (cfg.LLMBaseURL == "") != (cfg.LLMModel == "") {
		return Config{}, errors.New("LLM_BASE_URL и LLM_MODEL задаются вместе")
	}
```

- [ ] **Step 4: Сборка клиента и WriteTimeout**

В `cmd/server/main.go` в `main()` заменить строку 87 и добавить сборку клиента:

```go
	// LLM-клиент для AI-генерации; nil — фича выключена.
	var llmClient *llm.Client
	if cfg.LLMBaseURL != "" {
		llmClient = llm.NewClient(cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMAPIKey, nil)
		logger.Info("AI-генерация включена", "url", cfg.LLMBaseURL, "model", cfg.LLMModel)
	}

	mux := api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure, "", llmClient)
```

Импорты дополнить `"fortunata/internal/llm"`.

В конструкторе `http.Server` заменить `WriteTimeout: 15 * time.Second` на:

```go
		// WriteTimeout должен вмещать запрос к LLM (120 с в internal/llm),
		// иначе ответ AI-генерации умрёт при записи.
		WriteTimeout: 130 * time.Second,
```

- [ ] **Step 5: Инвариант таймаута архива**

В `internal/api/sync_test.go:270` заменить:

```go
	const writeTimeout = 15 * time.Second // WriteTimeout в cmd/server/main.go
```

на:

```go
	const writeTimeout = 130 * time.Second // WriteTimeout в cmd/server/main.go
```

- [ ] **Step 6: Все тесты зелёные**

Run: `go test ./...`
Expected: PASS во всех пакетах

- [ ] **Step 7: Коммит**

```bash
git add cmd/server/main.go cmd/server/main_test.go internal/api/sync_test.go
git commit -m "feat: конфиг LLM_* и WriteTimeout 130с для AI-генерации"
```

---

### Task 9: Фронтенд

**Files:**
- Modify: `web/index.html:24`
- Modify: `web/app.js`

- [ ] **Step 1: Кнопка в разметке**

В `web/index.html` после кнопки `#btn-generate` добавить:

```html
        <button id="btn-generate" class="btn-primary">Сгенерировать</button>
        <button id="btn-generate-ai" class="btn-primary" hidden>AI генерация</button>
```

(строку с `btn-generate` не менять, добавить только `btn-generate-ai`).

- [ ] **Step 2: Логика в app.js**

Блок «Генерация» (строки 24–47 `web/app.js`) заменить целиком на:

```js
// Генерация
const genError = document.getElementById('gen-error');
const ticketsBox = document.getElementById('tickets');
const countInput = document.getElementById('count');
const btnAI = document.getElementById('btn-generate-ai');

// renderTickets рисует пачку билетов в #tickets (общий для обеих кнопок).
function renderTickets(tickets) {
  ticketsBox.replaceChildren();
  for (const t of tickets) {
    const row = document.createElement('div');
    row.className = 'ticket';
    for (const n of t.numbers) {
      row.append(ball(n));
    }
    row.append(ball(t.bonus, 'bonus'));
    ticketsBox.append(row);
  }
}

function showGenError(e) {
  genError.textContent = e.message;
  genError.hidden = false;
}

document.getElementById('btn-generate').addEventListener('click', async () => {
  genError.hidden = true;
  try {
    const data = await api('/api/generate', { method: 'POST', body: { count: Number(countInput.value) } });
    renderTickets(data.tickets);
  } catch (e) {
    showGenError(e);
  }
});

btnAI.addEventListener('click', async () => {
  genError.hidden = true;
  btnAI.disabled = true;
  btnAI.textContent = 'AI думает…';
  countInput.disabled = true;
  try {
    const data = await api('/api/generate/ai', { method: 'POST', body: { count: Number(countInput.value) } });
    renderTickets(data.tickets);
  } catch (e) {
    showGenError(e);
  } finally {
    btnAI.disabled = false;
    btnAI.textContent = 'AI генерация';
    countInput.disabled = false;
  }
});

// AI-кнопка видна, только если сервер настроил LLM. Запрос после
// навешивания обработчиков: top-level await не задержит их.
try {
  const v = await api('/api/version');
  if (v && v.ai) {
    btnAI.hidden = false;
  }
} catch {
  // /api/version недоступен — кнопка остаётся скрытой
}
```

- [ ] **Step 3: Синтаксис и тесты**

Run: `node --test web/parse.test.mjs && node --check web/app.js`
Expected: тесты PASS, `--check` без ошибок

- [ ] **Step 4: Коммит**

```bash
git add web/index.html web/app.js
git commit -m "feat: кнопка AI-генерации на главной (видна при настроенном LLM)"
```

---

### Task 10: Docker и документация

**Files:**
- Modify: `compose.yaml`, `compose.local.yaml`, `.env.example`, `README.md`, `CLAUDE.md`

- [ ] **Step 1: compose.yaml**

В блок `environment:` сервиса `fortunata` добавить после `ADDR: ":8080"`:

```yaml
      LLM_BASE_URL: ${LLM_BASE_URL:-}
      LLM_MODEL: ${LLM_MODEL:-}
      LLM_API_KEY: ${LLM_API_KEY:-}
```

- [ ] **Step 2: compose.local.yaml**

В блок `environment:` добавить:

```yaml
      LLM_BASE_URL: ${LLM_BASE_URL:-http://192.168.1.128:18020}
      LLM_MODEL: ${LLM_MODEL:-qwen3.8-27b}
      LLM_API_KEY: ${LLM_API_KEY:-}
```

- [ ] **Step 3: .env.example**

В конец добавить:

```bash
# LLM-сервер (llama.cpp) для AI-генерации комбинаций. BASE_URL и MODEL
# задаются вместе; пустые значения выключают функцию. Клиент сам добавляет
# /v1/chat/completions к BASE_URL.
#LLM_BASE_URL=http://192.168.1.128:18020
#LLM_MODEL=qwen3.8-27b
# Bearer-токен LLM-сервера (опционален)
#LLM_API_KEY=
```

- [ ] **Step 4: README**

В таблицу «Переменные окружения» после строки `COOKIE_SECURE` добавить:

```markdown
| `LLM_BASE_URL` | База llama.cpp-сервера; задаётся вместе с `LLM_MODEL`, пусто — AI-генерация выключена | *(пусто)* |
| `LLM_MODEL` | Имя модели на LLM-сервере | *(пусто)* |
| `LLM_API_KEY` | Bearer-токен LLM-сервера | *(пусто)* |
```

- [ ] **Step 5: CLAUDE.md**

В раздел «Архитектура» после строки про `internal/generate/` добавить:

```markdown
- `internal/llm/` — клиент llama.cpp (OpenAI-совместимый) для AI-генерации:
  англоязычный промт, парсинг ответа (`<think>`, заборы), валидация и до 2
  доборов невалидных комбинаций
```

В разделе «Окружение» строку «Остальные: …» дополнить: `LLM_BASE_URL` + `LLM_MODEL` (задаются вместе; пусто = AI-генерация выключена), `LLM_API_KEY` (опционален).

- [ ] **Step 6: Коммит**

```bash
git add compose.yaml compose.local.yaml .env.example README.md CLAUDE.md
git commit -m "docs: конфигурация LLM_* для AI-генерации"
```

---

### Task 11: ручная проверка с реальным LLM (выполняет main-сессия, не субагент)

> Предусловие: пользователь создал `.env` с `LLM_BASE_URL`, `LLM_MODEL`, `LLM_API_KEY`.

- [ ] **Step 1: Полный тест-прогон**

```bash
go test ./... && node --test web/parse.test.mjs
```
Expected: всё PASS

- [ ] **Step 2: Запуск с реальным сервером**

```bash
set -a && source .env && set +a && ADMIN_PASSWORD=test go run ./cmd/server &
sleep 2
curl -s -X POST http://localhost:8080/api/generate/ai -H 'Content-Type: application/json' -d '{"count":3}'
```
Expected: 200 с `{"tickets":[...3 валидных билета...]}`; в логах — «AI-генерация включена».

- [ ] **Step 3: Ошибка недоступного LLM**

```bash
ADMIN_PASSWORD=test ADDR=:8081 LLM_BASE_URL=http://127.0.0.1:1 LLM_MODEL=m go run ./cmd/server &
sleep 2
curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8081/api/generate/ai -H 'Content-Type: application/json' -d '{"count":1}'
```
Expected: 502.

- [ ] **Step 4: UI**

Открыть `http://localhost:8080` в браузере: кнопка «AI генерация» видна, после клика — «AI думает…», затем билеты отрисованы как у обычной генерации.

- [ ] **Step 5: Финализация**

Остановить тестовые серверы. Согласно памяти пользователя: отправить ветку и открыть PR (`finish-with-pr`).
