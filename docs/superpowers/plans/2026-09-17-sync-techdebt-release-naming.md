# Техдолг синхронизации (#7) + именование релизов — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть все 9 открытых пунктов issue #7 (парсер, API, UI) и добавить время в имя релиза (`v2026.09.17-HHMM-хеш`).

**Architecture:** Правки трёх зон: чистый парсер `internal/timelottery` (неоднозначная колонка + юнит-тесты), хендлер `internal/api` (рефакторинг зависимости клиента + два новых теста + ограничитель ридера), UI `web/admin.js` (причина в итогах + live-region). Отдельно — одна строка в release-workflow и синхронизация двух спек.

**Tech Stack:** Go 1.26, `golang.org/x/net/html`, SQLite (`modernc.org/sqlite`), ванильный JS, GitHub Actions.

**Спека:** `docs/superpowers/specs/2026-09-17-sync-techdebt-release-naming-design.md`

---

## Подготовка

- [ ] **Шаг 0: создать ветку**

```bash
git checkout -b fix/issue-7-sync-techdebt
```

(Если работа идёт в worktree, созданном через using-git-worktrees, — ветка уже создана, шаг пропустить.)

---

### Task 1: Парсер — неоднозначная колонка с восемью числами

**Files:**
- Create: `internal/timelottery/testdata/ambiguous.html`
- Modify: `internal/timelottery/timelottery.go`
- Test: `internal/timelottery/timelottery_test.go`

- [ ] **Шаг 1.1: создать фикстуру со второй 8-числовой ячейкой**

Файл `internal/timelottery/testdata/ambiguous.html`:

```html
<html><body><table>
<tr><th>№</th><th>Дата</th><th>Числа</th><th>Проверка</th><th>Приз</th></tr>
<tr><td>64</td><td>14 сент</td><td><strong>19, 28, 24, 21, 10, 29, 05 и 18</strong></td><td>19-28-24-21-10-29-05-18</td><td>10 млн</td></tr>
</table></body></html>
```

Ячейка «Проверка» гипотетического редизайна содержит ровно восемь чисел → в строке две 8-числовые ячейки.

- [ ] **Шаг 1.2: написать падающий тест**

В `internal/timelottery/timelottery_test.go` после `TestParseIssues` добавить:

```go
// Две ячейки с восемью числами в одной строке — неоднозначность:
// issue вместо молчаливого «взять первую».
func TestParseAmbiguousRow(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader(readFixture(t, "ambiguous.html")))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(draws) != 0 {
		t.Fatalf("draws = %+v, хотели пусто", draws)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %+v, хотели 1", issues)
	}
	if issues[0].DrawNo != 64 || !strings.Contains(issues[0].Reason, "неоднозначно") {
		t.Fatalf("issue = %+v", issues[0])
	}
}
```

- [ ] **Шаг 1.3: убедиться, что тест падает**

```bash
go test ./internal/timelottery/ -run TestParseAmbiguousRow -v
```

Ожидание: FAIL — `draws = [No:64 ...]`, хотели пусто (парсер берёт первую ячейку).

- [ ] **Шаг 1.4: изменить `numbersCell` и `Parse`**

В `internal/timelottery/timelottery.go` заменить `numbersCell` (сейчас возвращает `([]int, bool)`):

```go
// numbersCell возвращает первую из ячеек с ровно восемью числами
// (семёрка + бонус); count — сколько таких ячеек всего.
func numbersCell(cells []string) (nums []int, count int) {
	for _, text := range cells {
		ns := extractNumbers(text)
		if len(ns) == 8 {
			if count == 0 {
				nums = ns
			}
			count++
		}
	}
	return nums, count
}
```

В `Parse` заменить:

```go
		nums, ok := numbersCell(cells[1:])
		if !ok {
			continue // не похоже на строку данных
		}
```

на:

```go
		nums, count := numbersCell(cells[1:])
		if count == 0 {
			continue // не похоже на строку данных
		}
		if count > 1 {
			issues = append(issues, Issue{
				DrawNo: no,
				Reason: fmt.Sprintf("неоднозначно: найдено %d ячеек с восемью числами", count),
			})
			continue
		}
```

Обновить комментарий пакета (строки 4–8) — фразу про строку данных:

```go
// Разбор не привязан к классам, стилям, порядку колонок и номеру таблицы:
// строкой данных считается <tr>, у которого первая ячейка — целое число ≥ 1,
// а среди остальных ровно одна ячейка с восемью числами (семёрка + бонус);
// если таких ячеек несколько — строка уходит в Issue (неоднозначность).
// У остальных ячеек такой плотности цифр не бывает: дата «14 сент» → 1 число,
// «10 млн» → 1, «93,3 млн» → 2, «архив (#64)» → 1.
```

- [ ] **Шаг 1.5: убедиться, что все тесты парсера проходят**

```bash
go test ./internal/timelottery/ -v
```

Ожидание: PASS, включая `TestParseAmbiguousRow`, `TestParseHappyPath`, `TestParseIssues`, `TestParseStructuralError`.

- [ ] **Шаг 1.6: коммит**

```bash
git add internal/timelottery/
git commit -m "feat: неоднозначная колонка с восемью числами — Issue вместо первой ячейки"
```

---

### Task 2: Парсер — прямые юнит-тесты `extractNumbers` / `validate`

**Files:**
- Test: `internal/timelottery/timelottery_test.go`

Тестируется существующий код — это фиксация контракта (characterization tests), поэтому шаг «падение» отсутствует: тесты должны пройти сразу. Если какой-то кейс падает — это дефект понимания, сначала разобраться, потом править.

- [ ] **Шаг 2.1: написать тесты**

В конец `internal/timelottery/timelottery_test.go`:

```go
func TestExtractNumbers(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []int
	}{
		{"дата", "14 сент", []int{14}},
		{"приз с запятой", "93,3 млн", []int{93, 3}},
		{"юникод-тире", "1–2", []int{1, 2}},
		{"неразрывный пробел", "7\u00a011", []int{7, 11}},
		{"цифры в конце строки", "7 и 11", []int{7, 11}},
		{"комбинация целиком", "19, 28, 24, 21, 10, 29, 05 и 18", []int{19, 28, 24, 21, 10, 29, 5, 18}},
		// 20 цифр: Atoi ошибается и молча даёт 0 — задокументированное
		// поведение; в реальной строке такой артефакт отсекается validate.
		{"переполнение Atoi", "99999999999999999999", []int{0}},
		{"нет цифр", "архив", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractNumbers(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("extractNumbers(%q) = %v, хотели %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	ok := []int{5, 10, 19, 21, 24, 28, 29}
	for _, tc := range []struct {
		name  string
		main  []int
		bonus int
		fragm string // "" — валидна
	}{
		{"валидна", ok, 18, ""},
		{"число вне диапазона", []int{0, 10, 19, 21, 24, 28, 29}, 18, "0 вне диапазона"},
		{"число больше 35", []int{5, 10, 19, 21, 24, 28, 36}, 18, "36 вне диапазона"},
		{"повтор", []int{5, 5, 19, 21, 24, 28, 29}, 18, "повторяется"},
		{"бонус вне диапазона", ok, 55, "бонусное число 55 вне диапазона"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := validate(tc.main, tc.bonus)
			if tc.fragm == "" {
				if msg != "" {
					t.Fatalf("validate(%v, %d) = %q, хотели \"\"", tc.main, tc.bonus, msg)
				}
				return
			}
			if !strings.Contains(msg, tc.fragm) {
				t.Fatalf("validate(%v, %d) = %q, хотели подстроку %q", tc.main, tc.bonus, msg, tc.fragm)
			}
		})
	}
}
```

- [ ] **Шаг 2.2: запустить**

```bash
go test ./internal/timelottery/ -run 'TestExtractNumbers|TestValidate' -v
```

Ожидание: PASS по всем подтестам.

- [ ] **Шаг 2.3: коммит**

```bash
git add internal/timelottery/timelottery_test.go
git commit -m "test: прямые юнит-тесты extractNumbers и validate"
```

---

### Task 3: Парсер — `TestParseIssues` в подтесты

**Files:**
- Test: `internal/timelottery/timelottery_test.go`

- [ ] **Шаг 3.1: переписать `TestParseIssues`**

Заменить функцию целиком (добавляется импорт `fmt`):

```go
func TestParseIssues(t *testing.T) {
	draws, issues, err := Parse(strings.NewReader(readFixture(t, "broken.html")))
	if err != nil {
		// Все строки похожи на данные — это не структурный отказ.
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(draws) != 0 {
		t.Fatalf("draws = %+v, хотели пусто", draws)
	}
	if len(issues) != 4 {
		t.Fatalf("issues = %+v, хотели 4", issues)
	}
	for _, tc := range []struct {
		no    int64
		fragm string
	}{
		{62, "36 вне диапазона"},
		{61, "повторяется"},
		{60, "бонусное число 55 вне диапазона"},
		{59, "0 вне диапазона"},
	} {
		t.Run(fmt.Sprintf("№%d", tc.no), func(t *testing.T) {
			found := false
			for _, is := range issues {
				if is.DrawNo == tc.no && strings.Contains(is.Reason, tc.fragm) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("нет issue для №%d с «%s»: %+v", tc.no, tc.fragm, issues)
			}
		})
	}
}
```

К блоку импортов добавить `"fmt"`.

- [ ] **Шаг 3.2: запустить**

```bash
go test ./internal/timelottery/ -run TestParseIssues -v
```

Ожидание: PASS, в выводе 4 подтеста (`№62`, `№61`, `№60`, `№59`).

- [ ] **Шаг 3.3: коммит**

```bash
git add internal/timelottery/timelottery_test.go
git commit -m "test: TestParseIssues — подтесты t.Run по кейсам"
```

---

### Task 4: API — `archiveClient` становится полем `Handler`

**Files:**
- Modify: `internal/api/api.go`
- Modify: `internal/api/sync.go`

Чистый рефакторинг без изменения поведения: все существующие тесты проходят до и после.

- [ ] **Шаг 4.1: правки в `internal/api/api.go`**

Добавить импорт `"time"`. Заменить структуру `Handler`:

```go
type Handler struct {
	st            *store.Store
	auth          *auth.Manager
	cookieSecure  bool
	archiveURL    string       // источник синхронизации; переопределяется в тестах
	archiveClient *http.Client // клиент скачивания архива; подменяется в тестах
}
```

Заменить `New` на `newHandler` + `register` + `New`:

```go
// newHandler собирает Handler; маршруты регистрирует register.
// Таймаут клиента должен оставаться меньше WriteTimeout HTTP-сервера
// (15 с в cmd/server/main.go), иначе вставки закоммитятся, а ответ
// до клиента не дойдёт.
func newHandler(st *store.Store, password, secret string, cookieSecure bool, archiveURL string) *Handler {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	return &Handler{
		st:            st,
		auth:          auth.New(password, secret),
		cookieSecure:  cookieSecure,
		archiveURL:    archiveURL,
		archiveClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
// Пустой archiveURL заменяется на defaultArchiveURL.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string) *http.ServeMux {
	h := newHandler(st, password, secret, cookieSecure, archiveURL)
	mux := http.NewServeMux()
	h.register(mux)
	return mux
}

// register регистрирует все /api-маршруты на mux.
func (h *Handler) register(mux *http.ServeMux) {
	mux.Handle("POST /api/login", requireJSON(h.login))
	mux.Handle("POST /api/logout", requireJSON(h.logout))
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("GET /api/draws", h.listDraws)
	mux.Handle("POST /api/draws", h.session(requireJSON(h.createDraw)))
	mux.Handle("PUT /api/draws/{no}", h.session(requireJSON(h.updateDraw)))
	mux.Handle("DELETE /api/draws/{no}", h.session(h.deleteDraw))
	mux.Handle("POST /api/sync", h.session(requireJSON(h.syncDraws)))
	mux.Handle("POST /api/generate", requireJSON(h.generate))
}
```

- [ ] **Шаг 4.2: правки в `internal/api/sync.go`**

Удалить `var archiveClient` вместе с комментарием (строки 17–20; комментарий переехал в `newHandler`). Удалить `"time"` из импортов — больше не используется.

Заменить вызов в `syncDraws`:

```go
	draws, issues, err := h.fetchArchive(h.archiveURL)
```

Заменить `fetchArchive` на метод:

```go
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
	return timelottery.Parse(io.LimitReader(resp.Body, maxArchiveBytes))
}
```

- [ ] **Шаг 4.3: собрать и прогнать все тесты**

```bash
go build ./... && go test ./...
```

Ожидание: компиляция без ошибок, все тесты PASS (поведение не изменилось).

- [ ] **Шаг 4.4: коммит**

```bash
git add internal/api/
git commit -m "refactor: archiveClient — явное поле Handler вместо package-level var"
```

---

### Task 5: API — тест таймаута скачивания архива

**Files:**
- Test: `internal/api/sync_test.go`

- [ ] **Шаг 5.1: добавить хелпер `newTestStore` и использовать его в `newSyncServer`**

В `internal/api/sync_test.go` перед `newSyncServer`:

```go
// newTestStore открывает in-memory стор; закрытие через t.Cleanup.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
```

В `newSyncServer` заменить блок `st, err := store.Open(":memory:") ... t.Cleanup(func() { st.Close() })` на:

```go
	st := newTestStore(t)
```

- [ ] **Шаг 5.2: написать тест таймаута**

В конец `internal/api/sync_test.go` (добавить импорт `"time"`):

```go
// Таймаут скачивания архива: медленный апстрим при коротком клиенте → 502.
// Возможен благодаря инъекции h.archiveClient (рефакторинг newHandler).
// Сон апстрима (500 мс) на порядок больше таймаута клиента (100 мс) —
// детерминированно; t.Cleanup дождётся сна, итого тест ~0,6 с.
func TestSyncArchiveTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	t.Cleanup(upstream.Close)

	h := newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL)
	h.archiveClient = &http.Client{Timeout: 100 * time.Millisecond}
	mux := http.NewServeMux()
	h.register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, хотим 502", resp.StatusCode)
	}
}
```

- [ ] **Шаг 5.3: запустить**

```bash
go test ./internal/api/ -run TestSyncArchiveTimeout -v
```

Ожидание: PASS, тест занимает ~100 мс (не 2 с).

- [ ] **Шаг 5.4: коммит**

```bash
git add internal/api/sync_test.go
git commit -m "test: таймаут скачивания архива — 502 при медленном апстриме"
```

---

### Task 6: API — страница больше лимита: 502 вместо тихой обрезки

**Files:**
- Modify: `internal/api/sync.go`
- Test: `internal/api/sync_test.go`

- [ ] **Шаг 6.1: написать падающий тест**

В конец `internal/api/sync_test.go`:

```go
// Страница архива больше лимита — 502, а не тихая обрезка с частичным
// парсом: после первых 5 МБ есть ещё валидная строка, LimitReader её бы
// потерял и вернул 200 с added=1.
func TestSyncOversizeArchive(t *testing.T) {
	page := "<html><body><table>" +
		"<tr><td>64</td><td>14 сент</td><td><strong>19, 28, 24, 21, 10, 29, 05 и 18</strong></td><td>10 млн</td></tr>" +
		strings.Repeat("<!-- отступ -->", maxArchiveBytes/15+1) +
		"<tr><td>63</td><td>13 сент</td><td><strong>11, 12, 16, 28, 29, 31, 35 и 14</strong></td><td>9 млн</td></tr>" +
		"</table></body></html>"
	ts := newSyncServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
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
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Error, "больше 5 МБ") {
		t.Fatalf("error = %q", body.Error)
	}
}
```

- [ ] **Шаг 6.2: убедиться, что тест падает**

```bash
go test ./internal/api/ -run TestSyncOversizeArchive -v
```

Ожидание: FAIL — `status = 200, хотим 502` (LimitReader тихо обрезает, первая строка парсится).

- [ ] **Шаг 6.3: реализовать `limitedReader`**

В `internal/api/sync.go` после `maxArchiveBytes` добавить:

```go
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
```

Заменить тело возврата `fetchArchive` (последняя строка метода):

```go
	draws, issues, err := timelottery.Parse(&limitedReader{r: resp.Body, n: maxArchiveBytes})
	if err != nil {
		if errors.Is(err, errArchiveTooBig) {
			return nil, nil, errArchiveTooBig // без префикса «разбор html:»
		}
		return nil, nil, err
	}
	return draws, issues, nil
```

- [ ] **Шаг 6.4: убедиться, что тест проходит**

```bash
go test ./internal/api/ -v
```

Ожидание: PASS все, включая `TestSyncOversizeArchive`; остальные sync-тесты не сломаны (реальные страницы << 5 МБ).

- [ ] **Шаг 6.5: коммит**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "fix: страница архива больше лимита — 502 вместо тихой обрезки"
```

---

### Task 7: API — тест ветки ошибки БД в `syncDraws`

**Files:**
- Test: `internal/api/sync_test.go`

- [ ] **Шаг 7.1: написать тест**

В конец `internal/api/sync_test.go`:

```go
// Ветка ошибки БД в syncDraws: закрытый стор даёт issues «ошибка сохранения»
// с 200. Логин в БД не ходит, поэтому сессию открываем до Close.
func TestSyncStoreClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readArchiveFixture(t)))
	}))
	t.Cleanup(upstream.Close)

	h := newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL)
	mux := http.NewServeMux()
	h.register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	c := loginClient(t, ts)
	h.st.Close() // провоцируем ошибку вставки

	resp := post(t, c, ts.URL+"/api/sync", map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, хотим 200", resp.StatusCode)
	}
	var res struct {
		Added  int `json:"added"`
		Issues []struct {
			DrawNo int64  `json:"drawNo"`
			Reason string `json:"reason"`
		} `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || len(res.Issues) != 2 { // в archive.html два розыгрыша
		t.Fatalf("res = %+v", res)
	}
	for _, is := range res.Issues {
		if is.Reason != "ошибка сохранения" {
			t.Fatalf("issue = %+v", is)
		}
	}
}
```

- [ ] **Шаг 7.2: запустить**

```bash
go test ./internal/api/ -run TestSyncStoreClosed -v
```

Ожидание: PASS (ветка `default` в `syncDraws` уже реализована; тест фиксирует её контракт).

- [ ] **Шаг 7.3: коммит**

```bash
git add internal/api/sync_test.go
git commit -m "test: ветка ошибки БД в syncDraws — issues «ошибка сохранения»"
```

---

### Task 8: UI — причина в итогах и канонический live-region

**Files:**
- Modify: `web/admin.js`

- [ ] **Шаг 8.1: заменить `syncSummary`**

```js
// syncSummary — краткий итог синхронизации одной строкой.
function syncSummary(res) {
  const issues = res.issues ?? [];
  if (res.added === 0 && issues.length === 0) {
    return 'Новых розыгрышей нет';
  }
  const parts = [`Добавлено ${res.added}, пропущено ${res.skipped}`];
  if (issues.length > 0) {
    // Парсер создаёт Issue только при drawNo ≥ 1 (timelottery.Parse), и
    // ошибки сохранения всегда с номером; «строка без номера» — защитный
    // контракт на случай изменения парсера.
    const names = issues.map((i) => (i.drawNo > 0 ? `№ ${i.drawNo} (${i.reason})` : 'строка без номера'));
    parts.push(`не удалось разобрать: ${names.join(', ')}`);
  }
  return parts.join('; ');
}
```

- [ ] **Шаг 8.2: заменить `sync`**

```js
async function sync() {
  syncStatus.hidden = true; // скрыть прошлый итог
  btnSync.disabled = true;
  btnSync.textContent = 'Синхронизация…';
  try {
    const res = await api('/api/sync', { method: 'POST', body: {} });
    // Канонический live-region: текст меняется в уже видимой области —
    // смена объявляется скринридером.
    syncStatus.hidden = false;
    syncStatus.textContent = syncSummary(res);
    syncStatus.className = 'hint';
  } catch (e) {
    syncStatus.hidden = false;
    syncStatus.textContent = e.message;
    syncStatus.className = 'error';
  } finally {
    btnSync.disabled = false;
    btnSync.textContent = 'Синхронизировать';
    refreshList();
  }
}
```

Отличие от прежнего: `syncStatus.hidden = false` переехал из `finally` в `try`/`catch` и выполняется **до** записи текста.

- [ ] **Шаг 8.3: регресс JS-тестов парсера и ручная проверка**

```bash
node --test web/parse.test.mjs
```

Ожидание: PASS. (`admin.js` без JS-тестов — норма репозитория, зафиксирована в issue; правку проверяют чтением: поменялись только две функции.)

- [ ] **Шаг 8.4: коммит**

```bash
git add web/admin.js
git commit -m "fix(ui): причина в итогах синхронизации и канонический live-region"
```

---

### Task 9: Именование релизов + синхронизация спек

**Files:**
- Modify: `.github/workflows/release.yml:29-30`
- Modify: `docs/superpowers/specs/2026-09-16-github-release-workflow-design.md:21,58`
- Modify: `docs/superpowers/specs/2026-09-17-timelottery-sync-design.md` (раздел «Алгоритм», п. 2)

- [ ] **Шаг 9.1: правка `release.yml`**

Заменить шаг «Версия»:

```yaml
      - name: Версия (CalVer + время + хеш)
        run: echo "VERSION=v$(git show -s --format='%cd' --date=format:'%Y.%m.%d-%H%M' HEAD)-$(git rev-parse --short HEAD)" >> "$GITHUB_ENV"
```

Было `--date=format:'%Y.%m.%d'` и имя шага «Версия (CalVer + хеш)». Итоговый формат: `v2026.09.17-0834-6ff9686`.

- [ ] **Шаг 9.2: правка спеки release-workflow (два места)**

Строку 21 заменить на:

```markdown
| Версия | CalVer: `v$(git show -s --format='%cd' --date=format:'%Y.%m.%d-%H%M' HEAD)-$(git rev-parse --short HEAD)` (дата и время из коммита), напр. `v2026.09.17-0834-6ff9686` |
```

Строку 58 заменить на:

```markdown
1. Версия: `VERSION="v$(git show -s --format='%cd' --date=format:'%Y.%m.%d-%H%M' HEAD)-$(git rev-parse --short HEAD)"` — дата и время (HHMM) из коммита, версия детерминирована; HHMM разносит релизы одного дня, чтобы список релизов сортировался хронологически.
```

- [ ] **Шаг 9.3: правка спеки синхронизации («Алгоритм», п. 2)**

Прочитать `docs/superpowers/specs/2026-09-17-timelottery-sync-design.md`, найти в разделе «Алгоритм» пункт 2, финал второго буллета:

```
     (7 основных + бонус); берётся первая такая ячейка — на реальной странице
     она ровно одна (финальное ревью: подсчёт совпадений признан избыточным,
     при гипотетическом появлении второй такой колонки парсер возьмёт первую,
     а валидация комбинации даст видимый `Issue`, а не тихую порчу).
```

заменить на:

```
     (7 основных + бонус). Ячейки с восемью числами подсчитываются: если
     их больше одной — строка уходит в `Issue` (неоднозначность), парсер
     не угадывает.
```

- [ ] **Шаг 9.4: коммит**

```bash
git add .github/workflows/release.yml docs/superpowers/specs/
git commit -m "fix(ci): время коммита (HHMM) в имени релиза для хронологической сортировки"
```

---

### Task 10: Финальная проверка

**Files:** без изменений (только команды)

- [ ] **Шаг 10.1: полный набор проверок**

```bash
gofmt -l . && go vet ./... && go test ./... && node --test web/parse.test.mjs
```

Ожидание: `gofmt -l` ничего не выводит (пустой список), vet молчит, все тесты PASS.

- [ ] **Шаг 10.2: сверка со спекой**

Пройти по `docs/superpowers/specs/2026-09-17-sync-techdebt-release-naming-design.md`: разделы 1–5 покрыты задачами 1–9; «Изменения поведения» (3 пункта) реализованы; новых файлов и правок сверх спеки нет.

Далее — интеграция ветки через superpowers:finishing-a-development-branch (PR с `Closes #7`; merge в main запустит release-workflow с новым форматом версии).
