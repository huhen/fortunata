# Страница статистики по архиву — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Вкладка «Статистика» с частотами выпадения шаров архива: основные, затем бонусные, по убыванию количества, с числом выпадений у каждого шара.

**Architecture:** Сервер считает частоты SQL-агрегацией (`UNION ALL` по n1..n7 / по bonus, `GROUP BY`, сортировка count DESC, номер ASC) и отдаёт готовые списки через новый публичный `GET /api/stats`. Фронтенд при открытии вкладки рисует две карточки из готовых данных.

**Tech Stack:** Go 1.x, `modernc.org/sqlite`, vanilla JS (ES-модули), `//go:embed` статика.

**Спека:** `docs/superpowers/specs/2026-09-17-archive-stats-design.md`

---

### Task 1: Store — `Freq` и `MainFrequency`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [x] **Step 1: Создать ветку**

```bash
git checkout -b feat/archive-stats
```

- [x] **Step 2: Написать падающий тест**

Добавить в конец `internal/store/store_test.go`:

```go
func TestMainFrequency(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(Draw{DrawNo: 2, Numbers: []int{1, 2, 3, 8, 9, 10, 11}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	got, err := st.MainFrequency()
	if err != nil {
		t.Fatal(err)
	}
	// 1,2,3 — по два раза; остальные по одному; при равенстве — номер ASC.
	// Числа 12–35 не выпадали и в результат попасть не должны.
	want := []Freq{
		{N: 1, Count: 2}, {N: 2, Count: 2}, {N: 3, Count: 2},
		{N: 4, Count: 1}, {N: 5, Count: 1}, {N: 6, Count: 1}, {N: 7, Count: 1},
		{N: 8, Count: 1}, {N: 9, Count: 1}, {N: 10, Count: 1}, {N: 11, Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, хотим %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %+v, хотим %+v (порядок: count DESC, затем номер ASC)", i, got[i], want[i])
		}
	}
}

func TestFrequencyEmptyBase(t *testing.T) {
	st := openTest(t)
	main, err := st.MainFrequency()
	if err != nil {
		t.Fatal(err)
	}
	if main == nil || len(main) != 0 {
		t.Fatalf("пустая база: main = %v, хотим непустой пустой срез", main)
	}
}
```

- [x] **Step 3: Убедиться, что тест падает**

Run: `go test ./internal/store/ -run 'TestMainFrequency|TestFrequencyEmptyBase' -v`
Expected: FAIL — сборка не проходит: `undefined: Freq` / `st.MainFrequency undefined`

- [x] **Step 4: Реализовать `Freq` и `MainFrequency`**

Добавить в конец `internal/store/store.go`:

```go
// Freq — сколько раз выпал шар n.
type Freq struct {
	N     int `json:"n"`
	Count int `json:"count"`
}

// frequency выполняет запрос вида «номер, количество» с сортировкой
// count DESC, затем номер ASC, и собирает результат в срез.
func (s *Store) frequency(query string) ([]Freq, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("frequency: %w", err)
	}
	defer rows.Close()
	out := []Freq{}
	for rows.Next() {
		var f Freq
		if err := rows.Scan(&f.N, &f.Count); err != nil {
			return nil, fmt.Errorf("scan freq: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

const mainFrequencyQuery = `SELECT n, COUNT(*) c FROM (
	SELECT n1 n FROM draws UNION ALL
	SELECT n2 FROM draws UNION ALL
	SELECT n3 FROM draws UNION ALL
	SELECT n4 FROM draws UNION ALL
	SELECT n5 FROM draws UNION ALL
	SELECT n6 FROM draws UNION ALL
	SELECT n7 FROM draws
) GROUP BY n ORDER BY c DESC, n ASC`

// MainFrequency — сколько раз выпал каждый основной шар (n1..n7).
// Никогда не выпадавшие номера в результат не попадают.
func (s *Store) MainFrequency() ([]Freq, error) {
	return s.frequency(mainFrequencyQuery)
}
```

- [x] **Step 5: Убедиться, что тесты проходят**

Run: `go test ./internal/store/ -v`
Expected: PASS, все тесты пакета зелёные

- [x] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: частоты основных шаров MainFrequency в store"
```

---

### Task 2: Store — `BonusFrequency`

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [x] **Step 1: Написать падающий тест**

Добавить в конец `internal/store/store_test.go`:

```go
func TestBonusFrequency(t *testing.T) {
	st := openTest(t)
	for _, d := range []Draw{
		{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8},
		{DrawNo: 2, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 54},
		{DrawNo: 3, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8},
	} {
		if err := st.Create(d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.BonusFrequency()
	if err != nil {
		t.Fatal(err)
	}
	want := []Freq{{N: 8, Count: 2}, {N: 54, Count: 1}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, хотим %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %+v, хотим %+v", i, got[i], want[i])
		}
	}
}
```

- [x] **Step 2: Убедиться, что тест падает**

Run: `go test ./internal/store/ -run TestBonusFrequency -v`
Expected: FAIL — `st.BonusFrequency undefined`

- [x] **Step 3: Реализовать `BonusFrequency`**

Добавить в конец `internal/store/store.go`:

```go
const bonusFrequencyQuery = `SELECT bonus n, COUNT(*) c FROM draws GROUP BY bonus ORDER BY c DESC, bonus ASC`

// BonusFrequency — сколько раз выпал каждый бонусный шар.
// Никогда не выпадавшие номера в результат не попадают.
func (s *Store) BonusFrequency() ([]Freq, error) {
	return s.frequency(bonusFrequencyQuery)
}
```

- [x] **Step 4: Убедиться, что тесты проходят**

Run: `go test ./internal/store/ -v`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: частоты бонусных шаров BonusFrequency в store"
```

---

### Task 3: API — публичный `GET /api/stats`

**Files:**
- Create: `internal/api/stats.go`
- Modify: `internal/api/api.go` (регистрация маршрута), `internal/api/api_test.go` (импорт `reflect` + тест)

- [x] **Step 1: Написать падающий тест**

В `internal/api/api_test.go` добавить `"reflect"` в список импортов:

```go
import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"fortunata/internal/store"
)
```

Добавить в конец файла тест:

```go
func TestStats(t *testing.T) {
	ts := newTestServer(t)
	type freq struct {
		N     int `json:"n"`
		Count int `json:"count"`
	}
	var body struct {
		Main  []freq `json:"main"`
		Bonus []freq `json:"bonus"`
	}
	// Пустая база: доступ без авторизации, пустые списки.
	resp := get(t, clientWithJar(), ts.URL+"/api/stats")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("пустая база: status = %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(body.Main) != 0 || len(body.Bonus) != 0 {
		t.Fatalf("пустая база: main = %v, bonus = %v", body.Main, body.Bonus)
	}

	// Сид: числа 1,2,3 — по два раза; бонус 8 — дважды.
	c := loginClient(t, ts)
	for _, d := range []map[string]any{
		{"drawNo": 1, "numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8},
		{"drawNo": 2, "numbers": []int{1, 2, 3, 8, 9, 10, 11}, "bonus": 8},
	} {
		r := post(t, c, ts.URL+"/api/draws", d)
		r.Body.Close()
	}

	resp2 := get(t, clientWithJar(), ts.URL+"/api/stats")
	defer resp2.Body.Close()
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	wantMain := []freq{
		{N: 1, Count: 2}, {N: 2, Count: 2}, {N: 3, Count: 2},
		{N: 4, Count: 1}, {N: 5, Count: 1}, {N: 6, Count: 1}, {N: 7, Count: 1},
		{N: 8, Count: 1}, {N: 9, Count: 1}, {N: 10, Count: 1}, {N: 11, Count: 1},
	}
	wantBonus := []freq{{N: 8, Count: 2}}
	if !reflect.DeepEqual(body.Main, wantMain) || !reflect.DeepEqual(body.Bonus, wantBonus) {
		t.Fatalf("main = %v, bonus = %v", body.Main, body.Bonus)
	}
}
```

- [x] **Step 2: Убедиться, что тест падает**

Run: `go test ./internal/api/ -run TestStats -v`
Expected: FAIL — 404 вместо 200 (маршрута ещё нет)

- [x] **Step 3: Создать `internal/api/stats.go`**

```go
package api

import "net/http"

// stats — публичная частотная статистика по архиву: основные и бонусные шары.
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	main, err := h.st.MainFrequency()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось посчитать статистику")
		return
	}
	bonus, err := h.st.BonusFrequency()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось посчитать статистику")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"main": main, "bonus": bonus})
}
```

- [x] **Step 4: Зарегистрировать маршрут**

В `internal/api/api.go` внутри `func (h *Handler) register(mux *http.ServeMux)` после строки
`mux.HandleFunc("GET /api/draws", h.listDraws)` добавить:

```go
	mux.HandleFunc("GET /api/stats", h.stats)
```

- [x] **Step 5: Убедиться, что тесты проходят**

Run: `go test ./...`
Expected: PASS, все пакеты зелёные

- [x] **Step 6: Commit**

```bash
git add internal/api/stats.go internal/api/api.go internal/api/api_test.go
git commit -m "feat: публичный GET /api/stats — частоты шаров"
```

---

### Task 4: Фронтенд — вкладка «Статистика»

Тестовой инфраструктуры для `app.js` нет (тесты есть только у `parse.js`), поэтому
проверка — сборка и smoke-тест curl'ом. Правки в `web/` требуют пересборки —
`go run`/`go build` делают это сами (embed).

**Files:**
- Modify: `web/index.html`, `web/app.js`, `web/style.css`

- [x] **Step 1: Вкладка и панель в `web/index.html`**

В `<nav class="tabs">` после кнопки Архива добавить:

```html
    <button class="tab" data-tab="stats" role="tab" aria-selected="false">Статистика</button>
```

После секции `<section id="tab-archive" ...>...</section>` добавить:

```html
    <section id="tab-stats" class="panel" role="tabpanel">
      <div id="stats-list"></div>
    </section>
```

- [x] **Step 2: Загрузка и отрисовка в `web/app.js`**

В обработчике клика по вкладкам после блока

```js
    if (tab.dataset.tab === 'archive') {
      loadDraws();
    }
```

добавить:

```js
    if (tab.dataset.tab === 'stats') {
      loadStats();
    }
```

В конец файла добавить (помощники `api` и `ball` уже импортированы вверху файла):

```js
// Статистика
async function loadStats() {
  const box = document.getElementById('stats-list');
  try {
    const data = await api('/api/stats');
    box.replaceChildren();
    if (data.main.length === 0) {
      box.textContent = 'Розыгрышей пока нет — добавьте их через редактирование.';
      return;
    }
    box.append(statsCard('Основные шары', data.main, ''));
    box.append(statsCard('Бонусные шары', data.bonus, 'bonus'));
  } catch (e) {
    box.textContent = e.message;
  }
}

// statsCard — карточка с заголовком и списком «шар + количество выпадений».
function statsCard(title, freqs, extra) {
  const card = document.createElement('div');
  card.className = 'card';
  const h2 = document.createElement('h2');
  h2.className = 'stat-title';
  h2.textContent = title;
  card.append(h2);
  const list = document.createElement('div');
  list.className = 'stat-list';
  for (const f of freqs) {
    const item = document.createElement('span');
    item.className = 'stat-item';
    item.append(ball(f.n, extra));
    const count = document.createElement('span');
    count.className = 'ball-count';
    count.textContent = f.count;
    item.append(count);
    list.append(item);
  }
  card.append(list);
  return card;
}
```

- [x] **Step 3: Стили в `web/style.css`**

В конец файла добавить:

```css
.stat-title { font-size: 1rem; font-weight: 700; margin: 0 0 10px; }
.stat-list { display: flex; flex-wrap: wrap; gap: 10px 14px; }
.stat-item { display: inline-flex; align-items: center; gap: 4px; }
.ball-count { color: var(--muted); font-size: 0.85rem; font-weight: 700; }
```

- [x] **Step 4: Собрать и прогнать все тесты**

```bash
go build ./... && go test ./... && node --test web/parse.test.mjs
```
Expected: сборка без ошибок, тесты PASS (parse-тесты не задеты)

- [x] **Step 5: Smoke-тест через запущенный сервер**

```bash
go build -o /tmp/fortunata-smoke ./cmd/server
ADDR=:18080 ADMIN_PASSWORD=test DB_PATH=/tmp/fortunata-smoke.db /tmp/fortunata-smoke &
SERVER_PID=$!
curl -sf --retry 5 --retry-connrefused http://localhost:18080/api/stats   # {"main":[],"bonus":[]}
curl -s http://localhost:18080/ | grep -c 'data-tab="stats"'   # 1
kill $SERVER_PID; rm -f /tmp/fortunata-smoke*
```
Expected: `{"main":[],"bonus":[]}` и `1`

- [x] **Step 6: Commit**

```bash
git add web/index.html web/app.js web/style.css
git commit -m "feat: вкладка «Статистика» — частоты шаров архива"
```

---

### Task 5: Пуш и PR

**Files:** без изменений кода.

- [x] **Step 1: Финальная проверка всего**

```bash
go vet ./... && go test ./... && gofmt -l .   # gofmt: пустой вывод
```
Expected: тесты PASS, `gofmt -l` ничего не печатает

- [x] **Step 2: Пуш ветки**

```bash
git push -u origin feat/archive-stats
```

- [x] **Step 3: Создать PR**

```bash
gh pr create --title "feat: страница статистики по архиву" --body "## Что сделано

- \`store\`: \`MainFrequency\`/\`BonusFrequency\` — SQL-агрегация частот шаров (count DESC, номер ASC; невыпадавшие не попадают)
- \`api\`: публичный \`GET /api/stats\` → \`{\"main\":[...],\"bonus\":[...]}\`
- \`web\`: третья вкладка «Статистика» — карточки «Основные шары» и «Бонусные шары», у каждого шара количество выпадений

Спека: docs/superpowers/specs/2026-09-17-archive-stats-design.md"
```

---

## Итог

Пять задач: два TDD-цикла в store, TDD-цикл в api, фронтенд со smoke-тестом, пуш+PR. После merge не забыть: правки `web/` уже внутри бинаря — деплой пересборкой.
