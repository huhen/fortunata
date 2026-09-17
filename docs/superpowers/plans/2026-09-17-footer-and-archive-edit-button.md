# Кнопка редактирования на Архиве + общий футер — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Перенести ссылку «Редактировать розыгрыши» на вкладку «Архив» и добавить обеим страницам общий футер (версия, GitHub, «наверх»).

**Architecture:** Версия — пакетная переменная `internal/version.Version` (дефолт `dev`), прошиваемая `-ldflags -X` в release.yml и Dockerfile, выдаётся публичной ручкой `GET /api/version`. Футер — одинаковая статичная разметка в `index.html` и `admin.html`, оживляемая общим `web/footer.js`. Спека: `docs/superpowers/specs/2026-09-17-footer-and-archive-edit-button-design.md`.

**Tech Stack:** Go 1.26 (net/http, httptest), ванильный JS-модули, static embed.

**Соглашения проекта** (из CLAUDE.md): комментарии в коде на русском; статика зашита через `go:embed` — после правок `web/` сервер надо перезапускать; числа/строки в UI — на русском.

---

### Task 1: Пакет `internal/version` и ручка `GET /api/version`

**Files:**
- Create: `internal/version/version.go`
- Create: `internal/api/version.go`
- Modify: `internal/api/api.go` (в `register`, после строки `mux.HandleFunc("GET /api/stats", h.stats)`)
- Test: `internal/api/version_test.go`

- [ ] **Step 1: Написать падающий тест**

Создать `internal/api/version_test.go` (в пакете уже есть хелперы `newTestServer` и `get` в `api_test.go` — используются они, ничего нового не заводить):

```go
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
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./internal/api/ -run TestGetVersion -v`
Expected: ошибка компиляции — пакет `fortunata/internal/version` не существует.

- [ ] **Step 3: Создать пакет version**

Создать `internal/version/version.go`:

```go
// Пакет version: версия сборки, прошиваемая при сборке через
// -ldflags "-X fortunata/internal/version.Version=...".
package version

// Version — версия сборки; "dev" для локального go run и go test.
var Version = "dev"
```

- [ ] **Step 4: Добавить хендлер и маршрут**

Создать `internal/api/version.go` (по образцу `stats.go`):

```go
package api

import (
	"net/http"

	"fortunata/internal/version"
)

// getVersion — публичная ручка с версией сборки для футера фронтенда.
func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version.Version})
}
```

В `internal/api/api.go` в методе `register` после строки `mux.HandleFunc("GET /api/stats", h.stats)` добавить:

```go
	mux.HandleFunc("GET /api/version", h.getVersion)
```

- [ ] **Step 5: Убедиться, что тест проходит**

Run: `go test ./internal/api/ -run TestGetVersion -v`
Expected: `PASS`, `TestGetVersion`.

- [ ] **Step 6: Прогнать весь бэкенд-набор**

Run: `go vet ./... && go test ./...`
Expected: нет ошибок, все пакеты `ok`.

- [ ] **Step 7: Коммит**

```bash
git add internal/version/version.go internal/api/version.go internal/api/version_test.go internal/api/api.go
git commit -m "feat: ручка GET /api/version — версия из пакета internal/version"
```

---

### Task 2: Кнопка на Архиве, общий футер, footer.js, стили

**Files:**
- Modify: `web/index.html` (строки 30–32 — `#tab-archive`; строка 39 — футер; строка 40 — скрипты)
- Modify: `web/admin.html` (строка 47 — футер; строка 48 — скрипты)
- Modify: `web/style.css` (строки 151–152 — заменить стили `footer`)
- Create: `web/footer.js`

- [ ] **Step 1: index.html — кнопка в `#tab-archive`**

Секцию архив-вкладки заменить на:

```html
    <section id="tab-archive" class="panel" role="tabpanel">
      <a class="btn-secondary" href="/admin">Редактировать розыгрыши</a>
      <div id="draws-list"></div>
    </section>
```

- [ ] **Step 2: index.html — общий футер вместо старого**

Строку `<footer><a href="/admin">Редактировать розыгрыши</a></footer>` заменить на:

```html
  <footer class="site-footer">
    <span id="app-version" hidden></span>
    <a href="https://github.com/huhen/fortunata" target="_blank" rel="noopener">GitHub</a>
    <button id="btn-top" type="button">↑ Наверх</button>
  </footer>
```

И сразу после `<script type="module" src="/app.js"></script>` добавить:

```html
  <script type="module" src="/footer.js"></script>
```

- [ ] **Step 3: admin.html — ссылка назад над общим футером**

Строку `<footer><a href="/">← К генератору</a></footer>` заменить на:

```html
  <p class="back-link"><a href="/">← К генератору</a></p>
  <footer class="site-footer">
    <span id="app-version" hidden></span>
    <a href="https://github.com/huhen/fortunata" target="_blank" rel="noopener">GitHub</a>
    <button id="btn-top" type="button">↑ Наверх</button>
  </footer>
```

И сразу после `<script type="module" src="/admin.js"></script>` добавить:

```html
  <script type="module" src="/footer.js"></script>
```

- [ ] **Step 4: style.css — заменить стили старого футера**

Строки:

```css
footer { text-align: center; padding: 16px; }
footer a { color: var(--muted); }
```

заменить на:

```css
a.btn-secondary {
  display: flex;
  align-items: center;
  justify-content: center;
  text-decoration: none;
  margin-top: 0;
  margin-bottom: 12px;
}
.back-link { text-align: center; padding: 0 16px; }
.back-link a { color: var(--muted); }
.site-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 6px 18px;
  padding: 16px;
  font-size: 0.9rem;
}
.site-footer a,
.site-footer button { color: var(--muted); }
.site-footer button {
  background: none;
  border: none;
  padding: 0;
  font: inherit;
  cursor: pointer;
}
```

(`a.btn-secondary` перекрывает `margin-top: 12px` от `.btn-secondary` и центрирует текст якоря; CSP запрещает инлайн-стили, поэтому `.back-link` — класс.)

- [ ] **Step 5: Создать footer.js**

Создать `web/footer.js` (общий для обеих страниц; `common.js` не трогаем — он остаётся чистыми хелперами):

```js
// Общий футер: версия приложения и кнопка «наверх».

// Версия: элемент скрыт, пока ответа нет; при сбое остаётся скрытым.
const versionEl = document.getElementById('app-version');
if (versionEl) {
  try {
    const res = await fetch('/api/version');
    if (res.ok) {
      const data = await res.json();
      versionEl.textContent = data.version;
      versionEl.hidden = false;
    }
  } catch {
    // нет соединения — версия просто не показывается
  }
}

// «Наверх»: плавная прокрутка к началу страницы.
document.getElementById('btn-top')?.addEventListener('click', () => {
  window.scrollTo({ top: 0, behavior: 'smooth' });
});
```

- [ ] **Step 6: Ручная проверка через запущенный сервер**

Run (в двух терминалах или через `run_in_background`; для go run:
`ADMIN_PASSWORD=test go run ./cmd/server`):

```bash
curl -s http://localhost:8080/api/version
# Expected: {"version":"dev"}

curl -s http://localhost:8080/ | grep -c "site-footer\|btn-top\|app-version"
# Expected: вхождения разметки футера

curl -s http://localhost:8080/admin | grep -c "site-footer\|back-link"
# Expected: вхождения разметки футера и ссылки назад
```

В браузере: на вкладке «Архив» видна кнопка «Редактировать розыгрыши», футер
на обеих страницах, «↑ Наверх» прокручивает, версия показывается. Статику
проверять только на пересобранном/перезапущенном сервере (embed!). Если
браузера нет — curl-проверки достаточны, визуальную часть финализирует Task 4.

- [ ] **Step 7: Коммит**

```bash
git add web/index.html web/admin.html web/style.css web/footer.js
git commit -m "feat: общий футер (версия, GitHub, наверх) и кнопка редактирования на Архиве"
```

---

### Task 3: Прошивка версии в сборки (Dockerfile, release.yml)

**Files:**
- Modify: `Dockerfile` (секция build: после `FROM golang:1.26-alpine AS build`, RUN-шаг сборки)
- Modify: `.github/workflows/release.yml` (шаг «Сборка бинарника», шаг «Публикация образа в GHCR»)

- [ ] **Step 1: Dockerfile**

После строки `FROM golang:1.26-alpine AS build` добавить:

```dockerfile
ARG VERSION=dev
```

RUN-шаг сборки заменить на:

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X fortunata/internal/version.Version=${VERSION}" -o /out/fortunata ./cmd/server \
 && mkdir -p /out/data && chown 65532:65532 /out/data
```

- [ ] **Step 2: release.yml — бинарь**

Шаг «Сборка бинарника linux/amd64» заменить на (переменная `VERSION` уже в `GITHUB_ENV` с предыдущего шага):

```yaml
      - name: Сборка бинарника linux/amd64
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -trimpath -ldflags "-s -w -X fortunata/internal/version.Version=${VERSION}" \
            -o dist/fortunata-linux-amd64 ./cmd/server
```

- [ ] **Step 3: release.yml — docker build-args**

В шаге «Публикация образа в GHCR» (`docker/build-push-action@v7`) добавить `build-args` (после `context:`):

```yaml
          build-args: |
            VERSION=${{ env.VERSION }}
```

- [ ] **Step 4: Локальная проверка прошивки**

```bash
go build -trimpath -ldflags "-X fortunata/internal/version.Version=v2026.09.17-TEST" -o /tmp/fortunata-ver ./cmd/server
ADMIN_PASSWORD=test ADDR=:8091 DB_PATH=/tmp/fortunata-ver.db /tmp/fortunata-ver &
sleep 1
curl -s http://localhost:8091/api/version
# Expected: {"version":"v2026.09.17-TEST"}
kill %1 && rm -f /tmp/fortunata-ver /tmp/fortunata-ver.db*
```

- [ ] **Step 5: Коммит**

```bash
git add Dockerfile .github/workflows/release.yml
git commit -m "build: прошивка версии через ldflags в Dockerfile и release"
```

---

### Task 4: Финальная проверка

**Files:** без изменений кода.

- [ ] **Step 1: Полный CI-набор локально**

```bash
go vet ./...
go test ./...
node --test web/parse.test.mjs
```

Expected: vet чисто, тесты всех пакетов `ok`, парсер-тесты зелёные.

- [ ] **Step 2: Сквозная ручная проверка**

Запустить `ADMIN_PASSWORD=test go run ./cmd/server` и в браузере проверить:
1. Вкладка «Архив»: сверху кнопка «Редактировать розыгрыши» → ведёт на `/admin`.
2. Обе страницы: внизу футер — версия (`dev`), ссылка GitHub
   (`https://github.com/huhen/fortunata`), «↑ Наверх» прокручивает страницу.
3. На `/admin` над футером есть «← К генератору».
4. Все элементы футера помещаются в одну строку при мобильной ширине.

- [ ] **Step 3: Итог**

Дальше — завершение ветки по superpowers:finishing-a-development-branch
(по памяти пользователя: сразу пуш и PR, без меню выбора).
