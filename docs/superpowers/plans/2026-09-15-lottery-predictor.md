# План реализации: сайт-«предсказатель» лотереи Fortunata

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Веб-приложение: архив розыгрышей (просмотр/добавление/редактирование под паролем) + частотно-взвешенный генератор билетов «7 из 35 + 1 из 54».

**Architecture:** Go-сервер (stdlib `net/http`, маршрутизация Go 1.22+) с JSON API, SQLite через `modernc.org/sqlite` (pure Go, без CGO), статический фронтенд на ванильном JS, зашитый в бинарник через `go:embed`. Один статический бинарник → Docker-образ на `scratch`, деплой за внешним Traefik.

**Tech Stack:** Go 1.26 (в go.mod директива go 1.25.0 — как сгенерировал тулчейн), modernc.org/sqlite, ванильный JS (ES-модули), `node --test` для JS-юнитов, Docker Compose.

**Спека:** `docs/superpowers/specs/2026-09-15-lottery-predictor-design.md`

**Конвенции:**
- Модуль Go: `generator`.
- Сообщения об ошибках для пользователя — на русском.
- Рабочая директория всех команд: корень репозитория `/home/usr1/coding/work/generator`.

---

## Карта файлов

```
generator/
├── cmd/server/main.go            # конфиг из env, сборка mux, graceful shutdown
├── internal/store/store.go       # SQLite: схема, Draw, CRUD
├── internal/store/store_test.go
├── internal/generate/generate.go # частоты, взвешенный сэмплер, пачка
├── internal/generate/generate_test.go
├── internal/auth/auth.go         # пароль (constant-time), подписанные cookie
├── internal/auth/auth_test.go
├── internal/api/api.go           # Handler, маршруты, middleware, JSON-хелперы
├── internal/api/auth_handlers.go # login / logout / me
├── internal/api/draws.go         # CRUD розыгрышей + валидация
├── internal/api/generate.go      # POST /api/generate
├── internal/api/*_test.go        # httptest-тесты
├── web/                          # фронтенд (go:embed)
│   ├── embed.go                  # //go:embed — раздача статики
│   ├── index.html  admin.html  app.js  admin.js  common.js  parse.js  style.css
│   └── parse.test.mjs            # node --test
├── package.json                  # {"type":"module"} — для node --test
├── Dockerfile  compose.yaml  compose.local.yaml  .env.example  .dockerignore
└── README.md
```

Зависимости пакетов: `api → {store, auth, generate}`; `generate` и `auth` ничего из проекта не импортируют; `main → {api, store, web}`.

---

### Task 1: Каркас проекта и конфиг из env

**Files:**
- Create: `go.mod`, `cmd/server/main.go`, `cmd/server/main_test.go`
- Modify: `.gitignore`

- [ ] **Step 1: go.mod и дополнение .gitignore**

```bash
go mod init generator
```

Добавить в конец `.gitignore` строку (бинарник локальной сборки):

```
/generator
```

- [ ] **Step 2: зависимости**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 3: тест конфига (сначала тест — TDD)**

Создать `cmd/server/main_test.go` со следующим содержимым:

```go
package main

import (
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(func(string) string { return "" })
	if err == nil {
		t.Fatal("ожидали ошибку: ADMIN_PASSWORD не задан")
	}
	if !strings.Contains(err.Error(), "ADMIN_PASSWORD") {
		t.Fatalf("не та ошибка: %v", err)
	}
	_ = cfg
}

func TestLoadConfigFull(t *testing.T) {
	env := map[string]string{
		"ADDR":           ":9000",
		"ADMIN_PASSWORD": "pass",
		"SESSION_SECRET": "s3cret",
		"COOKIE_SECURE":  "true",
		"DB_PATH":        "/tmp/x.db",
	}
	getenv := func(k string) string { return env[k] }
	cfg, err := loadConfig(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.Password != "pass" || cfg.Secret != "s3cret" ||
		!cfg.CookieSecure || cfg.DBPath != "/tmp/x.db" || cfg.SecretRandom {
		t.Fatalf("неожиданный конфиг: %+v", cfg)
	}
}

func TestLoadConfigRandomSecret(t *testing.T) {
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
	if len(cfg.Secret) != 64 || !cfg.SecretRandom {
		t.Fatalf("ожидали случайный 64-символьный hex-секрет, got %q", cfg.Secret)
	}
	if cfg.Addr != ":8080" || cfg.DBPath != "generator.db" || cfg.CookieSecure {
		t.Fatalf("дефолты сломались: %+v", cfg)
	}
}
```

- [ ] **Step 4: убедиться, что тест падает**

Run: `go test ./cmd/server/`
Expected: FAIL — `undefined: loadConfig`

- [ ] **Step 5: минимальный main.go**

`cmd/server/main.go` (сейчас — только конфиг; сервер появится в Task 9):

```go
// Команда server: конфигурация из переменных окружения и HTTP-сервер.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// Config — все настройки приложения, читаемые из окружения.
type Config struct {
	Addr         string // ADDR, по умолчанию ":8080"
	Password     string // ADMIN_PASSWORD, обязателен
	Secret       string // SESSION_SECRET; если пуст — случайный
	SecretRandom bool   // true, если SECRET сгенерирован при старте
	CookieSecure bool   // COOKIE_SECURE, по умолчанию false (за Traefik ставят true)
	DBPath       string // DB_PATH, по умолчанию "generator.db"
}

func loadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:   getenv("ADDR"),
		DBPath: getenv("DB_PATH"),
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "generator.db"
	}
	cfg.Password = getenv("ADMIN_PASSWORD")
	if cfg.Password == "" {
		return Config{}, errors.New("ADMIN_PASSWORD не задан")
	}
	cfg.Secret = getenv("SESSION_SECRET")
	if cfg.Secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return Config{}, fmt.Errorf("генерация SESSION_SECRET: %w", err)
		}
		cfg.Secret = hex.EncodeToString(buf)
		cfg.SecretRandom = true
	}
	secure := getenv("COOKIE_SECURE")
	if secure == "" {
		secure = "false"
	}
	b, err := strconv.ParseBool(secure)
	if err != nil {
		return Config{}, fmt.Errorf("COOKIE_SECURE: %w", err)
	}
	cfg.CookieSecure = b
	return cfg, nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	// Временная точка: сервер подключается в Task 9.
	_ = cfg
}
```

- [ ] **Step 6: тесты зелёные, всё компилируется**

Run: `go test ./... && go vet ./...`
Expected: `ok  generator/cmd/server`, vet без замечаний

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore cmd/server/
git commit -m "feat: каркас модуля и конфиг из переменных окружения
```

---

### Task 2: store — схема, Open, Create

**Files:**
- Create: `internal/store/store.go`, `internal/store/store_test.go`

- [ ] **Step 1: тесты (сначала тест — TDD)**

`internal/store/store_test.go`:

```go
package store

import (
	"errors"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCreateAndList(t *testing.T) {
	st := openTest(t)
	err := st.Create(Draw{DrawNo: 5, Numbers: []int{7, 3, 34, 19, 26, 2, 14}, Bonus: 48})
	if err != nil {
		t.Fatal(err)
	}
	draws, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(draws) != 1 {
		t.Fatalf("ожидали 1 розыгрыш, got %d", len(draws))
	}
	d := draws[0]
	if d.DrawNo != 5 || d.Bonus != 48 {
		t.Fatalf("не те поля: %+v", d)
	}
	want := []int{2, 3, 7, 14, 19, 26, 34} // записываются отсортированными
	for i := range want {
		if d.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v, хотим %v", d.Numbers, want)
		}
	}
}

func TestListSortedDesc(t *testing.T) {
	st := openTest(t)
	for _, no := range []int64{1, 10, 3} {
		if err := st.Create(Draw{DrawNo: no, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
			t.Fatal(err)
		}
	}
	draws, _ := st.List()
	if draws[0].DrawNo != 10 || draws[1].DrawNo != 3 || draws[2].DrawNo != 1 {
		t.Fatalf("порядок нарушен: %v", draws)
	}
}

func TestCreateDuplicate(t *testing.T) {
	st := openTest(t)
	d := Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}
	if err := st.Create(d); err != nil {
		t.Fatal(err)
	}
	err := st.Create(d)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("ожидали ErrDuplicate, got %v", err)
	}
}

func TestCheckConstraints(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 36}, Bonus: 8}); err == nil {
		t.Fatal("ожидали ошибку CHECK: 36 вне 1–35")
	}
	if err := st.Create(Draw{DrawNo: 1, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 55}); err == nil {
		t.Fatal("ожидали ошибку CHECK: бонус 55 вне 1–54")
	}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/store/`
Expected: FAIL — `undefined: Open` (и остальные)

- [ ] **Step 3: реализация store.go (часть 1: Open, Create)**

`internal/store/store.go`:

```go
// Пакет store: хранение розыгрышей в SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite"
)

var (
	ErrDuplicate = errors.New("розыгрыш с таким номером уже существует")
	ErrNotFound  = errors.New("розыгрыш не найден")
)

// Draw — один прошедший розыгрыш.
type Draw struct {
	DrawNo    int64  `json:"drawNo"`
	Numbers   []int  `json:"numbers"` // ровно 7 чисел 1–35, хранятся по возрастанию
	Bonus     int    `json:"bonus"`   // 1–54
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Store struct{ db *sql.DB }

const schema = `CREATE TABLE IF NOT EXISTS draws (
	draw_no    INTEGER PRIMARY KEY CHECK (draw_no >= 1),
	n1 INTEGER NOT NULL CHECK (n1 BETWEEN 1 AND 35),
	n2 INTEGER NOT NULL CHECK (n2 BETWEEN 1 AND 35),
	n3 INTEGER NOT NULL CHECK (n3 BETWEEN 1 AND 35),
	n4 INTEGER NOT NULL CHECK (n4 BETWEEN 1 AND 35),
	n5 INTEGER NOT NULL CHECK (n5 BETWEEN 1 AND 35),
	n6 INTEGER NOT NULL CHECK (n6 BETWEEN 1 AND 35),
	n7 INTEGER NOT NULL CHECK (n7 BETWEEN 1 AND 35),
	bonus      INTEGER NOT NULL CHECK (bonus BETWEEN 1 AND 54),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);`

// Open открывает (и при необходимости создаёт) базу. Для тестов допустим ":memory:".
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("открыть sqlite: %w", err)
	}
	if path == ":memory:" {
		// Каждое соединение получает свою память; ограничиваем пул одним.
		db.SetMaxOpenConns(1)
	}
	for _, pragma := range []string{`PRAGMA journal_mode = WAL`, `PRAGMA busy_timeout = 5000`} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("схема: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const insertCols = `INSERT INTO draws (draw_no, n1, n2, n3, n4, n5, n6, n7, bonus, created_at, updated_at)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Create добавляет розыгрыш. Повторный номер — ErrDuplicate.
func (s *Store) Create(d Draw) error {
	nums, err := normalize(d.Numbers)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(insertCols,
		d.DrawNo, nums[0], nums[1], nums[2], nums[3], nums[4], nums[5], nums[6], d.Bonus, now, now)
	if err != nil {
		// modernc.org/sqlite не даёт стабильного публичного типа кода ошибки —
		// ориентируемся на текст, который тест TestCreateDuplicate фиксирует.
		if strings.Contains(err.Error(), "UNIQUE constraint failed: draws.draw_no") {
			return ErrDuplicate
		}
		return fmt.Errorf("insert draw: %w", err)
	}
	return nil
}

// normalize проверяет и сортирует семёрку.
func normalize(nums []int) ([]int, error) {
	if len(nums) != 7 {
		return nil, errors.New("нужно ровно 7 чисел")
	}
	out := append([]int(nil), nums...)
	sort.Ints(out)
	return out, nil
}

// List возвращает все розыгрыши по убыванию номера.
func (s *Store) List() ([]Draw, error) {
	rows, err := s.db.Query(`SELECT draw_no, n1, n2, n3, n4, n5, n6, n7, bonus, created_at, updated_at
		FROM draws ORDER BY draw_no DESC`)
	if err != nil {
		return nil, fmt.Errorf("list draws: %w", err)
	}
	defer rows.Close()
	var out []Draw
	for rows.Next() {
		var d Draw
		var n1, n2, n3, n4, n5, n6, n7 int
		var created, updated string
		if err := rows.Scan(&d.DrawNo, &n1, &n2, &n3, &n4, &n5, &n6, &n7,
			&d.Bonus, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan draw: %w", err)
		}
		d.Numbers = []int{n1, n2, n3, n4, n5, n6, n7}
		if d.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, fmt.Errorf("created_at: %w", err)
		}
		if d.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
			return nil, fmt.Errorf("updated_at: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/store/`
Expected: PASS (4 теста)

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat: store — схема БД, открытие, создание розыгрыша
```

---

### Task 3: store — List уже есть; Update и Delete

**Files:**
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: тесты**

Добавить в `internal/store/store_test.go`:

```go
func TestUpdate(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(5, []int{9, 8, 7, 6, 5, 4, 3}, 54); err != nil {
		t.Fatal(err)
	}
	draws, _ := st.List()
	d := draws[0]
	want := []int{3, 4, 5, 6, 7, 8, 9}
	for i := range want {
		if d.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v, хотим %v", d.Numbers, want)
		}
	}
	if d.Bonus != 54 {
		t.Fatalf("bonus = %d", d.Bonus)
	}
	if err := st.Update(999, []int{1, 2, 3, 4, 5, 6, 7}, 8); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	st := openTest(t)
	if err := st.Create(Draw{DrawNo: 5, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete(5); err != nil {
		t.Fatal(err)
	}
	draws, _ := st.List()
	if len(draws) != 0 {
		t.Fatalf("после удаления осталось %d", len(draws))
	}
	if err := st.Delete(5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/store/`
Expected: FAIL — `st.Update undefined`

- [ ] **Step 3: реализация — добавить в конец store.go**

```go
// Update заменяет комбинацию розыгрыша drawNo.
func (s *Store) Update(drawNo int64, numbers []int, bonus int) error {
	nums, err := normalize(numbers)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE draws SET n1=?, n2=?, n3=?, n4=?, n5=?, n6=?, n7=?, bonus=?, updated_at=? WHERE draw_no=?`,
		nums[0], nums[1], nums[2], nums[3], nums[4], nums[5], nums[6], bonus,
		time.Now().UTC().Format(time.RFC3339), drawNo)
	if err != nil {
		return fmt.Errorf("update draw: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete удаляет розыгрыш drawNo.
func (s *Store) Delete(drawNo int64) error {
	res, err := s.db.Exec(`DELETE FROM draws WHERE draw_no=?`, drawNo)
	if err != nil {
		return fmt.Errorf("delete draw: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/store/`
Expected: PASS (6 тестов)

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat: store — список, обновление и удаление розыгрышей
```

---

### Task 4: generate — частоты и взвешенный выбор билета

**Files:**
- Create: `internal/generate/generate.go`, `internal/generate/generate_test.go`

- [ ] **Step 1: тесты**

`internal/generate/generate_test.go`:

```go
package generate

import (
	"reflect"
	"testing"
)

// fakeRng выдаёт значения из очереди; после исчерпания возвращает 0.
type fakeRng struct {
	vals  []int
	calls []int
}

func (f *fakeRng) Intn(n int) int {
	f.calls = append(f.calls, n)
	if len(f.vals) == 0 {
		return 0
	}
	v := f.vals[0]
	f.vals = f.vals[1:]
	return v
}

func TestFrequencies(t *testing.T) {
	draws := []DrawFreq{
		{Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 48},
		{Numbers: []int{7, 8, 9, 10, 11, 12, 13}, Bonus: 48},
	}
	main, bonus := Frequencies(draws)
	if main[6] != 2 { // число 7 выпадало дважды
		t.Fatalf("main[6] = %d, хотим 2", main[6])
	}
	if main[0] != 1 || bonus[47] != 2 {
		t.Fatalf("main[0]=%d bonus[47]=%d", main[0], bonus[47])
	}
	if len(main) != 35 || len(bonus) != 54 {
		t.Fatalf("размеры: %d/%d", len(main), len(bonus))
	}
}

func TestGenerateTicketEmptyBase(t *testing.T) {
	// Пустая база: все веса равны, rng всё время даёт 0 → числа 1..7, бонус 1.
	rng := &fakeRng{}
	ticket := GenerateTicket(make([]int, 35), make([]int, 54), rng)
	want := []int{1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(ticket.Numbers, want) || ticket.Bonus != 1 {
		t.Fatalf("ticket = %+v", ticket)
	}
}

func TestGenerateTicketWeighted(t *testing.T) {
	// Число 1 имеет частоту 100 (вес 101), остальные — вес 1.
	mainFreq := make([]int, 35)
	mainFreq[0] = 100
	rng := &fakeRng{vals: []int{0, 32}} // после 0 → первый взвешенный; 32 из 34 → число 34
	ticket := GenerateTicket(mainFreq, make([]int, 54), rng)
	want := []int{1, 2, 3, 4, 5, 6, 34} // 1 выбран первым, затем 34, затем 2..6
	if !reflect.DeepEqual(ticket.Numbers, want) {
		t.Fatalf("numbers = %v, хотим %v", ticket.Numbers, want)
	}
	// Проверяем суммарные веса на каждом шаге: 135, 34, 33, 32, 31, 30, 29, затем бонус 54.
	wantCalls := []int{135, 34, 33, 32, 31, 30, 29, 54}
	if !reflect.DeepEqual(rng.calls, wantCalls) {
		t.Fatalf("calls = %v, хотим %v", rng.calls, wantCalls)
	}
}

func TestGenerateTicketNoDuplicates(t *testing.T) {
	rng := &fakeRng{vals: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	ticket := GenerateTicket(make([]int, 35), make([]int, 54), rng)
	seen := map[int]bool{}
	for _, n := range ticket.Numbers {
		if seen[n] {
			t.Fatalf("повтор числа %d в %v", n, ticket.Numbers)
		}
		seen[n] = true
		if n < 1 || n > 35 {
			t.Fatalf("число %d вне 1–35", n)
		}
	}
	if ticket.Bonus < 1 || ticket.Bonus > 54 {
		t.Fatalf("бонус %d вне 1–54", ticket.Bonus)
	}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/generate/`
Expected: FAIL — `undefined: DrawFreq`

- [ ] **Step 3: реализация generate.go**

```go
// Пакет generate: частотно-взвешенная генерация билетов.
package generate

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
)

// DrawFreq — вход частот: одна комбинация прошедшего розыгрыша.
type DrawFreq struct {
	Numbers []int
	Bonus   int
}

// Ticket — сгенерированный билет.
type Ticket struct {
	Numbers []int `json:"numbers"` // 7 чисел 1–35 по возрастанию
	Bonus   int   `json:"bonus"`   // 1–54
}

// RNG — источник случайности; изолирован для детерминированных тестов.
type RNG interface{ Intn(n int) int }

// CryptoRand — RNG на crypto/rand.
type CryptoRand struct{}

func (CryptoRand) Intn(n int) int {
	if n <= 0 {
		panic(fmt.Sprintf("Intn: недопустимое n=%d", n))
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

// Frequencies считает, сколько раз каждое число выпадало в прошлых розыгрышах.
// main: индекс 0..34 ↔ числа 1..35; bonus: 0..53 ↔ 1..54.
func Frequencies(draws []DrawFreq) (main, bonus []int) {
	main = make([]int, 35)
	bonus = make([]int, 54)
	for _, d := range draws {
		for _, n := range d.Numbers {
			if n >= 1 && n <= 35 {
				main[n-1]++
			}
		}
		if d.Bonus >= 1 && d.Bonus <= 54 {
			bonus[d.Bonus-1]++
		}
	}
	return main, bonus
}

// weightedPick выбирает число (индекс+1) с вероятностью ∝ (вес+1).
// taken может быть nil (ничего не исключено).
func weightedPick(weights []int, taken []bool, rng RNG) int {
	total := 0
	for i, w := range weights {
		if taken != nil && taken[i] {
			continue
		}
		total += w + 1 // +1 — сглаживание: «холодные» числа остаются возможными
	}
	r := rng.Intn(total)
	cum := 0
	for i, w := range weights {
		if taken != nil && taken[i] {
			continue
		}
		cum += w + 1
		if r < cum {
			return i + 1
		}
	}
	panic("weightedPick: недостижимо")
}

// GenerateTicket генерирует один билет: 7 разных чисел без повторов + бонус.
func GenerateTicket(mainFreq, bonusFreq []int, rng RNG) Ticket {
	taken := make([]bool, len(mainFreq))
	nums := make([]int, 0, 7)
	for len(nums) < 7 {
		n := weightedPick(mainFreq, taken, rng)
		taken[n-1] = true
		nums = append(nums, n)
	}
	sort.Ints(nums)
	return Ticket{Numbers: nums, Bonus: weightedPick(bonusFreq, nil, rng)}
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/generate/`
Expected: PASS (4 теста)

- [ ] **Step 5: Commit**

```bash
git add internal/generate/
git commit -m "feat: generate — частоты и взвешенный выбор билета
```

---

### Task 5: generate — пачка билетов с дедупликацией

**Files:**
- Modify: `internal/generate/generate.go`
- Test: `internal/generate/generate_test.go`

- [ ] **Step 1: тесты**

Добавить в `internal/generate/generate_test.go`:

```go
func TestGenerateBatchDistinct(t *testing.T) {
	// Циклический rng: каждый вызов возвращает счётчик по модулю n.
	rng := &counterRng{}
	tickets := GenerateBatch(make([]int, 35), make([]int, 54), 10, rng)
	if len(tickets) != 10 {
		t.Fatalf("len = %d", len(tickets))
	}
	unique := map[string]bool{}
	for _, tk := range tickets {
		unique[ticketKey(tk)] = true
	}
	if len(unique) != 10 {
		t.Fatalf("уникальных билетов %d из 10", len(unique))
	}
}

type counterRng struct{ c int }

func (f *counterRng) Intn(n int) int {
	v := f.c % n
	f.c++
	return v
}

func TestGenerateBatchAcceptsDupAfterCap(t *testing.T) {
	// rng всегда 0 → все билеты одинаковы; после 20 попыток принимаем дубликат.
	rng := &fakeRng{}
	tickets := GenerateBatch(make([]int, 35), make([]int, 54), 3, rng)
	if len(tickets) != 3 {
		t.Fatalf("len = %d", len(tickets))
	}
	for _, tk := range tickets {
		if !reflect.DeepEqual(tk.Numbers, []int{1, 2, 3, 4, 5, 6, 7}) || tk.Bonus != 1 {
			t.Fatalf("неожиданный билет %+v", tk)
		}
	}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/generate/`
Expected: FAIL — `undefined: GenerateBatch`

- [ ] **Step 3: реализация — добавить в generate.go**

```go
// dupAttempts — сколько попыток сгенерировать не-дубликат, прежде чем принять дубликат.
const dupAttempts = 20

// GenerateBatch генерирует count билетов, стараясь не повторяться внутри пачки.
func GenerateBatch(mainFreq, bonusFreq []int, count int, rng RNG) []Ticket {
	seen := make(map[string]struct{}, count)
	tickets := make([]Ticket, 0, count)
	for len(tickets) < count {
		var t Ticket
		for attempt := 0; ; attempt++ {
			t = GenerateTicket(mainFreq, bonusFreq, rng)
			key := ticketKey(t)
			if _, dup := seen[key]; !dup || attempt >= dupAttempts-1 {
				seen[key] = struct{}{}
				tickets = append(tickets, t)
				break
			}
		}
	}
	return tickets
}

func ticketKey(t Ticket) string {
	return fmt.Sprintf("%v+%d", t.Numbers, t.Bonus)
}
```

- [ ] **Step 4: тесты зелёные, весь пакетный прогон чистый**

Run: `go test ./... && go vet ./...`
Expected: все пакеты `ok`, vet без замечаний

- [ ] **Step 5: Commit**

```bash
git add internal/generate/
git commit -m "feat: generate — пачка билетов с дедупликацией
```

---

### Task 6: auth — пароль и подписанные cookie-сессии

**Files:**
- Create: `internal/auth/auth.go`, `internal/auth/auth_test.go`

- [ ] **Step 1: тесты**

`internal/auth/auth_test.go`:

```go
package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func newTest() *Manager {
	m := New("pass123", "test-secret")
	return m
}

func TestCheckPassword(t *testing.T) {
	m := newTest()
	if !m.CheckPassword("pass123") {
		t.Fatal("верный пароль не принят")
	}
	if m.CheckPassword("wrong") || m.CheckPassword("") {
		t.Fatal("неверный пароль принят")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	m := newTest()
	tok := m.NewToken()
	if !m.Verify(tok) {
		t.Fatal("свежий токен не прошёл проверку")
	}
}

func TestTokenGarbage(t *testing.T) {
	m := newTest()
	for _, tok := range []string{"", "abc", "1.zzz", "abc.def", "123."} {
		if m.Verify(tok) {
			t.Fatalf("мусор принят: %q", tok)
		}
	}
}

func TestTokenTampered(t *testing.T) {
	m := newTest()
	tok := m.NewToken()
	msg, sigHex, _ := strings.Cut(tok, ".")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatal(err)
	}
	sig[0] ^= 0xff
	bad := msg + "." + hex.EncodeToString(sig)
	if m.Verify(bad) {
		t.Fatal("подделанная подпись принята")
	}
	// Подмена срока годности при сохранённой подписи.
	if m.Verify("9999999999." + sigHex) {
		t.Fatal("токен с подменённым сроком принят")
	}
}

func TestTokenExpiry(t *testing.T) {
	m := newTest()
	now := time.Unix(1_700_000_000, 0)
	m.now = func() time.Time { return now }
	tok := m.NewToken()
	if !m.Verify(tok) {
		t.Fatal("токен должен быть жив до истечения")
	}
	m.now = func() time.Time { return now.Add(SessionTTL + time.Minute) }
	if m.Verify(tok) {
		t.Fatal("просроченный токен принят")
	}
}

func TestSecretSeparatesTokens(t *testing.T) {
	tok := New("p", "secret-a").NewToken()
	if New("p", "secret-b").Verify(tok) {
		t.Fatal("токен, подписанный другим секретом, принят")
	}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/auth/`
Expected: FAIL — `undefined: New`

- [ ] **Step 3: реализация auth.go**

```go
// Пакет auth: проверка пароля и подписанные cookie-сессии без серверного хранилища.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	CookieName = "session"
	SessionTTL = 7 * 24 * time.Hour
)

type Manager struct {
	passwordHash [32]byte
	secret       []byte
	now          func() time.Time // заменяется в тестах
}

func New(password, secret string) *Manager {
	return &Manager{
		passwordHash: sha256.Sum256([]byte(password)),
		secret:       []byte(secret),
		now:          time.Now,
	}
}

// CheckPassword сравнивает пароль в constant-time (через sha256, чтобы выровнять длину).
func (m *Manager) CheckPassword(password string) bool {
	h := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(h[:], m.passwordHash[:]) == 1
}

// NewToken возвращает "<expiry-unix>.<hmac-sha256(expiry)>".
func (m *Manager) NewToken() string {
	msg := strconv.FormatInt(m.now().Add(SessionTTL).Unix(), 10)
	return msg + "." + hex.EncodeToString(m.sign(msg))
}

// Verify проверяет подпись и срок годности токена.
func (m *Manager) Verify(token string) bool {
	msg, sigHex, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	exp, err := strconv.ParseInt(msg, 10, 64)
	if err != nil || m.now().Unix() > exp {
		return false
	}
	return subtle.ConstantTimeCompare(sig, m.sign(msg)) == 1
}

func (m *Manager) sign(msg string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/auth/`
Expected: PASS (6 тестов)

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat: auth — constant-time пароль и подписанные cookie-сессии
```

---

### Task 7: api — каркас, middleware, login/logout/me

Расширение относительно спеки: эндпоинт `GET /api/me` (`{"authenticated": bool}`) — нужен странице `/admin`, чтобы узнать, активна ли сессия, без пробной записи.

**Files:**
- Create: `internal/api/api.go`, `internal/api/auth_handlers.go`, `internal/api/api_test.go`

- [ ] **Step 1: тесты каркаса**

`internal/api/api_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"generator/internal/store"
)

// newTestServer поднимает api на httptest с паролем "pass123".
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false))
	t.Cleanup(ts.Close)
	return ts
}

// clientWithJar — клиент, хранящий cookie между запросами.
func clientWithJar() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func post(t *testing.T, c *http.Client, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestLoginWrongPassword(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	start := time.Now()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "nope"})
	elapsed := time.Since(start)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("нет паузы против перебора: %v", elapsed)
	}
}

func TestLoginSetsCookie(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	// cookiejar возвращает cookie без атрибутов (только Name/Value/Quoted),
	// поэтому HttpOnly проверяем по заголовку Set-Cookie ответа.
	var session *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "session" {
			session = ck
			break
		}
	}
	if session == nil {
		t.Fatal("cookie session не установлена")
	}
	if !session.HttpOnly {
		t.Fatal("cookie должна быть HttpOnly")
	}
	if len(c.Jar.Cookies(mustURL(t, ts.URL))) == 0 {
		t.Fatal("jar не сохранил cookie")
	}
}

func TestMeReflectsSession(t *testing.T) {
	ts := newTestServer(t)
	anon := clientWithJar()
	resp := get(t, anon, ts.URL+"/api/me")
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if body.Authenticated {
		t.Fatal("аноним не должен быть аутентифицирован")
	}

	authed := clientWithJar()
	resp2 := post(t, authed, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp2.Body.Close()
	resp3 := get(t, authed, ts.URL+"/api/me")
	var body2 struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(resp3.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if !body2.Authenticated {
		t.Fatal("после входа /api/me должен вернуть true")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp.Body.Close()
	resp = post(t, c, ts.URL+"/api/logout", map[string]any{})
	resp.Body.Close()
	resp = get(t, c, ts.URL+"/api/me")
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Authenticated {
		t.Fatal("после logout сессия должна быть сброшена")
	}
}

func TestMutationsRequireJSONContentType(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/login", "text/plain", bytes.NewBufferString(`{"password":"pass123"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, хотим 415", resp.StatusCode)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
```

Дополнить импорты теста: `"time"` (используется в TestLoginWrongPassword).

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/api/`
Expected: FAIL — `undefined: New`

- [ ] **Step 3: реализация api.go**

```go
// Пакет api: JSON API приложения.
package api

import (
	"encoding/json"
	"net/http"

	"generator/internal/auth"
	"generator/internal/store"
)

// maxBodyBytes — лимит тела запроса: на порядки больше реальных запросов.
const maxBodyBytes = 64 << 10

type Handler struct {
	st           *store.Store
	auth         *auth.Manager
	cookieSecure bool
}

// New собирает все /api-маршруты; main может добавить на этот же mux статику.
func New(st *store.Store, password, secret string, cookieSecure bool) *http.ServeMux {
	h := &Handler{st: st, auth: auth.New(password, secret), cookieSecure: cookieSecure}
	mux := http.NewServeMux()
	mux.Handle("POST /api/login", requireJSON(h.login))
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/me", h.me)
	mux.HandleFunc("GET /api/draws", h.listDraws)
	mux.Handle("POST /api/draws", h.session(requireJSON(h.createDraw)))
	mux.Handle("PUT /api/draws/{no}", h.session(requireJSON(h.updateDraw)))
	mux.Handle("DELETE /api/draws/{no}", h.session(h.deleteDraw))
	mux.Handle("POST /api/generate", requireJSON(h.generate))
	return mux
}

// requireJSON защищает мутации: CSRF-запрос из формы со стороннего сайта
// не сможет поставить Content-Type: application/json.
func requireJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			errorJSON(w, http.StatusUnsupportedMediaType, "Ожидается Content-Type: application/json")
			return
		}
		next(w, r)
	}
}

// session пропускает дальше только запросы с валидной cookie-сессией.
func (h *Handler) session(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName)
		if err != nil || !h.auth.Verify(c.Value) {
			errorJSON(w, http.StatusUnauthorized, "Требуется вход")
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON декодирует тело запроса; при ошибке сам пишет 400 и возвращает false.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		errorJSON(w, http.StatusBadRequest, "Некорректный JSON")
		return false
	}
	return true
}
```

- [ ] **Step 4: реализация auth_handlers.go**

```go
package api

import (
	"net/http"
	"time"

	"generator/internal/auth"
)

// wrongPasswordPause — пауза перед ответом 401, замедляет перебор пароля.
const wrongPasswordPause = time.Second

// setSessionCookie ставит (или при maxAge<0 удаляет) cookie-сессию.
func (h *Handler) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.cookieSecure,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !h.auth.CheckPassword(req.Password) {
		time.Sleep(wrongPasswordPause)
		errorJSON(w, http.StatusUnauthorized, "Неверный пароль")
		return
	}
	h.setSessionCookie(w, h.auth.NewToken(), int(auth.SessionTTL.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// me — есть ли у запроса валидная сессия (для показа админ-панели).
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	authed := false
	if c, err := r.Cookie(auth.CookieName); err == nil {
		authed = h.auth.Verify(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": authed})
}
```

- [ ] **Step 5: тесты зелёные**

Run: `go test ./internal/api/`
Expected: PASS (5 тестов). Если компилятор требует заглушки `h.listDraws`, `h.createDraw`, `h.updateDraw`, `h.deleteDraw`, `h.generate` — они появятся в Task 8–9; на этом шаге допустимо создать в `draws.go`/`generate.go` пустые файлы-хендлеры, которые будут заполнены следующими задачами:

```go
// internal/api/draws.go — временные заглушки, заполнятся в Task 8.
package api

import "net/http"

func (h *Handler) listDraws(w http.ResponseWriter, r *http.Request)   {}
func (h *Handler) createDraw(w http.ResponseWriter, r *http.Request)  {}
func (h *Handler) updateDraw(w http.ResponseWriter, r *http.Request)  {}
func (h *Handler) deleteDraw(w http.ResponseWriter, r *http.Request)  {}
```

```go
// internal/api/generate.go — временная заглушка, заполнится в Task 9.
package api

import "net/http"

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {}
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat: api — каркас, сессионный middleware, login/logout/me
```

---

### Task 8: api — CRUD розыгрышей

**Files:**
- Modify: `internal/api/draws.go` (заменить заглушки), `internal/api/api_test.go`

- [ ] **Step 1: тесты**

Добавить в `internal/api/api_test.go` (в конец файла):

```go
// doReq — запрос с произвольным методом и JSON-телом (тело может быть nil).
func doReq(t *testing.T, c *http.Client, method, url string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// loginClient возвращает клиент с активной сессией.
func loginClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	c := clientWithJar()
	resp := post(t, c, ts.URL+"/api/login", map[string]string{"password": "pass123"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	return c
}

func validCombo() map[string]any {
	return map[string]any{"drawNo": 12, "numbers": []int{7, 3, 34, 19, 26, 2, 14}, "bonus": 48}
}

func TestCreateDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var created struct {
		DrawNo  int64 `json:"drawNo"`
		Numbers []int `json:"numbers"`
		Bonus   int   `json:"bonus"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	if created.DrawNo != 12 || created.Bonus != 48 {
		t.Fatalf("created = %+v", created)
	}
	want := []int{2, 3, 7, 14, 19, 26, 34} // ответ с отсортированной семёркой
	for i := range want {
		if created.Numbers[i] != want[i] {
			t.Fatalf("numbers = %v", created.Numbers)
		}
	}
	// Публичное чтение видит созданный розыгрыш.
	resp2 := get(t, clientWithJar(), ts.URL+"/api/draws")
	var list struct {
		Draws []struct {
			DrawNo int64 `json:"drawNo"`
		} `json:"draws"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
	if len(list.Draws) != 1 || list.Draws[0].DrawNo != 12 {
		t.Fatalf("list = %+v", list.Draws)
	}
}

func TestCreateDrawRequiresSession(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, clientWithJar(), ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestCreateDrawDuplicate(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = post(t, c, ts.URL+"/api/draws", validCombo())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Error == "" {
		t.Fatal("ожидали сообщение об ошибке")
	}
}

func TestCreateDrawValidation(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	cases := []struct {
		name string
		body map[string]any
	}{
		{"мало чисел", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3}, "bonus": 8}},
		{"вне диапазона", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3, 4, 5, 6, 36}, "bonus": 8}},
		{"повтор числа", map[string]any{"drawNo": 1, "numbers": []int{1, 1, 3, 4, 5, 6, 7}, "bonus": 8}},
		{"бонус вне диапазона", map[string]any{"drawNo": 1, "numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 55}},
		{"нулевой номер", map[string]any{"drawNo": 0, "numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8}},
	}
	for _, tc := range cases {
		resp := post(t, c, ts.URL+"/api/draws", tc.body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", tc.name, resp.StatusCode)
		}
	}
}

func TestUpdateDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = doReq(t, c, "PUT", ts.URL+"/api/draws/12",
		map[string]any{"numbers": []int{9, 8, 7, 6, 5, 4, 3}, "bonus": 54})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp = doReq(t, c, "PUT", ts.URL+"/api/draws/999",
		map[string]any{"numbers": []int{1, 2, 3, 4, 5, 6, 7}, "bonus": 8})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: status = %d", resp.StatusCode)
	}
}

func TestDeleteDraw(t *testing.T) {
	ts := newTestServer(t)
	c := loginClient(t, ts)
	resp := post(t, c, ts.URL+"/api/draws", validCombo())
	resp.Body.Close()
	resp = doReq(t, c, "DELETE", ts.URL+"/api/draws/12", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	resp = doReq(t, c, "DELETE", ts.URL+"/api/draws/12", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("повторное удаление: status = %d", resp.StatusCode)
	}
}
```

Дополнить импорты `api_test.go`: `"io"`.

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/api/`
Expected: FAIL — заглушки не пишут ответов (статусы не совпадают)

- [ ] **Step 3: реализация — заменить draws.go целиком**

```go
package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"generator/internal/store"
)

type drawPayload struct {
	DrawNo  int64 `json:"drawNo"`
	Numbers []int `json:"numbers"`
	Bonus   int   `json:"bonus"`
}

func (h *Handler) listDraws(w http.ResponseWriter, r *http.Request) {
	draws, err := h.st.List()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось получить список розыгрышей")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draws": draws})
}

func (h *Handler) createDraw(w http.ResponseWriter, r *http.Request) {
	var p drawPayload
	if !readJSON(w, r, &p) {
		return
	}
	if p.DrawNo < 1 {
		errorJSON(w, http.StatusBadRequest, "Номер розыгрыша должен быть ≥ 1")
		return
	}
	if msg := validateDraw(p.Numbers, p.Bonus); msg != "" {
		errorJSON(w, http.StatusBadRequest, msg)
		return
	}
	sort.Ints(p.Numbers)
	err := h.st.Create(store.Draw{DrawNo: p.DrawNo, Numbers: p.Numbers, Bonus: p.Bonus})
	if errors.Is(err, store.ErrDuplicate) {
		errorJSON(w, http.StatusConflict,
			"Розыгрыш № "+strconv.FormatInt(p.DrawNo, 10)+" уже есть — отредактируйте его")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Не удалось сохранить розыгрыш")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) updateDraw(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.ParseInt(r.PathValue("no"), 10, 64)
	if err != nil || no < 1 {
		errorJSON(w, http.StatusBadRequest, "Некорректный номер розыгрыша")
		return
	}
	var p drawPayload
	if !readJSON(w, r, &p) {
		return
	}
	if msg := validateDraw(p.Numbers, p.Bonus); msg != "" {
		errorJSON(w, http.StatusBadRequest, msg)
		return
	}
	sort.Ints(p.Numbers)
	if err := h.st.Update(no, p.Numbers, p.Bonus); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			errorJSON(w, http.StatusNotFound, "Розыгрыш не найден")
		default:
			errorJSON(w, http.StatusInternalServerError, "Не удалось сохранить розыгрыш")
		}
		return
	}
	p.DrawNo = no
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) deleteDraw(w http.ResponseWriter, r *http.Request) {
	no, err := strconv.ParseInt(r.PathValue("no"), 10, 64)
	if err != nil || no < 1 {
		errorJSON(w, http.StatusBadRequest, "Некорректный номер розыгрыша")
		return
	}
	if err := h.st.Delete(no); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			errorJSON(w, http.StatusNotFound, "Розыгрыш не найден")
		default:
			errorJSON(w, http.StatusInternalServerError, "Не удалось удалить розыгрыш")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateDraw возвращает "" если комбинация корректна, иначе — сообщение пользователю.
func validateDraw(numbers []int, bonus int) string {
	if len(numbers) != 7 {
		return "Нужно ровно 7 основных чисел"
	}
	seen := make(map[int]struct{}, 7)
	for _, n := range numbers {
		if n < 1 || n > 35 {
			return "Основные числа должны быть в диапазоне 1–35"
		}
		if _, dup := seen[n]; dup {
			return "Основные числа не должны повторяться"
		}
		seen[n] = struct{}{}
	}
	if bonus < 1 || bonus > 54 {
		return "Бонусное число должно быть в диапазоне 1–54"
	}
	return ""
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/api/`
Expected: PASS (все тесты файла)

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat: api — CRUD розыгрышей с валидацией
```

---

### Task 9: api — генерация билетов; финальный main с сервером

**Files:**
- Modify: `internal/api/generate.go` (заменить заглушку), `internal/api/api_test.go`, `cmd/server/main.go`

- [ ] **Step 1: тесты generate-эндпоинта**

Добавить в `internal/api/api_test.go`:

```go
func TestGenerate(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, clientWithJar(), ts.URL+"/api/generate", map[string]any{"count": 3})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
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
	if len(body.Tickets) != 3 {
		t.Fatalf("билетов: %d", len(body.Tickets))
	}
	for _, tk := range body.Tickets {
		if len(tk.Numbers) != 7 {
			t.Fatalf("чисел в билете: %d", len(tk.Numbers))
		}
		prev := 0
		for _, n := range tk.Numbers {
			if n <= prev || n < 1 || n > 35 {
				t.Fatalf("семёрка не отсортирована/вне диапазона: %v", tk.Numbers)
			}
			prev = n
		}
		if tk.Bonus < 1 || tk.Bonus > 54 {
			t.Fatalf("бонус %d вне 1–54", tk.Bonus)
		}
	}
}

func TestGenerateValidation(t *testing.T) {
	ts := newTestServer(t)
	c := clientWithJar()
	for _, count := range []int{0, -1, 101} {
		resp := post(t, c, ts.URL+"/api/generate", map[string]any{"count": count})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("count=%d: status = %d", count, resp.StatusCode)
		}
	}
	// Пустая база — валидный режим: генерация равномерная, но работает.
	resp := post(t, c, ts.URL+"/api/generate", map[string]any{"count": 1})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("пустая база: status = %d", resp.StatusCode)
	}
}

func TestGenerateUsesHistory(t *testing.T) {
	// 50 розыгрышей, где всегда выпадает число 1 → самый частый номер
	// (вес 51 против 1 у остальных). ГСЧ crypto/rand, поэтому проверка
	// статистическая: в 10 билетах 1 почти наверняка встретится.
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for i := 1; i <= 50; i++ {
		if err := st.Create(drawStruct(int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	ts := httptest.NewServer(New(st, "pass123", "test-secret", false))
	t.Cleanup(ts.Close)
	resp := post(t, clientWithJar(), ts.URL+"/api/generate", map[string]any{"count": 10})
	defer resp.Body.Close()
	var body struct {
		Tickets []struct {
			Numbers []int `json:"numbers"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tk := range body.Tickets {
		for _, n := range tk.Numbers {
			if n == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("самое частое число 1 не встретилось ни в одном билете из 10")
	}
}

// drawStruct — розыгрыш с фиксированной комбинацией {1,2,3,4,5,6,7}+8.
func drawStruct(no int64) store.Draw {
	return store.Draw{DrawNo: no, Numbers: []int{1, 2, 3, 4, 5, 6, 7}, Bonus: 8}
}
```

- [ ] **Step 2: убедиться, что тесты падают**

Run: `go test ./internal/api/`
Expected: FAIL — заглушка generate не возвращает билеты

- [ ] **Step 3: реализация generate.go**

```go
package api

import (
	"net/http"

	"generator/internal/generate"
)

// maxTicketCount — верхняя граница пачки.
const maxTicketCount = 100

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Count < 1 || req.Count > maxTicketCount {
		errorJSON(w, http.StatusBadRequest, "Количество билетов — от 1 до 100")
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
	tickets := generate.GenerateBatch(mainFreq, bonusFreq, req.Count, generate.CryptoRand{})
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}
```

- [ ] **Step 4: тесты зелёные**

Run: `go test ./internal/api/`
Expected: PASS

- [ ] **Step 5: финальный main.go — подключить сервер**

Заменить в `cmd/server/main.go` функцию `main` и импорты на:

```go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"generator/internal/api"
	"generator/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	if cfg.SecretRandom {
		logger.Warn("SESSION_SECRET не задан — сгенерирован случайный, сессии слетят при перезапуске")
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("не удалось открыть базу", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	mux := api.New(st, cfg.Password, cfg.Secret, cfg.CookieSecure)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		logger.Info("слушаю", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http-сервер", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown", "err", err)
	}
}
```

- [ ] **Step 6: ручной smoke-прогон**

```bash
ADMIN_PASSWORD=test go run ./cmd/server &
SERVER_PID=$!
sleep 1
curl -s http://localhost:8080/api/draws            # {"draws":[]}
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H 'Content-Type: application/json' \
  -d '{"password":"test"}' http://localhost:8080/api/login   # 204
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"count":2}' http://localhost:8080/api/generate        # два билета
kill $SERVER_PID
```

Expected: `{"draws":[]}`, `204`, JSON с `"tickets":[...]`

- [ ] **Step 7: Commit**

```bash
git add cmd/server/ internal/api/
git commit -m "feat: эндпоинт генерации и запуск HTTP-сервера
```

---

### Task 10: web — парсер комбинации (TDD через node --test)

**Files:**
- Create: `package.json`, `web/parse.js`, `web/parse.test.mjs`

- [ ] **Step 1: package.json (чтобы Node трактовал web/*.js как ES-модули)**

`package.json` в корне репозитория:

```json
{
  "name": "generator-web",
  "private": true,
  "type": "module"
}
```

- [ ] **Step 2: тесты парсера**

`web/parse.test.mjs`:

```js
import test from 'node:test';
import assert from 'node:assert/strict';
import { parseCombination } from './parse.js';

test('пример из ТЗ: запятые и «и»', () => {
  assert.deepEqual(
    parseCombination('02, 19, 34, 07, 26, 03, 14 и 48'),
    { numbers: [2, 3, 7, 14, 19, 26, 34], bonus: 48 },
  );
});

test('разделитель — просто пробелы', () => {
  assert.deepEqual(
    parseCombination('2 3 7 14 19 26 34 48'),
    { numbers: [2, 3, 7, 14, 19, 26, 34], bonus: 48 },
  );
});

test('ведущие нули', () => {
  assert.deepEqual(
    parseCombination('01,02,03,04,05,06,07,54'),
    { numbers: [1, 2, 3, 4, 5, 6, 7], bonus: 54 },
  );
});

test('мало чисел — ошибка', () => {
  assert.match(parseCombination('1 2 3').error, /ровно 8 чисел/);
});

test('повтор в семёрке — ошибка', () => {
  assert.match(parseCombination('5 5 2 3 4 6 7 8').error, /повторяется/);
});

test('основное число вне 1–35 — ошибка', () => {
  assert.match(parseCombination('1 2 3 4 5 6 36 8').error, /вне диапазона 1–35/);
});

test('бонус вне 1–54 — ошибка', () => {
  assert.match(parseCombination('1 2 3 4 5 6 7 55').error, /вне диапазона 1–54/);
});

test('нечисловая строка — ошибка «мало чисел»', () => {
  assert.match(parseCombination('привет').error, /ровно 8 чисел/);
});
```

- [ ] **Step 3: убедиться, что тесты падают**

Run: `node --test web/parse.test.mjs`
Expected: FAIL — не удаётся импортировать `./parse.js`

- [ ] **Step 4: реализация parse.js**

```js
// Разбор строки с комбинацией вида «02, 19, 34, 07, 26, 03, 14 и 48».
// Разделителем считается любая подстрока без цифр: запятые, пробелы, «и» и т.п.
// Возвращает { numbers: [7 чисел по возрастанию], bonus } или { error: 'сообщение' }.
export function parseCombination(input) {
  const parts = String(input).split(/[^0-9]+/).filter((p) => p !== '');
  const nums = parts.map(Number);
  if (nums.length !== 8) {
    return { error: `Нужно ровно 8 чисел (7 основных и 1 бонус), найдено: ${nums.length}` };
  }
  const main = nums.slice(0, 7);
  const seen = new Set();
  for (const n of main) {
    if (n < 1 || n > 35) {
      return { error: `Число ${n} вне диапазона 1–35` };
    }
    if (seen.has(n)) {
      return { error: `Число ${n} повторяется — все семь должны быть разными` };
    }
    seen.add(n);
  }
  const bonus = nums[7];
  if (bonus < 1 || bonus > 54) {
    return { error: `Бонусное число ${bonus} вне диапазона 1–54` };
  }
  return { numbers: main.sort((a, b) => a - b), bonus };
}
```

- [ ] **Step 5: тесты зелёные**

Run: `node --test web/parse.test.mjs`
Expected: `pass 8`

- [ ] **Step 6: Commit**

```bash
git add package.json web/parse.js web/parse.test.mjs
git commit -m "feat: web — парсер комбинации с валидацией
```

---

### Task 11: web — общий помощник API и стили

> **Примечание:** листинги ниже — исходная версия из плана. После код-ревью в файлы внесены улучшения (коммиты `da720e1`, `5661685`, `+` фикс бонусного шара): русское сообщение при сетевой ошибке в `api()`, контраст `.ball.bonus` (переменная `--bonus-text: #1c1c28` для обеих тем), правило `.form-actions`, затемнённый `--muted`, переменная `--link` для тёмной темы, fallback `100vh` перед `100dvh`. Источник истины — файлы в репозитории.

**Files:**
- Create: `web/common.js`, `web/style.css`

- [ ] **Step 1: common.js**

```js
// Общие помощники фронтенда.

// api выполняет запрос к JSON API; 204 → null; не-2xx → Error с сообщением сервера.
export async function api(path, { method = 'GET', body } = {}) {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) {
    return null;
  }
  let data = null;
  try {
    data = await res.json();
  } catch {
    // тело пустое или не JSON — оставляем null
  }
  if (!res.ok) {
    throw new Error((data && data.error) || `Ошибка ${res.status}`);
  }
  return data;
}

// fmt приводит число к виду «02» для шаров.
export function fmt(n) {
  return String(n).padStart(2, '0');
}

// ball создаёт элемент-шар.
export function ball(n, extra = '') {
  const el = document.createElement('span');
  el.className = `ball${extra ? ' ' + extra : ''}`;
  el.textContent = fmt(n);
  return el;
}
```

- [ ] **Step 2: style.css**

```css
/* Мобильный-first, системный шрифт, светлая и тёмная темы. */
:root {
  --bg: #f5f6fa;
  --card: #ffffff;
  --text: #1c1c28;
  --muted: #6b7280;
  --accent: #7c3aed;
  --accent-text: #ffffff;
  --bonus: #f59e0b;
  --border: #e5e7eb;
  --danger: #dc2626;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #12121a;
    --card: #1e1e2a;
    --text: #f3f4f6;
    --muted: #9ca3af;
    --border: #2e2e3d;
    --danger: #f87171;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
}
main {
  flex: 1;
  width: 100%;
  max-width: 560px;
  margin: 0 auto;
  padding: 12px;
}
.topbar h1 {
  font-size: 1.05rem;
  margin: 0;
  padding: 14px 16px 10px;
  text-align: center;
}
.tabs {
  display: flex;
  gap: 6px;
  max-width: 560px;
  margin: 0 auto;
  padding: 0 12px;
}
.tab {
  flex: 1;
  min-height: 44px;
  padding: 12px;
  font-size: 1rem;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--card);
  color: var(--muted);
  cursor: pointer;
}
.tab.active {
  background: var(--accent);
  color: var(--accent-text);
  border-color: var(--accent);
}
.panel { display: none; margin-top: 12px; }
.panel.active { display: block; }
.card {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 16px;
  margin-bottom: 12px;
}
label { display: block; margin: 10px 0 6px; font-size: 0.95rem; }
input {
  width: 100%;
  min-height: 48px;
  padding: 12px;
  font-size: 1.05rem;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--bg);
  color: var(--text);
}
input:disabled { opacity: 0.55; }
.btn-primary, .btn-secondary, .btn-danger {
  min-height: 48px;
  padding: 0 18px;
  margin-top: 12px;
  font-size: 1rem;
  border: none;
  border-radius: 12px;
  cursor: pointer;
}
.btn-primary { width: 100%; background: var(--accent); color: var(--accent-text); }
.btn-secondary { background: var(--card); color: var(--text); border: 1px solid var(--border); }
.btn-danger { background: transparent; color: var(--danger); border: 1px solid var(--danger); }
.ticket {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 12px;
  margin-bottom: 10px;
}
.ball {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 0.95rem;
  background: var(--accent);
  color: var(--accent-text);
}
.ball.bonus { background: var(--bonus); }
.ball.small { width: 32px; height: 32px; font-size: 0.8rem; }
.ticket .ball.bonus { margin-left: auto; }
#tickets:empty::after { content: 'Билеты появятся здесь'; color: var(--muted); }
.draw-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 10px 12px;
  margin-bottom: 10px;
}
.draw-no { font-weight: 700; margin-right: 4px; }
.row-actions { display: flex; gap: 8px; width: 100%; margin-top: 8px; }
.row-actions button { flex: 1; min-height: 44px; margin-top: 0; }
.error { color: var(--danger); font-size: 0.95rem; }
.hint { color: var(--muted); font-size: 0.9rem; }
.hint a { color: var(--accent); }
footer { text-align: center; padding: 16px; }
footer a { color: var(--muted); }
```

- [ ] **Step 3: Commit**

```bash
git add web/common.js web/style.css
git commit -m "feat: web — API-помощники и мобильные стили
```

---

### Task 12: web — публичная страница (генератор + архив)

**Files:**
- Create: `web/index.html`, `web/app.js`

- [ ] **Step 1: index.html**

```html
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#7c3aed">
  <title>Fortunata — предсказатель</title>
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <header class="topbar"><h1>Fortunata · предсказатель</h1></header>

  <nav class="tabs" role="tablist" aria-label="Разделы">
    <button class="tab active" data-tab="generate" role="tab" aria-selected="true">Генератор</button>
    <button class="tab" data-tab="archive" role="tab" aria-selected="false">Архив</button>
  </nav>

  <main>
    <section id="tab-generate" class="panel active" role="tabpanel">
      <div class="card">
        <label for="count">Сколько билетов сгенерировать</label>
        <input id="count" type="number" inputmode="numeric" min="1" max="100" value="10">
        <button id="btn-generate" class="btn-primary">Сгенерировать</button>
        <p id="gen-error" class="error" hidden></p>
      </div>
      <div id="tickets"></div>
    </section>

    <section id="tab-archive" class="panel" role="tabpanel">
      <div id="draws-list"></div>
    </section>
  </main>

  <footer><a href="/admin">Редактировать розыгрыши</a></footer>
  <script type="module" src="/app.js"></script>
</body>
</html>
```

- [ ] **Step 2: app.js**

```js
import { api, ball } from './common.js';

// Вкладки
const tabs = document.querySelectorAll('.tab');
tabs.forEach((tab) => {
  tab.addEventListener('click', () => {
    tabs.forEach((t) => {
      const active = t === tab;
      t.classList.toggle('active', active);
      t.setAttribute('aria-selected', String(active));
    });
    document.querySelectorAll('.panel').forEach((p) => {
      p.classList.toggle('active', p.id === `tab-${tab.dataset.tab}`);
    });
    if (tab.dataset.tab === 'archive') {
      loadDraws();
    }
  });
});

// Генерация
const genError = document.getElementById('gen-error');
const ticketsBox = document.getElementById('tickets');

document.getElementById('btn-generate').addEventListener('click', async () => {
  genError.hidden = true;
  const count = Number(document.getElementById('count').value);
  try {
    const data = await api('/api/generate', { method: 'POST', body: { count } });
    ticketsBox.replaceChildren();
    for (const t of data.tickets) {
      const row = document.createElement('div');
      row.className = 'ticket';
      for (const n of t.numbers) {
        row.append(ball(n));
      }
      row.append(ball(t.bonus, 'bonus'));
      ticketsBox.append(row);
    }
  } catch (e) {
    genError.textContent = e.message;
    genError.hidden = false;
  }
});

// Архив
async function loadDraws() {
  const box = document.getElementById('draws-list');
  try {
    const data = await api('/api/draws');
    box.replaceChildren();
    if (data.draws.length === 0) {
      box.textContent = 'Розыгрышей пока нет — добавьте их через редактирование.';
      return;
    }
    for (const d of data.draws) {
      const row = document.createElement('div');
      row.className = 'draw-row';
      const no = document.createElement('span');
      no.className = 'draw-no';
      no.textContent = `№ ${d.drawNo}`;
      row.append(no);
      for (const n of d.numbers) {
        row.append(ball(n, 'small'));
      }
      row.append(ball(d.bonus, 'small bonus'));
      box.append(row);
    }
  } catch (e) {
    box.textContent = e.message;
  }
}
```

- [ ] **Step 3: Commit**

```bash
git add web/index.html web/app.js
git commit -m "feat: web — публичная страница с генератором и архивом
```

---

### Task 13: web — страница администрирования

**Files:**
- Create: `web/admin.html`, `web/admin.js`

- [ ] **Step 1: admin.html**

```html
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#7c3aed">
  <title>Fortunata — редактирование розыгрышей</title>
  <link rel="stylesheet" href="/style.css">
</head>
<body>
  <header class="topbar"><h1>Редактирование розыгрышей</h1></header>

  <main>
    <section id="login-card" class="card" hidden>
      <h2>Вход</h2>
      <label for="password">Пароль</label>
      <input id="password" type="password" autocomplete="current-password">
      <button id="btn-login" class="btn-primary">Войти</button>
      <p id="login-error" class="error" hidden></p>
    </section>

    <section id="panel" hidden>
      <div class="card">
        <h2>Добавить розыгрыш</h2>
        <label for="draw-no">Номер розыгрыша</label>
        <input id="draw-no" type="number" inputmode="numeric" min="1">
        <label for="combo">Комбинация: 7 чисел 1–35 и бонус 1–54</label>
        <input id="combo" type="text" inputmode="numeric"
               placeholder="02, 19, 34, 07, 26, 03, 14 и 48">
        <p class="hint">Откуда брать результаты:
          <a href="https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/"
             target="_blank" rel="noopener">timelottery.ru — архив розыгрышей Fortunata</a>
        </p>
        <div class="form-actions">
          <button id="btn-save" class="btn-primary">Добавить</button>
          <button id="btn-cancel" class="btn-secondary" hidden>Отмена</button>
        </div>
        <p id="form-error" class="error" hidden></p>
      </div>
      <div id="admin-draws"></div>
      <button id="btn-logout" class="btn-secondary">Выйти</button>
    </section>
  </main>

  <footer><a href="/">← К генератору</a></footer>
  <script type="module" src="/admin.js"></script>
</body>
</html>
```

- [ ] **Step 2: admin.js**

```js
import { api, ball, fmt } from './common.js';
import { parseCombination } from './parse.js';

const loginCard = document.getElementById('login-card');
const panel = document.getElementById('panel');
const loginError = document.getElementById('login-error');
const formError = document.getElementById('form-error');
const drawNoInput = document.getElementById('draw-no');
const comboInput = document.getElementById('combo');
const btnSave = document.getElementById('btn-save');
const btnCancel = document.getElementById('btn-cancel');
const list = document.getElementById('admin-draws');

let editNo = null; // null — добавление, число — редактирование существующего

init();

async function init() {
  try {
    const me = await api('/api/me');
    show(me.authenticated);
    if (me.authenticated) {
      refreshList();
    }
  } catch {
    show(false);
  }
}

function show(authed) {
  loginCard.hidden = authed;
  panel.hidden = !authed;
}

// Вход / выход
async function login() {
  loginError.hidden = true;
  try {
    await api('/api/login', {
      method: 'POST',
      body: { password: document.getElementById('password').value },
    });
    document.getElementById('password').value = '';
    show(true);
    refreshList();
  } catch (e) {
    loginError.textContent = e.message;
    loginError.hidden = false;
  }
}

document.getElementById('btn-login').addEventListener('click', login);
document.getElementById('password').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') login();
});

document.getElementById('btn-logout').addEventListener('click', async () => {
  await api('/api/logout', { method: 'POST', body: {} });
  show(false);
});

// Добавление / редактирование
btnSave.addEventListener('click', save);
btnCancel.addEventListener('click', () => setEditMode(null));

async function save() {
  formError.hidden = true;
  const parsed = parseCombination(comboInput.value);
  if (parsed.error) {
    formError.textContent = parsed.error;
    formError.hidden = false;
    return;
  }
  try {
    if (editNo === null) {
      await api('/api/draws', {
        method: 'POST',
        body: { drawNo: Number(drawNoInput.value), numbers: parsed.numbers, bonus: parsed.bonus },
      });
    } else {
      await api(`/api/draws/${editNo}`, {
        method: 'PUT',
        body: { numbers: parsed.numbers, bonus: parsed.bonus },
      });
    }
    comboInput.value = '';
    drawNoInput.value = '';
    setEditMode(null);
    refreshList();
  } catch (e) {
    formError.textContent = e.message;
    formError.hidden = false;
  }
}

function setEditMode(no) {
  editNo = no;
  drawNoInput.disabled = no !== null; // номер розыгрыша при редактировании заблокирован
  btnSave.textContent = no === null ? 'Добавить' : 'Сохранить';
  btnCancel.hidden = no === null;
}

// Список
async function refreshList() {
  try {
    const data = await api('/api/draws');
    list.replaceChildren();
    for (const d of data.draws) {
      list.append(drawRow(d));
    }
  } catch (e) {
    list.replaceChildren(e.message);
  }
}

function drawRow(d) {
  const row = document.createElement('div');
  row.className = 'draw-row';
  const no = document.createElement('span');
  no.className = 'draw-no';
  no.textContent = `№ ${d.drawNo}`;
  row.append(no);
  for (const n of d.numbers) {
    row.append(ball(n, 'small'));
  }
  row.append(ball(d.bonus, 'small bonus'));

  const actions = document.createElement('span');
  actions.className = 'row-actions';

  const edit = document.createElement('button');
  edit.className = 'btn-secondary';
  edit.textContent = 'Изменить';
  edit.addEventListener('click', () => {
    setEditMode(d.drawNo);
    drawNoInput.value = d.drawNo;
    comboInput.value = [...d.numbers, d.bonus].map(fmt).join(', ');
    comboInput.focus();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  });

  const del = document.createElement('button');
  del.className = 'btn-danger';
  del.textContent = 'Удалить';
  del.addEventListener('click', async () => {
    if (!confirm(`Удалить розыгрыш № ${d.drawNo}?`)) {
      return;
    }
    try {
      await api(`/api/draws/${d.drawNo}`, { method: 'DELETE' });
      if (editNo === d.drawNo) {
        setEditMode(null);
      }
      refreshList();
    } catch (e) {
      alert(e.message);
    }
  });

  actions.append(edit, del);
  row.append(actions);
  return row;
}
```

- [ ] **Step 3: Commit**

```bash
git add web/admin.html web/admin.js
git commit -m "feat: web — страница администрирования с входом по паролю
```

---

### Task 14: web — go:embed и раздача статики

**Files:**
- Create: `web/embed.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: web/embed.go**

```go
// Package web содержит статические файлы фронтенда, зашитые в бинарник.
package web

import "embed"

//go:embed index.html admin.html app.js admin.js common.js parse.js style.css
var Files embed.FS
```

- [ ] **Step 2: раздача в main.go**

В `cmd/server/main.go` добавить импорт `"generator/web"` (пакет лежит в корневом каталоге `web/`, не в internal/) и после строки `mux := api.New(...)`:

```go
	// Статика: / — index.html, /admin — админка, остальное — файлы из web/.
	mux.Handle("GET /", http.FileServerFS(web.Files))
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		b, err := web.Files.ReadFile("admin.html")
		if err != nil {
			http.Error(w, "admin.html не найден", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
```

Примечание: маршруты `/api/...` конкретнее шаблона `GET /`, поэтому конфликтов нет; `http.FileServerFS` для пути `/` сам отдаёт `index.html`.

- [ ] **Step 3: ручная проверка раздачи**

```bash
go build ./... && go vet ./...
ADMIN_PASSWORD=test DB_PATH=$(mktemp -u).db go run ./cmd/server &
SERVER_PID=$!
sleep 1
curl -s -o /dev/null -w '%{http_code} ' http://localhost:8080/            # 200
curl -s -o /dev/null -w '%{http_code} ' http://localhost:8080/admin       # 200
curl -s -o /dev/null -w '%{http_code} ' http://localhost:8080/style.css   # 200
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/app.js     # 200
kill $SERVER_PID
```

Expected: `200 200 200 200`

- [ ] **Step 4: Commit**

```bash
git add web/embed.go cmd/server/main.go
git commit -m "feat: раздача статики через go:embed
```

---

### Task 15: Docker — Dockerfile, compose.yaml, compose.local.yaml

**Files:**
- Create: `.dockerignore`, `Dockerfile`, `compose.yaml`, `compose.local.yaml`, `.env.example`

- [ ] **Step 1: .dockerignore**

```
.git
.claude
docs
data
*.db
*.db-wal
*.db-shm
.env
compose.yaml
compose.local.yaml
.env.example
Dockerfile
README.md
generator
```

- [ ] **Step 2: Dockerfile**

```dockerfile
# Сборка статического бинарника.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/generator ./cmd/server \
 && mkdir -p /out/data && chown 65532:65532 /out/data

# Минимальный рантайм: только бинарник и пустой /data с владельцем 65532
# (иначе named volume при первой инициализации получит root:root и БД не создастся).
FROM scratch
COPY --from=build /out/generator /generator
COPY --from=build --chown=65532:65532 /out/data /data
ENV DB_PATH=/data/generator.db ADDR=:8080
VOLUME /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/generator"]
```

- [ ] **Step 3: compose.yaml (прод, за внешним Traefik)**

```yaml
services:
  generator:
    build: .
    image: generator:latest
    restart: unless-stopped
    environment:
      ADMIN_PASSWORD: ${ADMIN_PASSWORD:?задайте ADMIN_PASSWORD в .env}
      SESSION_SECRET: ${SESSION_SECRET:?задайте SESSION_SECRET в .env}
      COOKIE_SECURE: ${COOKIE_SECURE:-true}
      DB_PATH: /data/generator.db
      ADDR: ":8080"
    volumes:
      - data:/data
    networks:
      - proxy
    labels:
      - traefik.enable=true
      - traefik.http.routers.generator.rule=Host(`${DOMAIN:?задайте DOMAIN в .env}`)
      - traefik.http.routers.generator.entrypoints=websecure
      - traefik.http.routers.generator.tls.certresolver=${TRAEFIK_CERTRESOLVER:?задайте TRAEFIK_CERTRESOLVER в .env}
      - traefik.http.services.generator.loadbalancer.server.port=8080
      - traefik.docker.network=${TRAEFIK_NETWORK:-traefik}

networks:
  proxy:
    external: true
    name: ${TRAEFIK_NETWORK:-traefik}

volumes:
  data:
```

- [ ] **Step 4: compose.local.yaml (локальный прогон без Traefik)**

```yaml
services:
  generator:
    build: .
    image: generator:local
    ports:
      - "8080:8080"
    environment:
      ADMIN_PASSWORD: test123
      SESSION_SECRET: local-test-secret
      COOKIE_SECURE: "false"
      DB_PATH: /data/generator.db
      ADDR: ":8080"
    volumes:
      - generator-local-data:/data

volumes:
  generator-local-data:
```

- [ ] **Step 5: .env.example**

```
# Домен сайта (обязательно)
DOMAIN=lottery.example.com

# Имя certresolver, настроенного в вашем Traefik (обязательно)
TRAEFIK_CERTRESOLVER=letsencrypt

# Имя внешней docker-сети, к которой подключён Traefik
TRAEFIK_NETWORK=traefik

# Пароль администратора (обязательно)
ADMIN_PASSWORD=

# Секрет подписи сессий; сгенерируйте: openssl rand -hex 32 (обязательно)
SESSION_SECRET=

# true за HTTPS-Traefik; для локального http-прогона — false
COOKIE_SECURE=true
```

- [ ] **Step 6: сборка и smoke через docker**

```bash
docker build -t generator:local .
docker run -d --rm --name gen-smoke -p 8081:8080 \
  -e ADMIN_PASSWORD=test -e SESSION_SECRET=s -e COOKIE_SECURE=false generator:local
sleep 1
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8081/   # 200
curl -s http://localhost:8081/api/draws                          # {"draws":[]}
docker stop gen-smoke
```

Expected: `200`, `{"draws":[]}`

- [ ] **Step 7: Commit**

```bash
git add .dockerignore Dockerfile compose.yaml compose.local.yaml .env.example
git commit -m "feat: Dockerfile и compose для Traefik и локального прогона
```

---

### Task 16: README.md

**Files:**
- Create: `README.md`

- [ ] **Step 1: README.md**

````markdown
# Fortunata — предсказатель

Генератор билетов для лотереи «Fortunata» (7 чисел из 1–35 + 1 число из 1–54)
и архив прошедших розыгрышей. Часто выпадавшие числа получают повышенный
приоритет при генерации (вес = число появлений + 1).

## Возможности

- **Генератор** — N билетов (по умолчанию 10, до 100), семёрка по возрастанию, бонус отдельно.
- **Архив** — публичный просмотр прошедших розыгрышей.
- **Редактирование** — добавление/изменение/удаление под паролем (без имён пользователей).
- Источник результатов: [timelottery.ru — архив розыгрышей Fortunata](https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/).

## Локальный запуск

### Вариант 1: локальный Go (нужен Go ≥ 1.24)

```bash
ADMIN_PASSWORD=test go run ./cmd/server
# http://localhost:8080
```

### Вариант 2: Docker (без Traefik)

```bash
docker compose -f compose.local.yaml up --build -d
# http://localhost:8080, пароль админа: test123
# сброс данных: docker compose -f compose.local.yaml down -v
```

## Тесты

```bash
go test ./...            # бэкенд
node --test web/parse.test.mjs   # парсер комбинации
```

## Деплой за Traefik

1. Скопируйте `.env.example` в `.env` и заполните:

   ```bash
   cp .env.example .env
   openssl rand -hex 32   # значение для SESSION_SECRET
   ```

2. Убедитесь, что внешняя сеть Traefik существует (имя из `TRAEFIK_NETWORK`):

   ```bash
   docker network create traefik   # если ещё нет
   ```

3. Запуск:

   ```bash
   docker compose up -d --build
   ```

Traefik должен иметь доступ к этой сети; роутер `generator` слушает домен
из `DOMAIN` на entrypoint `websecure` с certresolver из `TRAEFIK_CERTRESOLVER`.

## Переменные окружения

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `ADDR` | Адрес слушателя | `:8080` |
| `DB_PATH` | Путь к файлу SQLite | `generator.db` |
| `ADMIN_PASSWORD` | Пароль редактирования (обязателен) | — |
| `SESSION_SECRET` | Секрет подписи сессий; пусто — случайный | случайный |
| `COOKIE_SECURE` | Флаг Secure у cookie (true за HTTPS) | `false` |

## API

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| POST | `/api/login` | публично | `{password}` → cookie-сессия |
| POST | `/api/logout` | — | сброс сессии |
| GET | `/api/me` | публично | `{authenticated}` |
| GET | `/api/draws` | публично | список розыгрышей |
| POST | `/api/draws` | сессия | создать `{drawNo, numbers[7], bonus}` |
| PUT | `/api/draws/{no}` | сессия | заменить комбинацию |
| DELETE | `/api/draws/{no}` | сессия | удалить |
| POST | `/api/generate` | публично | `{count}` → `{tickets}` |
````

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README с запуском, тестами и деплоем
```

---

### Task 17: Интеграционная проверка (мобильный вьюпорт, Playwright)

Проверка всего сценария на собранном `compose.local.yaml` через браузер.
Инструмент: Playwright MCP (`browser_navigate`, `browser_resize`, `browser_snapshot`,
`browser_click`, `browser_type`, `browser_take_screenshot`). Скриншоты сохранять
в `docs/screenshots/` и закоммитить.

- [ ] **Step 1: поднять приложение**

```bash
docker compose -f compose.local.yaml up --build -d
sleep 2
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/   # 200
```

- [ ] **Step 2: браузер в мобильном вьюпорте**

Playwright: `browser_resize` → width 390, height 844; `browser_navigate` → `http://localhost:8080`.

- [ ] **Step 3: сценарий генератора**

1. Нажать «Сгенерировать» (количество 10 по умолчанию).
2. Проверить: 10 билетов, в каждом 7 шаров по возрастанию + жёлтый бонус.
3. Скриншот `docs/screenshots/01-generator.png`.

- [ ] **Step 4: сценарий архива (пустой)**

1. Вкладка «Архив» → текст «Розыгрышей пока нет…».

- [ ] **Step 5: сценарий администрирования**

1. Перейти на `/admin`.
2. Ввести неверный пароль → сообщение «Неверный пароль» (занимает ~1 с).
3. Ввести `test123` → панель открылась.
4. Добавить розыгрыш: номер `777`, комбинация `02, 19, 34, 07, 26, 03, 14 и 48` → появился в списке отсортированным.
5. Попробовать добавить дубликат № 777 → ошибка «уже есть — отредактируйте».
6. «Изменить» у № 777 → поле номера заблокировано, форма заполнена; поменять бонус на 53 → «Сохранить» → в списке 53.
7. Добавить второй розыгрыш № 778 с комбинацией `1 2 3 4 5 6 7 8` → виден в списке.
8. Скриншот `docs/screenshots/02-admin.png`.
9. Удалить № 778 (с подтверждением) → исчез из списка.
10. «Выйти» → снова форма входа.

- [ ] **Step 6: сценарий генерации с историей**

1. Вернуться на `/`, вкладка «Архив» → розыгрыш № 777 виден.
2. «Генератор» → «Сгенерировать» → билеты сгенерированы без ошибок.
3. Скриншот `docs/screenshots/03-generator-with-history.png`.

- [ ] **Step 7: финальный прогон всех тестов**

```bash
go test ./... && go vet ./... && node --test web/parse.test.mjs
docker compose -f compose.local.yaml down -v
```

Expected: все `ok`, node — `pass 8`

- [ ] **Step 8: Commit**

```bash
git add docs/screenshots/
git commit -m "test: интеграционная проверка в мобильном вьюпорте, скриншоты
```

---

## Самопроверка плана (выполнена при написании)

**Покрытие спеки:**
- Архив (просмотр/добавление/редактирование/удаление, пароль) — Tasks 2, 3, 7, 8, 13.
- Генератор с частотными весами — Tasks 4, 5, 9, 12.
- Подсказка-источник timelottery — Task 13 (`admin.html`).
- Формат ввода «02, 19, … и 48» — Task 10.
- Мобильная оптимизация — Task 11 (стили), Task 17 (проверка 390×844).
- SQLite, WAL, volume — Tasks 2, 15.
- Docker/compose/Traefik labels, .env — Task 15.
- Сессии: HMAC-cookie, SameSite=Strict, 7 дней, constant-time пароль, пауза при переборе — Tasks 6, 7.
- `GET /api/me` — добавление сверх спеки (нужно странице `/admin` для определения состояния входа); указано в Task 7.

**Согласованность типов:** `store.Draw` (json-теги `drawNo/numbers/bonus/createdAt/updatedAt`) ↔ `api.drawPayload`; `generate.DrawFreq`/`Ticket`/`RNG` ↔ использование в `api.generate`; `auth.Manager{CheckPassword,NewToken,Verify}` ↔ `api.Handler.session`; `api.New(st, password, secret, cookieSecure) *http.ServeMux` ↔ `main.go` и тесты.

**Известные места, где исполнитель может увидеть расхождение с реальностью:**
- Точная версия `modernc.org/sqlite@latest` на момент выполнения — тест `TestCreateDuplicate` фиксирует текст ошибки `UNIQUE constraint failed: draws.draw_no`; если текст у драйвера изменится, правится одна строка в `store.Create`.
