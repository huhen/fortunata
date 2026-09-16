# Синхронизация розыгрышей с архивом timelottery.ru — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Кнопка «Синхронизировать» в админке: сервер скачивает страницу архива timelottery.ru, добавляет в базу только новые розыгрыши и показывает краткий итог.

**Architecture:** Новый чистый пакет `internal/timelottery` (HTML → `[]Draw` без сети и БД), эндпоинт `POST /api/sync` в `internal/api` (скачивание + вставка + сводка), кнопка и строка статуса в `web/admin.html` + `web/admin.js`. Разбор устойчив к редизайну: строки данных ищутся по содержимому ячеек, без привязки к классам, стилям, порядку колонок и номеру таблицы.

**Tech Stack:** Go 1.25, `golang.org/x/net/html`, SQLite (`modernc.org/sqlite`), ванильный JS. Спека: `docs/superpowers/specs/2026-09-17-timelottery-sync-design.md`.

**Тесты запускать из корня репозитория** `/home/usr1/coding/github/fortunata`.

---

### Task 1: Парсер `internal/timelottery` — happy path

**Files:**
- Create: `internal/timelottery/testdata/archive.html`
- Create: `internal/timelottery/timelottery_test.go`
- Create: `internal/timelottery/timelottery.go`
- Modify: `go.mod`, `go.sum` (новая зависимость `golang.org/x/net`)

- [ ] **Step 1: Создать фикстуру `internal/timelottery/testdata/archive.html`**

Вырезка реальной страницы: шапка таблицы, две строки данных (одна — точно как на сайте, другая — без стилей, чтобы доказать независимость от разметки), строка из соседней таблицы и пустая строка-сноска.

```html
<!doctype html>
<html><body>
<article>
<table>
<tr style="height: 48px">
<td style="width: 35.8636px;text-align: center;height: 48px"><strong>№</strong></td>
<td style="width: 64.4659px;text-align: center;height: 48px"><strong>Дата</strong></td>
<td style="width: 89.1477px;text-align: center;height: 48px"><strong>Выпавшие числа</strong></td>
<td style="width: 69.3239px;text-align: center;height: 48px"><strong>Джекпот ₽&nbsp;</strong></td>
<td style="width: 68.392px;text-align: center;height: 48px"><strong>Билетов</strong></td>
<td style="width: 61.8977px;text-align: center;height: 48px"><strong>Ссылка</strong></td>
</tr>
<tr>
<td style="width: 35.8636px;text-align: center">64</td>
<td style="width: 64.4659px;text-align: center">14 сент</td>
<td style="width: 89.1477px;text-align: center"><strong>19, 28, 24, 21, 10, 29, 05 и <span style="color: #ff00ff">18</span></strong></td>
<td style="width: 69.3239px;text-align: center">10 млн</td>
<td style="width: 68.392px;text-align: center">93,3 млн</td>
<td style="width: 61.8977px;text-align: center"><a href="https://timelottery.ru/arhiv/itogi-fortunaty-v-morkovske-tirazh-14-sentyabrya/">архив (#64)</a></td>
</tr>
<tr>
<td>63</td><td>10 сент</td><td><strong>29, 16, 11, 12, 31, 35, 28 и 14</strong></td><td>5 млн</td><td>67,0 млн</td><td><a href="https://timelottery.ru/arhiv/x63/">архив (#63)</a></td>
</tr>
</table>
<table>
<tr><td>Разыгранные джекпоты</td><td></td></tr>
<tr><td>64</td><td>победитель из Мокровска</td></tr>
</table>
<p>* Количество билетов оценочное.</p>
<table><tr><td>&nbsp;</td></tr></table>
</article>
</body></html>
```

- [ ] **Step 2: Написать падающий тест `internal/timelottery/timelottery_test.go`**

```go
// Тесты парсера архива timelottery.ru; фикстуры — вырезки реальной страницы.
package timelottery

import (
	"os"
	"reflect"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseHappyPath(t *testing.T) {
	draws, issues, err := Parse([]byte(readFixture(t, "archive.html")))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v, хотели пусто", issues)
	}
	want := []Draw{
		{No: 64, Numbers: []int{10, 19, 21, 24, 28, 29}, Bonus: 18},
		{No: 63, Numbers: []int{11, 12, 16, 28, 29, 31, 35}, Bonus: 14},
	}
	if len(draws) != len(want) {
		t.Fatalf("draws = %+v, хотели %d розыгрышей", draws, len(want))
	}
	for i := range want {
		if draws[i].No != want[i].No || !reflect.DeepEqual(draws[i].Numbers, want[i].Numbers) ||
			draws[i].Bonus != want[i].Bonus {
			t.Fatalf("draws[%d] = %+v, хотели %+v", i, draws[i], want[i])
		}
	}
}
```

- [ ] **Step 3: Убедиться, что тест падает (пакета ещё нет)**

Run: `go test ./internal/timelottery/`
Expected: FAIL — `matched no packages` / ошибка сборки: пакета `fortunata/internal/timelottery` ещё не существует.

- [ ] **Step 4: Добавить зависимость**

Run: `go get golang.org/x/net@latest && go mod tidy`
Expected: `go.mod` получает `golang.org/x/net` (чистый Go, от команды Go), команда завершается без ошибок.

- [ ] **Step 5: Написать реализацию `internal/timelottery/timelottery.go`**

```go
// Пакет timelottery: разбор страницы архива розыгрышей timelottery.ru
// (https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/).
//
// Разбор не привязан к классам, стилям, порядку колонок и номеру таблицы:
// строкой данных считается <tr>, у которого первая ячейка — целое число ≥ 1,
// а среди остальных есть ячейка с ровно восемью числами (семёрка + бонус).
// У остальных ячеек такой плотности цифр не бывает: дата «14 сент» → 1 число,
// «10 млн» → 1, «93,3 млн» → 2, «архив (#64)» → 1.
package timelottery

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Draw — розыгрыш, извлечённый из архива.
type Draw struct {
	No      int64 // номер розыгрыша
	Numbers []int // 7 основных чисел 1–35, по возрастанию
	Bonus   int   // бонусное число 1–54
}

// Issue — похожая на данные строка с невалидной комбинацией.
type Issue struct {
	DrawNo int64  // номер розыгрыша из первой ячейки
	Reason string // человекочитаемая причина
}

// Parse разбирает HTML архива: возвращает розыгрыши и проблемы. Ошибка —
// структурная: похожих на данные строк нет вовсе (сайт изменился).
func Parse(r io.Reader) ([]Draw, []Issue, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, nil, fmt.Errorf("разбор html: %w", err)
	}
	var draws []Draw
	for _, cells := range tableRows(doc) {
		d, ok := dataRow(cells)
		if !ok {
			continue // шапка, сноска или строка другой таблицы
		}
		draws = append(draws, d)
	}
	return draws, nil, nil
}

// dataRow распознаёт строку данных и разбирает её.
func dataRow(cells []string) (Draw, bool) {
	if len(cells) < 2 {
		return Draw{}, false
	}
	no, err := strconv.ParseInt(strings.TrimSpace(cells[0]), 10, 64)
	if err != nil || no < 1 {
		return Draw{}, false
	}
	for _, cellText := range cells[1:] {
		nums := extractNumbers(cellText)
		if len(nums) != 8 {
			continue
		}
		main := nums[:7] // сортировка на месте, nums[7] (бонус) не трогает
		sort.Ints(main)
		return Draw{No: no, Numbers: main, Bonus: nums[7]}, true
	}
	return Draw{}, false
}

// tableRows возвращает тексты ячеек каждой <tr> в порядке следования.
func tableRows(n *html.Node) [][]string {
	var rows [][]string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			rows = append(rows, rowCells(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return rows
}

// rowCells — тексты <td>/<th> строки в порядке следования.
func rowCells(tr *html.Node) []string {
	var out []string
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			out = append(out, nodeText(c))
		}
	}
	return out
}

// nodeText — конкатенация текстовых узлов поддерева (плоский текст ячейки).
func nodeText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}

// extractNumbers — все последовательности цифр в тексте как числа
// (разделителем считается любая подстрока без цифр: запятые, «и», пробелы).
func extractNumbers(s string) []int {
	var out []int
	start := -1 // байтовый индекс начала текущей группы цифр
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			n, _ := strconv.Atoi(s[start:i])
			out = append(out, n)
			start = -1
		}
	}
	if start >= 0 {
		n, _ := strconv.Atoi(s[start:])
		out = append(out, n)
	}
	return out
}
```

- [ ] **Step 6: Убедиться, что тест проходит**

Run: `go test ./internal/timelottery/`
Expected: `ok  fortunata/internal/timelottery`

- [ ] **Step 7: Закоммитить**

```bash
git add internal/timelottery go.mod go.sum
git commit -m "feat: парсер архива timelottery"
```

---

### Task 2: Парсер — валидация, Issue, структурная ошибка

**Files:**
- Create: `internal/timelottery/testdata/broken.html`
- Modify: `internal/timelottery/timelottery_test.go`
- Modify: `internal/timelottery/timelottery.go`

- [ ] **Step 1: Создать фикстуру `internal/timelottery/testdata/broken.html`**

Все четыре строки похожи на данные (первая ячейка — число, есть ячейка с 8 числами), но каждая невалидна.

```html
<!doctype html>
<html><body>
<table>
<tr><td>62</td><td>3 сент</td><td><strong>1, 2, 3, 4, 5, 36, 07 и 08</strong></td><td>5 млн</td></tr>
<tr><td>61</td><td>27 авг</td><td><strong>1, 2, 3, 4, 5, 06, 06 и 08</strong></td><td>5 млн</td></tr>
<tr><td>60</td><td>20 авг</td><td><strong>1, 2, 3, 4, 5, 06, 07 и 55</strong></td><td>5 млн</td></tr>
<tr><td>59</td><td>13 авг</td><td><strong>0, 2, 3, 4, 5, 06, 07 и 08</strong></td><td>5 млн</td></tr>
</table>
</body></html>
```

- [ ] **Step 2: Добавить падающие тесты в `internal/timelottery/timelottery_test.go`**

Дописать в конец файла:

```go
func TestParseIssues(t *testing.T) {
	draws, issues, err := Parse([]byte(readFixture(t, "broken.html")))
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
		no     int64
		fragm  string
	}{
		{62, "36 вне диапазона"},
		{61, "повторяется"},
		{60, "бонусное число 55 вне диапазона"},
		{59, "0 вне диапазона"},
	} {
		found := false
		for _, is := range issues {
			if is.DrawNo == tc.no && strings.Contains(is.Reason, tc.fragm) {
				found = true
			}
		}
		if !found {
			t.Errorf("нет issue для №%d с «%s»: %+v", tc.no, tc.fragm, issues)
		}
	}
}

func TestParseStructuralError(t *testing.T) {
	draws, issues, err := Parse([]byte("<html><body><p>Пусто</p></body></html>"))
	if err == nil {
		t.Fatalf("хотели структурную ошибку, получили draws=%+v issues=%+v", draws, issues)
	}
	if !strings.Contains(err.Error(), "не найдены результаты") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}
```

И добавить `"strings"` в блок `import` тестового файла.

- [ ] **Step 3: Убедиться, что тесты падают**

Run: `go test ./internal/timelottery/`
Expected: FAIL — `TestParseIssues` получает `draws` с розыгрышами (валидации нет), `TestParseStructuralError` получает `err == nil`.

- [ ] **Step 4: Реализовать валидацию и структурную ошибку**

В `internal/timelottery/timelottery.go`:

Добавить `"errors"` в импорт. Заменить `Parse` и `dataRow` на:

```go
// Parse разбирает HTML архива: возвращает розыгрыши и проблемы. Ошибка —
// структурная: похожих на данные строк нет вовсе (сайт изменился).
func Parse(r io.Reader) ([]Draw, []Issue, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, nil, fmt.Errorf("разбор html: %w", err)
	}
	var (
		draws  []Draw
		issues []Issue
	)
	for _, cells := range tableRows(doc) {
		if len(cells) < 2 {
			continue
		}
		no, err := strconv.ParseInt(strings.TrimSpace(cells[0]), 10, 64)
		if err != nil || no < 1 {
			continue // шапка, сноска или строка другой таблицы
		}
		nums, ok := numbersCell(cells[1:])
		if !ok {
			continue // не похоже на строку данных
		}
		main := nums[:7]
		if msg := validate(main, nums[7]); msg != "" {
			issues = append(issues, Issue{DrawNo: no, Reason: msg})
			continue
		}
		sort.Ints(main)
		draws = append(draws, Draw{No: no, Numbers: main, Bonus: nums[7]})
	}
	if len(draws) == 0 && len(issues) == 0 {
		return nil, nil, errors.New("на странице не найдены результаты розыгрышей")
	}
	return draws, issues, nil
}

// numbersCell ищет первую ячейку с ровно восемью числами (семёрка + бонус).
func numbersCell(cells []string) ([]int, bool) {
	for _, text := range cells {
		nums := extractNumbers(text)
		if len(nums) == 8 {
			return nums, true
		}
	}
	return nil, false
}

// validate — первые 7 чисел в 1–35 без повторов, бонус в 1–54;
// "" если комбинация валидна, иначе причина для Issue.
func validate(main []int, bonus int) string {
	seen := make(map[int]struct{}, 7)
	for _, n := range main {
		if n < 1 || n > 35 {
			return fmt.Sprintf("число %d вне диапазона 1–35", n)
		}
		if _, dup := seen[n]; dup {
			return fmt.Sprintf("число %d повторяется", n)
		}
		seen[n] = struct{}{}
	}
	if bonus < 1 || bonus > 54 {
		return fmt.Sprintf("бонусное число %d вне диапазона 1–54", bonus)
	}
	return ""
}
```

Удалить ставшую ненужной функцию `dataRow` целиком.

- [ ] **Step 5: Убедиться, что все тесты пакета проходят**

Run: `go test ./internal/timelottery/`
Expected: `ok  fortunata/internal/timelottery` (happy path из Task 1 остался зелёным — поведение не изменилось).

- [ ] **Step 6: Закоммитить**

```bash
git add internal/timelottery
git commit -m "feat: парсер — валидация комбинаций и структурная ошибка"
```

---

### Task 3: Эндпоинт `POST /api/sync`

**Files:**
- Create: `internal/api/sync.go`
- Create: `internal/api/sync_test.go`
- Modify: `internal/api/api.go:15-33` (Handler, New, маршрут)
- Modify: `cmd/server/main.go:87` (вызов `New` — новый параметр)
- Modify: `internal/api/api_test.go:25` (`newTestServer`), `internal/api/api_test.go:454` (`TestGenerateUsesHistory`)

- [ ] **Step 1: Обновить существующие вызовы `New` (иначе пакет не соберётся)**

В `internal/api/api_test.go:25`:

```go
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, ""))
```

В `internal/api/api_test.go:454` (тест `TestGenerateUsesHistory`):

```go
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false, ""))
```

В `cmd/server/main.go:87`:

```go
	mux := api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure, "")
```

- [ ] **Step 2: Написать падающие тесты `internal/api/sync_test.go`**

```go
// Тесты POST /api/sync: «архив» — отдельный httptest-сервер с фикстурой.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	want := []int{10, 19, 21, 24, 28, 29}
	if d64.Bonus != 18 || len(d64.Numbers) != 7 {
		t.Fatalf("d64 = %+v", d64)
	}
	for i, n := range want {
		if d64.Numbers[i] != n {
			t.Fatalf("numbers = %v", d64.Numbers)
		}
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
```

- [ ] **Step 3: Убедиться, что тесты падают (сборка ломается о 5-й параметр New)**

Run: `go test ./internal/api/`
Expected: FAIL — `too many arguments in call to New`: вызовы обновлены на 5 аргументов (Step 1), а сигнатура `New` ещё с четырьмя; роут `/api/sync` и хендлер не добавлены.

- [ ] **Step 4: Правки `internal/api/api.go`**

В `Handler` добавить поле, `New` — параметр и маршрут. Итоговый вид верха файла:

```go
// Пакет api: JSON API приложения.
package api

import (
	"encoding/json"
	"net/http"

	"fortunata/internal/auth"
	"fortunata/internal/store"
)

// maxBodyBytes — лимит тела запроса: на порядки больше реальных запросов.
const maxBodyBytes = 64 << 10

// defaultArchiveURL — страница архива результатов на timelottery.ru.
const defaultArchiveURL = "https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/"

type Handler struct {
	st           *store.Store
	auth         *auth.Manager
	cookieSecure bool
	archiveURL   string // источник синхронизации; переопределяется в тестах
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
// Пустой archiveURL заменяется на defaultArchiveURL.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string) *http.ServeMux {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	h := &Handler{st: st, auth: auth.New(password, secret), cookieSecure: cookieSecure, archiveURL: archiveURL}
	mux := http.NewServeMux()
	mux.Handle("POST /api/login", requireJSON(h.login))
	mux.Handle("POST /api/logout", requireJSON(h.logout))
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("GET /api/draws", h.listDraws)
	mux.Handle("POST /api/draws", h.session(requireJSON(h.createDraw)))
	mux.Handle("PUT /api/draws/{no}", h.session(requireJSON(h.updateDraw)))
	mux.Handle("DELETE /api/draws/{no}", h.session(h.deleteDraw))
	mux.Handle("POST /api/sync", h.session(requireJSON(h.syncDraws)))
	mux.Handle("POST /api/generate", requireJSON(h.generate))
	return mux
}
```

(Ниже по файлу всё без изменений.)

- [ ] **Step 5: Создать `internal/api/sync.go`**

```go
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
```

- [ ] **Step 6: Убедиться, что все тесты API проходят**

Run: `go test ./internal/api/`
Expected: `ok  fortunata/internal/api` — включая старые тесты (сигнатура `New` обновлена в Step 1).

- [ ] **Step 7: Убедиться, что весь бэкенд собирается и тестируется**

Run: `go build ./... && go vet ./...`
Expected: без вывода (успех).

- [ ] **Step 8: Закоммитить**

```bash
git add internal/api cmd/server
git commit -m "feat: эндпоинт POST /api/sync — добавление новых розыгрышей из архива"
```

---

### Task 4: Фронтенд — кнопка «Синхронизировать»

**Files:**
- Modify: `web/admin.html:34-39` (блок form-actions)
- Modify: `web/admin.js` (кнопка, статус, функция sync)

- [ ] **Step 1: Правки `web/admin.html`**

Заменить блок form-actions (строки 34–37) на:

```html
        <div class="form-actions">
          <button id="btn-save" class="btn-primary">Добавить</button>
          <button id="btn-cancel" class="btn-secondary" hidden>Отмена</button>
          <button id="btn-sync" class="btn-secondary">Синхронизировать</button>
        </div>
        <p id="sync-status" class="hint" hidden></p>
```

- [ ] **Step 2: Правки `web/admin.js`**

После строки `const list = document.getElementById('admin-draws');` добавить:

```js
const btnSync = document.getElementById('btn-sync');
const syncStatus = document.getElementById('sync-status');
```

После блока «Добавление / редактирование» (после `btnCancel.addEventListener(...)`) добавить:

```js
// Синхронизация с архивом timelottery.ru: добавляем только новые розыгрыши.
btnSync.addEventListener('click', sync);

async function sync() {
  syncStatus.hidden = true;
  btnSync.disabled = true;
  btnSync.textContent = 'Синхронизация…';
  try {
    const res = await api('/api/sync', { method: 'POST', body: {} });
    syncStatus.textContent = syncSummary(res);
    syncStatus.className = 'hint';
  } catch (e) {
    syncStatus.textContent = e.message;
    syncStatus.className = 'error';
  } finally {
    btnSync.disabled = false;
    btnSync.textContent = 'Синхронизировать';
    syncStatus.hidden = false;
    refreshList();
  }
}

// syncSummary — краткий итог синхронизации одной строкой.
function syncSummary(res) {
  const issues = res.issues ?? [];
  if (res.added === 0 && issues.length === 0) {
    return 'Новых розыгрышей нет';
  }
  const parts = [`Добавлено ${res.added}, пропущено ${res.skipped}`];
  if (issues.length > 0) {
    const names = issues.map((i) => (i.drawNo > 0 ? `№ ${i.drawNo}` : 'строка без номера'));
    parts.push(`не удалось разобрать: ${names.join(', ')}`);
  }
  return parts.join('; ');
}
```

- [ ] **Step 3: Проверить, что фронтенд-тесты и сборка зелёные**

Run: `node --test web/parse.test.mjs && go build ./...`
Expected: тесты парсера `pass`, сборка без ошибок. (Правки в `web/` требуют пересборки бинаря — embed; `go run` в Task 5 это учитывает.)

- [ ] **Step 4: Закоммитить**

```bash
git add web/admin.html web/admin.js
git commit -m "feat: кнопка синхронизации с архивом в админке"
```

---

### Task 5: Финальная проверка и документация

**Files:**
- Modify: `CLAUDE.md` (архитектура — новый пакет)

- [ ] **Step 1: Прогнать все проверки**

Run: `gofmt -l . ; go vet ./... && go test ./... && node --test web/parse.test.mjs`
Expected: `gofmt` ничего не печатает, vet молчит, все Go-пакеты `ok`, node-тесты `pass`.

- [ ] **Step 2: Сквозная проверка на реальном сайте**

Запустить сервер с временной базой (не трогаем репозиторий) и синхронизировать дважды:

```bash
DB_PATH=/tmp/fortunata-sync-test.db ADMIN_PASSWORD=test go run ./cmd/server &
sleep 3
curl -s -c /tmp/fortunata-cookies.txt -H 'Content-Type: application/json' \
  -d '{"password":"test"}' http://localhost:8080/api/login
curl -s -b /tmp/fortunata-cookies.txt -H 'Content-Type: application/json' \
  -d '{}' http://localhost:8080/api/sync
curl -s -b /tmp/fortunata-cookies.txt -H 'Content-Type: application/json' \
  -d '{}' http://localhost:8080/api/sync
curl -s http://localhost:8080/api/draws | head -c 300; echo
kill %1 2>/dev/null; rm -f /tmp/fortunata-sync-test.db* /tmp/fortunata-cookies.txt
```

Expected: первый вызов — `{"added":N,...}` с N ≥ 60 и пустым `issues` (реальный сайт, номера до ~64); второй — `{"added":0,"skipped":N,...}`; `/api/draws` отдаёт розыгрыши. Если сеть недоступна — допускается ошибка загрузки, но тогда пометить шаг как непройденный и сообщить пользователю, а не отчитаться успехом.

- [ ] **Step 3: Обновить `CLAUDE.md`**

В разделе «Архитектура» после строки про `internal/generate/` добавить:

```markdown
- `internal/timelottery/` — парсер страницы архива timelottery.ru для
  `POST /api/sync` (ищет строки данных по содержимому, устойчив к редизайну)
```

- [ ] **Step 4: Закоммитить**

```bash
git add CLAUDE.md
git commit -m "docs: internal/timelottery в архитектуре CLAUDE.md"
```
