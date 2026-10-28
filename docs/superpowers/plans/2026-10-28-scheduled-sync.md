# Расписание автосинхронизации розыгрышей — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сервер сам запускает синхронизацию архива раз в сутки в HH:MM UTC (по умолчанию 15:00), с догоном при старте, почасовым повтором при ошибке и статусом в админке.

**Architecture:** Новый дженерик-пакет `internal/scheduler` (цикл «спим до HH:MM UTC, при ошибке повтор через retryEvery»). Логика синки извлекается из HTTP-хендлера в `runSync(ctx)`, обёртка `runSyncAndRecord` пишет статус в память `Handler` и используется планировщиком как `fn`. Новый эндпоинт `GET /api/sync-status` (под сессией) отдаёт расписание/nextRun/последний итог; админка показывает его строкой под кнопкой синхронизации.

**Tech Stack:** Go (stdlib + `golang.org/x/net/html`, `modernc.org/sqlite`), ванильный JS.

Спека: `docs/superpowers/specs/2026-10-28-scheduled-sync-design.md`. Ветка: `feat/scheduled-sync`.

---

### Task 1: Пакет internal/scheduler

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Написать падающие тесты**

Создать `internal/scheduler/scheduler_test.go`:

```go
// Тесты планировщика: фейковые часы (now) и ручной «срок» (wait) —
// без реального сна, детерминированно.
package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNextRun(t *testing.T) {
	cases := []struct{ name, now, want string }{
		{"до времени — сегодня", "2026-10-28T10:00:00Z", "2026-10-28T15:00:00Z"},
		{"после времени — завтра", "2026-10-28T16:30:00Z", "2026-10-29T15:00:00Z"},
		{"ровно время — завтра", "2026-10-28T15:00:00Z", "2026-10-29T15:00:00Z"},
		{"перед полуночью", "2026-10-28T23:59:59Z", "2026-10-29T15:00:00Z"},
		{"сразу после полуночи", "2026-10-28T00:00:00Z", "2026-10-28T15:00:00Z"},
	}
	for _, tc := range cases {
		got := nextRun(mustTime(t, tc.now), 15, 0)
		if !got.Equal(mustTime(t, tc.want)) {
			t.Fatalf("%s: got %v, want %s", tc.name, got.UTC(), tc.want)
		}
	}
}

// fakeClock — фейковые часы с ручным сроком: wait пишет задержку в delays
// и ждёт tick() или отмены ctx.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	delays []time.Duration
	fire   chan struct{}
}

func newFakeClock(t *testing.T, now string) *fakeClock {
	return &fakeClock{now: mustTime(t, now), fire: make(chan struct{})}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) wait(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.delays = append(f.delays, d)
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.fire:
		return nil
	}
}

func (f *fakeClock) tick() { f.fire <- struct{}{} }

// delayN — n-я записанная задержка (нумерация с 1); ждёт её появления
// до 2 с — Run работает в горутине, задержка появляется асинхронно.
func (f *fakeClock) delayN(t *testing.T, n int) time.Duration {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		var d time.Duration
		enough := len(f.delays) >= n
		if enough {
			d = f.delays[n-1]
		}
		f.mu.Unlock()
		if enough {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("задержка #%d так и не записана", n)
		}
		time.Sleep(time.Millisecond)
	}
}

// Старт после сегодняшних 15:00 — немедленный запуск (догон после рестарта),
// после успеха — завтрашние 15:00 (от 16:00 это 23 ч).
func TestRunCatchUpThenDaily(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T16:00:00Z")
	calls := make(chan struct{}, 4)
	s := New(15, 0, func(context.Context) error { calls <- struct{}{}; return nil })
	s.now, s.wait = clk.Now, clk.wait

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	clk.tick()
	<-calls
	if d := clk.delayN(t, 1); d != 0 {
		t.Fatalf("догон: задержка = %v, хотим 0", d)
	}
	if d := clk.delayN(t, 2); d != 23*time.Hour {
		t.Fatalf("после успеха: задержка = %v, хотим 23ч", d)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился по отмене ctx")
	}
}

// Старт до 15:00 — ждём ровно до 15:00 (от 14:00 это 1 ч), fn не зовём.
func TestRunWaitsUntilTodayTime(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T14:00:00Z")
	calls := make(chan struct{}, 1)
	s := New(15, 0, func(context.Context) error { calls <- struct{}{}; return nil })
	s.now, s.wait = clk.Now, clk.wait

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	if d := clk.delayN(t, 1); d != time.Hour {
		t.Fatalf("задержка = %v, хотим 1ч", d)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился по отмене ctx")
	}
	select {
	case <-calls:
		t.Fatal("fn вызван без tick")
	default:
	}
}

// Ошибка fn — повтор через retryEvery; успех после повтора — возврат
// к суточному ритму.
func TestRunRetryOnError(t *testing.T) {
	clk := newFakeClock(t, "2026-10-28T16:00:00Z")
	var fail atomic.Bool
	fail.Store(true)
	calls := make(chan struct{}, 4)
	s := New(15, 0, func(context.Context) error {
		calls <- struct{}{}
		if fail.Load() {
			return errors.New("архив недоступен")
		}
		return nil
	})
	s.now, s.wait, s.retryEvery = clk.Now, clk.wait, time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	clk.tick()
	<-calls
	if d := clk.delayN(t, 2); d != time.Hour {
		t.Fatalf("после ошибки: задержка = %v, хотим 1ч (retryEvery)", d)
	}
	fail.Store(false)
	clk.tick()
	<-calls
	if d := clk.delayN(t, 3); d != 23*time.Hour {
		t.Fatalf("после успеха: задержка = %v, хотим 23ч", d)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./internal/scheduler/`
Expected: FAIL — «undefined: New», «undefined: nextRun» (пакета ещё нет).

- [ ] **Step 3: Реализовать пакет**

Создать `internal/scheduler/scheduler.go`:

```go
// Пакет scheduler: ежедневный запуск функции в HH:MM UTC с почасовым
// повтором при ошибке. Ничего не знает о том, что запускает, — вызывающий
// передаёт fn.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler вызывает fn ежедневно в hour:min UTC. Поле now и wait — точка
// инъекции для детерминированных тестов (см. scheduler_test.go).
type Scheduler struct {
	hour, min  int
	fn         func(context.Context) error
	now        func() time.Time
	wait       func(context.Context, time.Duration) error
	retryEvery time.Duration
}

// New создаёт планировщик с реальными часами и часовым интервалом повтора.
func New(hour, min int, fn func(context.Context) error) *Scheduler {
	return &Scheduler{
		hour:       hour,
		min:        min,
		fn:         fn,
		now:        time.Now,
		wait:       waitDur,
		retryEvery: time.Hour,
	}
}

// NextRun — ближайшие hour:min UTC. Использует статус-эндпоинт.
func (s *Scheduler) NextRun() time.Time { return nextRun(s.now(), s.hour, s.min) }

// Run — блокирующий цикл: дождаться срока, вызвать fn. При ошибке fn
// повтор через retryEvery, при успехе — завтра в hour:min. Выход по ctx.
func (s *Scheduler) Run(ctx context.Context) {
	delay := s.initialDelay()
	for {
		if err := s.wait(ctx, delay); err != nil {
			return // ctx отменён — graceful shutdown
		}
		if err := s.fn(ctx); err != nil {
			slog.Warn("scheduler: запуск не удался, повтор позже", "err", err, "retryIn", s.retryEvery)
			delay = s.retryEvery
		} else {
			delay = time.Until(s.NextRun())
		}
	}
}

// initialDelay — сколько ждать первого запуска. Если сегодняшние hour:min
// уже прошли — 0: догон после рестарта (повтор безвреден, синка
// идемпотентна — дубли пропускаются).
func (s *Scheduler) initialDelay() time.Duration {
	now := s.now().UTC()
	run := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.min, 0, 0, time.UTC)
	if !run.After(now) {
		return 0
	}
	return run.Sub(now)
}

// nextRun — ближайшие hour:min UTC относительно now: сегодня, если ещё не
// наступили, иначе завтра. Ровно hour:min считается наступившим.
func nextRun(now time.Time, hour, min int) time.Time {
	now = now.UTC()
	run := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	if !run.After(now) {
		run = run.AddDate(0, 0, 1)
	}
	return run
}

// waitDur — реальная реализация wait: спать d или выйти по ctx.
// d <= 0 — не спать (немедленный запуск), но отменённый ctx важнее.
func waitDur(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./internal/scheduler/ -v`
Expected: все PASS, без `go vet`-замечаний (`go vet ./internal/scheduler/`).

- [ ] **Step 5: Коммит**

```bash
git add internal/scheduler/
git commit -m "feat: планировщик ежедневного запуска задач по UTC-времени"
```

---

### Task 2: Конфиг SYNC_AT / SYNC_ENABLED

**Files:**
- Modify: `cmd/server/main.go` (struct `Config` c 25, `loadConfig` c 38-78)
- Test: `cmd/server/main_test.go`

- [ ] **Step 1: Написать падающие тесты**

Добавить в `cmd/server/main_test.go`:

```go
func TestLoadConfigSyncDefaults(t *testing.T) {
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
	if cfg.SyncAtHour != 15 || cfg.SyncAtMin != 0 || !cfg.SyncEnabled {
		t.Fatalf("дефолты SYNC_*: %+v", cfg)
	}
}

func TestLoadConfigSyncCustom(t *testing.T) {
	env := map[string]string{
		"ADMIN_PASSWORD": "pass",
		"SYNC_AT":        "07:30",
		"SYNC_ENABLED":   "false",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyncAtHour != 7 || cfg.SyncAtMin != 30 || cfg.SyncEnabled {
		t.Fatalf("свой SYNC_*: %+v", cfg)
	}
}

func TestLoadConfigSyncBadAt(t *testing.T) {
	// «7:30» и «15:00:00» отвергаем: формат строго HH:MM, две цифры каждого поля.
	for _, at := range []string{"25:00", "7:30", "15:60", "abc", "15:00:00", "-1:00"} {
		env := map[string]string{"ADMIN_PASSWORD": "pass", "SYNC_AT": at}
		_, err := loadConfig(func(k string) string { return env[k] })
		if err == nil || !strings.Contains(err.Error(), "SYNC_AT") {
			t.Fatalf("SYNC_AT=%q: ожидали ошибку, got %v", at, err)
		}
	}
}

func TestLoadConfigSyncBadEnabled(t *testing.T) {
	env := map[string]string{"ADMIN_PASSWORD": "pass", "SYNC_ENABLED": "maybe"}
	_, err := loadConfig(func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "SYNC_ENABLED") {
		t.Fatalf("ожидали ошибку SYNC_ENABLED, got %v", err)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./cmd/server/ -run 'Sync' -v`
Expected: FAIL — «cfg.SyncAtHour undefined» (полей ещё нет).

- [ ] **Step 3: Реализовать**

В `cmd/server/main.go` в struct `Config` после `LLMAPIKey` добавить:

```go
	SyncAtHour  int  // SYNC_AT, час запуска синки в UTC (по умолчанию 15)
	SyncAtMin   int  // SYNC_AT, минута (по умолчанию 0)
	SyncEnabled bool // SYNC_ENABLED, по умолчанию true
```

В `loadConfig` после блока `COOKIE_SECURE` (перед `cfg.LLMBaseURL = ...`) добавить:

```go
	syncAt := getenv("SYNC_AT")
	if syncAt == "" {
		syncAt = "15:00"
	}
	hour, min, err := parseSyncAt(syncAt)
	if err != nil {
		return Config{}, fmt.Errorf("SYNC_AT: %w", err)
	}
	cfg.SyncAtHour, cfg.SyncAtMin = hour, min
	enabled := getenv("SYNC_ENABLED")
	if enabled == "" {
		enabled = "true"
	}
	b, err = strconv.ParseBool(enabled)
	if err != nil {
		return Config{}, fmt.Errorf("SYNC_ENABLED: %w", err)
	}
	cfg.SyncEnabled = b
```

После `loadConfig` добавить функцию (и в импорты — `"strings"`):

```go
// parseSyncAt разбирает SYNC_AT формата HH:MM (время в UTC).
// Формат строгий: ровно две цифры на поле — «7:30» отвергается.
func parseSyncAt(s string) (hour, min int, err error) {
	hh, mm, ok := strings.Cut(s, ":")
	if !ok || len(hh) != 2 || len(mm) != 2 {
		return 0, 0, errors.New(`ожидается время в формате HH:MM, например "15:00"`)
	}
	if hour, err = strconv.Atoi(hh); err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("часы %q вне 00–23", hh)
	}
	if min, err = strconv.Atoi(mm); err != nil || min < 0 || min > 59 {
		return 0, 0, fmt.Errorf("минуты %q вне 00–59", mm)
	}
	return hour, min, nil
}
```

- [ ] **Step 4: Прогнать тесты**

Run: `go test ./cmd/server/ -v`
Expected: все PASS (новые и существующие `TestLoadConfig*`).

- [ ] **Step 5: Коммит**

```bash
git add cmd/server/main.go cmd/server/main_test.go
git commit -m "feat: конфиг SYNC_AT и SYNC_ENABLED для автосинхронизации"
```

---

### Task 3: Рефакторинг — runSync(ctx) и отменяемый fetchArchive

Поведение `POST /api/sync` не меняется; существующие тесты — регрессия.

**Files:**
- Modify: `internal/api/sync.go`

- [ ] **Step 1: Выделить runSync и прокинуть ctx**

В `internal/api/sync.go` добавить `"context"` в импорты. Заменить тело `syncDraws` и `fetchArchive` на:

```go
func (h *Handler) syncDraws(w http.ResponseWriter, r *http.Request) {
	res, err := h.runSync(r.Context())
	if err != nil {
		errorJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// runSync — тело синхронизации без HTTP: fetch → parse → вставки.
// Вызывается хендлером и (через runSyncAndRecord) планировщиком.
func (h *Handler) runSync(ctx context.Context) (syncResult, error) {
	draws, issues, err := h.fetchArchive(ctx, h.archiveURL)
	if err != nil {
		return syncResult{}, err
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
	return res, nil
}

// fetchArchive скачивает страницу архива и разбирает её. Запрос отменяется
// вместе с ctx (HTTP-хендлер — контекст запроса, планировщик — контекст сервера).
func (h *Handler) fetchArchive(ctx context.Context, url string) ([]timelottery.Draw, []timelottery.Issue, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := h.archiveClient.Do(req)
	if err != nil {
		slog.Warn("sync: загрузка архива", "url", url, "err", err)
		return nil, nil, errors.New("не удалось загрузить страницу архива")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("сервер архива ответил %d", resp.StatusCode)
	}
	draws, issues, err := timelottery.Parse(&limitedReader{r: resp.Body, n: maxArchiveBytes})
	if err != nil {
		if errors.Is(err, errArchiveTooBig) {
			return nil, nil, errArchiveTooBig // без префикса «разбор html:»
		}
		return nil, nil, err
	}
	return draws, issues, nil
}
```

- [ ] **Step 2: Регрессия**

Run: `go test ./internal/api/ -run 'Sync' -v`
Expected: все существующие `TestSync*` PASS (включая `TestSyncArchiveTimeout` — таймаут клиента работает и через `Do`).

- [ ] **Step 3: Коммит**

```bash
git add internal/api/sync.go
git commit -m "refactor: runSync(ctx) — синка вне HTTP-хендлера, отменяемый fetchArchive"
```

---

### Task 4: Статус синки, эндпоинт и подключение планировщика

**Files:**
- Create: `internal/api/sync_status.go`
- Test: `internal/api/sync_status_test.go`
- Modify: `internal/api/api.go` (Handler, newHandler, New, register)
- Modify: `cmd/server/main.go:105` (вызов api.New + запуск планировщика)
- Modify: `internal/api/api_test.go:26,455`, `internal/api/sync_test.go:45,227,251,271,328`, `internal/api/generate_ai_test.go:32,43` (новые сигнатуры)

- [ ] **Step 1: Написать падающие тесты статуса**

Создать `internal/api/sync_status_test.go`:

```go
// Тесты GET /api/sync-status и записи статуса последней синки.
package api

import (
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
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `go test ./internal/api/ -run 'SyncStatus' -v`
Expected: FAIL — «undefined: SyncConfig», «undefined: New» с 7 аргументами.

- [ ] **Step 3: Реализовать статус и эндпоинт**

Создать `internal/api/sync_status.go`:

```go
// Статус последней синки (в памяти процесса) и расписание автосинхронизации
// для админки: GET /api/sync-status.
package api

import (
	"context"
	"net/http"
	"time"
)

// syncStatus — итог последней синки (ручной или автоматической). Хранится
// в памяти: после рестарта last == nil до первой синки — догон при старте
// заполняет его сразу.
type syncStatus struct {
	At      time.Time `json:"at"`
	Added   int       `json:"added"`
	Skipped int       `json:"skipped"`
	Issues  int       `json:"issues"`
	Err     string    `json:"error,omitempty"` // пусто — успех
}

// runSyncAndRecord — синка с записью итога в статус. Её вызывает и
// POST /api/sync, и планировщик (её fn). Ошибка возвращается наружу —
// по ней планировщик планирует повтор.
func (h *Handler) runSyncAndRecord(ctx context.Context) error {
	res, err := h.runSync(ctx)
	st := syncStatus{At: time.Now().UTC(), Issues: len(res.Issues)}
	if err != nil {
		st.Err = err.Error()
	} else {
		st.Added, st.Skipped = res.Added, res.Skipped
	}
	h.mu.Lock()
	h.lastSync = &st
	h.mu.Unlock()
	return err
}

type syncStatusResponse struct {
	Enabled  bool        `json:"enabled"`
	Schedule string      `json:"schedule"`
	NextRun  *time.Time  `json:"nextRun"`
	Last     *syncStatus `json:"last"`
}

// syncStatusHandler — GET /api/sync-status (под сессией, как остальные
// админские маршруты).
func (h *Handler) syncStatusHandler(w http.ResponseWriter, r *http.Request) {
	resp := syncStatusResponse{Enabled: h.syncEnabled, Schedule: h.syncAt}
	if h.sched != nil {
		nr := h.sched.NextRun()
		resp.NextRun = &nr
	}
	h.mu.Lock()
	resp.Last = h.lastSync
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}
```

В `internal/api/sync.go` в `syncDraws` заменить `h.runSync(r.Context())` на `h.runSyncAndRecord(r.Context())` (ручная синка тоже обновляет статус).

В `internal/api/api.go`:

Импорты дополнить: `"fmt"`, `"sync"`, `"fortunata/internal/scheduler"`.

После `defaultArchiveURL` добавить:

```go
// SyncConfig — настройки автосинхронизации из env (см. cmd/server).
type SyncConfig struct {
	Hour, Min int
	Enabled   bool
}
```

В struct `Handler` добавить поля:

```go
	syncEnabled bool
	syncAt      string               // «15:00 UTC» — для статуса
	sched       *scheduler.Scheduler // nil — автосинхронизация выключена
	mu          sync.Mutex           // защищает lastSync
	lastSync    *syncStatus
```

`newHandler` — новый последний параметр `syncCfg SyncConfig`, тело:

```go
func newHandler(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client, syncCfg SyncConfig) *Handler {
	if archiveURL == "" {
		archiveURL = defaultArchiveURL
	}
	h := &Handler{
		st:            st,
		auth:          auth.New(password, secret),
		cookieSecure:  cookieSecure,
		archiveURL:    archiveURL,
		archiveClient: &http.Client{Timeout: 10 * time.Second},
		llm:           llmClient,
		aiSlots:       make(chan struct{}, 1),
		syncEnabled:   syncCfg.Enabled,
		syncAt:        fmt.Sprintf("%02d:%02d UTC", syncCfg.Hour, syncCfg.Min),
	}
	if syncCfg.Enabled {
		h.sched = scheduler.New(syncCfg.Hour, syncCfg.Min, h.runSyncAndRecord)
	}
	return h
}
```

`New` — новый сигнатурой (см. спеку: планировщик создаётся здесь, его fn — метод Handler):

```go
// New собирает все /api-маршруты; возвращает mux и планировщик автосинки
// (nil — выключена; main запускает его: go sched.Run(ctx)). Пустой
// archiveURL заменяется на defaultArchiveURL; llmClient == nil выключает
// AI-генерацию.
func New(st *store.Store, password, secret string, cookieSecure bool, archiveURL string, llmClient *llm.Client, syncCfg SyncConfig) (*http.ServeMux, *scheduler.Scheduler) {
	h := newHandler(st, password, secret, cookieSecure, archiveURL, llmClient, syncCfg)
	mux := http.NewServeMux()
	h.register(mux)
	return mux, h.sched
}
```

В `register` добавить строку:

```go
	mux.HandleFunc("GET /api/sync-status", h.session(h.syncStatusHandler))
```

- [ ] **Step 4: Обновить все вызовы New/newHandler**

`cmd/server/main.go:105` (в импорты `"fmt"` уже есть):

```go
	mux, sched := api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure, "", llmClient,
		api.SyncConfig{Hour: cfg.SyncAtHour, Min: cfg.SyncAtMin, Enabled: cfg.SyncEnabled})
```

После создания `ctx, stop := signal.NotifyContext(...)` (и до `go func() { ... ListenAndServe ... }()`):

```go
	if sched != nil {
		go sched.Run(ctx)
		logger.Info("автосинхронизация включена", "at", fmt.Sprintf("%02d:%02d UTC", cfg.SyncAtHour, cfg.SyncAtMin))
	}
```

В `internal/api/api_test.go` — хелпер рядом с `newTestServer` (и оба вызова `New` в файле, строки ~26 и ~455, перевести на него):

```go
// syncOn — включённый SyncConfig для тестов: планировщик создаётся,
// но не запускается (Run дёргает только main).
func syncOn() SyncConfig { return SyncConfig{Hour: 15, Min: 0, Enabled: true} }
```

```go
	mux, _ := New(st, "pass123", "test-secret", false, "", nil, syncOn())
	ts := httptest.NewServer(mux)
```

В `internal/api/sync_test.go`: строки ~45 и ~227 — так же через `syncOn()` и `mux, _ :=`; строки ~251, ~271, ~328 (`newHandler`) — добавить последний аргумент `SyncConfig{}`:

```go
	h := newHandler(newTestStore(t), "pass123", "test-secret", false, upstream.URL, nil, SyncConfig{})
```

В `internal/api/generate_ai_test.go`: строки ~32 (`New`) — `mux, _ := New(st, "pass123", "test-secret", false, "", llm.NewClient(upstream.URL, "test-model", "", nil), syncOn())` + `httptest.NewServer(mux)`; строка ~43 (`newHandler`) — добавить `, SyncConfig{}`.

- [ ] **Step 5: Прогнать всё**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: все PASS.

- [ ] **Step 6: Коммит**

```bash
git add internal/api/ cmd/server/main.go
git commit -m "feat: автосинхронизация по расписанию, статус в GET /api/sync-status"
```

---

### Task 5: Админка — строка статуса автосинхронизации

**Files:**
- Modify: `web/admin.html` (после строки 39)
- Modify: `web/admin.js`

- [ ] **Step 1: Разметка**

В `web/admin.html` сразу за `<p id="sync-status" ...></p>` (строка 39) добавить:

```html
        <p id="auto-sync" class="hint" hidden></p>
```

- [ ] **Step 2: Логика**

В `web/admin.js`:

После `const syncStatus = ...` (строка 15) добавить:

```js
const autoSync = document.getElementById('auto-sync');
```

В `init()` внутри `if (me.authenticated) { ... }` после `refreshList();` добавить:

```js
      refreshAutoSync();
```

В `sync()` в блоке `finally` после `refreshList();` добавить:

```js
    refreshAutoSync();
```

В конец файла добавить:

```js
// Статус автосинхронизации: расписание, следующий запуск, последний итог.
async function refreshAutoSync() {
  try {
    const s = await api('/api/sync-status');
    autoSync.hidden = false;
    autoSync.textContent = autoSyncText(s);
  } catch {
    autoSync.hidden = true; // статус не критичен — скрываем, панель работает
  }
}

function autoSyncText(s) {
  if (!s.enabled) {
    return 'Автосинхронизация выключена';
  }
  const parts = [`Автосинхронизация ежедневно в ${s.schedule}`];
  if (s.nextRun) {
    parts.push(`следующая — ${fmtUTC(s.nextRun)}`);
  }
  if (s.last) {
    if (s.last.error) {
      parts.push(`последняя: ${fmtUTC(s.last.at)}, ошибка: ${s.last.error}`);
    } else {
      parts.push(`последняя: ${fmtUTC(s.last.at)}, добавлено ${s.last.added}`);
    }
  }
  return parts.join('. ') + '.';
}

// fmtUTC — время в UTC: расписание задаётся в UTC, показываем то же время,
// что и на сервере, без перевода в локальный часовой пояс.
function fmtUTC(iso) {
  return new Date(iso).toLocaleString('ru-RU', {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', timeZone: 'UTC',
  });
}
```

- [ ] **Step 3: Проверить вживую**

```bash
docker compose -f compose.local.yaml up --build -d
```

Открыть http://localhost:8080/admin, войти (пароль `test123`), убедиться: под кнопкой «Синхронизировать» появилась строка «Автосинхронизация ежедневно в 15:00 UTC, следующая — …»; после нажатия «Синхронизировать» строка обновилась с итогом. Затем `docker compose -f compose.local.yaml down -v` (внимание: `-v` удалит базу — это локальный тестовый контейнер).

- [ ] **Step 4: Коммит**

```bash
git add web/admin.html web/admin.js
git commit -m "feat: строка статуса автосинхронизации в админке"
```

---

### Task 6: Документация и финальная проверка

**Files:**
- Modify: `CLAUDE.md` (секции «Архитектура» и «Окружение»)

- [ ] **Step 1: Обновить CLAUDE.md**

В «Архитектура» после пункта про `internal/timelottery/` добавить:

```markdown
- `internal/scheduler/` — ежедневный запуск задачи в HH:MM UTC (автосинка):
  догон при старте, повтор через час при ошибке; время/интервалы — точка
  инъекции для тестов
```

В «Окружение» после предложения про `LLM_API_KEY` добавить:

```markdown
`SYNC_AT` (`15:00`, время суточной автосинки в UTC, формат `HH:MM` строго),
`SYNC_ENABLED` (`false` — планировщик не заводится, ручная синка работает).
```

- [ ] **Step 2: Полная проверка**

Run: `gofmt -l . && go vet ./... && go test ./... && node --test web/parse.test.mjs`
Expected: пустой вывод `gofmt`, тесты PASS.

- [ ] **Step 3: Коммит**

```bash
git add CLAUDE.md
git commit -m "docs: SYNC_AT и SYNC_ENABLED в описании окружения"
```
