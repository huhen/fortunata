# Переименование generator → fortunata: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить все упоминания «generator» на «fortunata» (модуль, код, Docker, compose, CI, документация, скриншоты) и выпустить релиз через слитый PR.

**Architecture:** Механическое переименование без изменения логики. Спецификация: `docs/superpowers/specs/2026-09-16-fortunata-rename-design.md`. Работа идёт в ветке `rename-fortunata`, финал — squash-PR в `main`; мерж триггерит `release.yml`, который сам создаёт релиз и публикует образ. Это работа-переименование: новых тестов не пишем, регрессию проверяем существующими наборами (`go test ./...`, `node --test`) и grep-верификацией.

**Tech Stack:** Go 1.25, SQLite (modernc.org/sqlite), vanilla JS, Docker Compose, GitHub Actions, `gh` CLI.

**Важный контекст для исполнителя:**
- Git-пуш по SSH в этой машине не работает: `origin` переводится на HTTPS с credential-helper от `gh` (шаг уже выполнен: `gh auth setup-git`).
- Ветка `main` защищена правилом «только через PR» — вся работа в ветке, мерж squash-ом.
- `sed -i 's|generator|fortunata|g'` безопасен в файлах этого плана: кириллическое «генератор» и пакет `internal/generate` не содержат подстроки `generator`.
- Исключение из замен — спека и сам этот план (мета-документы).

---

### Task 1: Подготовка — origin на HTTPS и рабочая ветка

**Files:** без изменений файлов.

- [ ] **Step 1: Перевести origin на HTTPS**

```bash
git remote set-url origin https://github.com/huhen/fortunata.git
git remote -v
```

Expected: обе строки `origin` показывают `https://github.com/huhen/fortunata.git`.

- [ ] **Step 2: Обновить main и создать ветку**

```bash
git switch main
git pull origin main
git switch -c rename-fortunata
```

Expected: `main` актуальна (последний коммит — `d2f5996` или новее), ветка `rename-fortunata` создана.

### Task 2: Go-модуль и код

**Files:**
- Modify: `go.mod:1`
- Modify: `cmd/server/main.go` (импорты:18-20, дефолт БД:30,42)
- Modify: `cmd/server/main_test.go:52`
- Modify: `internal/api/api.go:8-9`, `internal/api/draws.go:9`, `internal/api/auth_handlers.go:7`, `internal/api/generate.go:6`, `internal/api/api_test.go:14`

- [ ] **Step 1: Заменить имя модуля**

```bash
sed -i 's|^module generator$|module fortunata|' go.mod
head -1 go.mod
```

Expected: `module fortunata`.

- [ ] **Step 2: Заменить пути импортов**

```bash
sed -i 's|"generator/internal|"fortunata/internal|g; s|"generator/web"|"fortunata/web"|' \
  cmd/server/main.go \
  internal/api/api.go internal/api/draws.go \
  internal/api/auth_handlers.go internal/api/generate.go \
  internal/api/api_test.go
grep -rn '"generator' cmd internal web
```

Expected: grep пуст (ничего не вывел).

- [ ] **Step 3: Заменить дефолт имени БД**

```bash
sed -i 's|generator\.db|fortunata.db|g' cmd/server/main.go cmd/server/main_test.go
grep -rn 'generator' cmd internal
```

Expected: grep пуст.

- [ ] **Step 4: Сборка, vet, тесты**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: всё молча зелёное; тесты — строки вида `ok  fortunata/cmd/server` (имя модуля новое в выводе).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: go-модуль и дефолт БД — generator → fortunata"
```

### Task 3: Dockerfile, .gitignore, package.json

**Files:**
- Modify: `Dockerfile` (бинарник:10, COPY:17, ENV:19, ENTRYPOINT:22)
- Modify: `.gitignore:9` (`/generator` — игнор локального бинарника)
- Modify: `.dockerignore:14` (`generator` — запись локального бинарника)
- Modify: `package.json:2`

- [ ] **Step 1: Заменить в Dockerfile**

```bash
sed -i 's|generator|fortunata|g' Dockerfile
grep -n 'fortunata\|generator' Dockerfile
```

Expected: `/out/fortunata`, `/fortunata` (COPY и ENTRYPOINT), `DB_PATH=/data/fortunata.db`; строк с `generator` нет.

- [ ] **Step 2: Заменить в .gitignore, .dockerignore и package.json**

```bash
sed -i 's|^/generator$|/fortunata|' .gitignore
sed -i 's|^generator$|fortunata|' .dockerignore
sed -i 's|"name": "generator-web"|"name": "fortunata-web"|' package.json
grep -rn -i 'generator' .gitignore .dockerignore package.json Dockerfile
```

Expected: grep пуст.

- [ ] **Step 3: Commit**

```bash
git add .gitignore .dockerignore Dockerfile package.json
git commit -m "build: Dockerfile, .gitignore, .dockerignore, package.json — generator → fortunata"
```

### Task 4: Compose-файлы и сборка образа

**Files:**
- Modify: `compose.yaml` (сервис:2, DB_PATH:15, traefik-метки:23-26)
- Modify: `compose.local.yaml` (сервис:2, image:4, DB_PATH:11, volume:14,17)

- [ ] **Step 1: Заменить в обоих файлах**

```bash
sed -i 's|generator|fortunata|g' compose.yaml compose.local.yaml
grep -n 'generator' compose.yaml compose.local.yaml
```

Expected: grep пуст. В `compose.local.yaml` появились `image: fortunata:local` и volume `fortunata-local-data` (ссылка и объявление); в `compose.yaml` — метки `traefik.http.routers.fortunata.*`, `traefik.http.services.fortunata.*`.

- [ ] **Step 2: Валидация и сборка образа**

```bash
docker compose -f compose.local.yaml config --quiet && echo CONFIG_OK
docker compose -f compose.local.yaml build
```

Expected: `CONFIG_OK`; сборка проходит, образ `fortunata:local` создан.

- [ ] **Step 3: Commit**

```bash
git add compose.yaml compose.local.yaml
git commit -m "deploy: compose — сервис, метки traefik, volume и БД fortunata"
```

### Task 5: CI-воркфлоу и основная документация

**Files:**
- Modify: `.github/workflows/release.yml:43,61` (имя артефакта)
- Modify: `README.md:60,68,70,86`
- Modify: `CLAUDE.md:30`

- [ ] **Step 1: Заменить в release.yml**

```bash
sed -i 's|generator-linux-amd64|fortunata-linux-amd64|g' .github/workflows/release.yml
grep -n 'linux-amd64' .github/workflows/release.yml
```

Expected: обе строки (`-o dist/fortunata-linux-amd64` и `files: dist/fortunata-linux-amd64`).

- [ ] **Step 2: Заменить в README.md и CLAUDE.md**

```bash
sed -i 's|generator|fortunata|g' README.md CLAUDE.md
grep -n -i 'generator' README.md CLAUDE.md
```

Expected: grep пуст. В README: роутер `fortunata`, `/data/fortunata.db`, `fortunata-db.tar.gz`, дефолт `DB_PATH` `fortunata.db`. В CLAUDE.md: дефолт `DB_PATH` (`fortunata.db`).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml README.md CLAUDE.md
git commit -m "ci+docs: релизный артефакт и документация — fortunata"
```

### Task 6: Исторические доки и скриншоты (фиксированный список!)

**Files:**
- Modify: `docs/superpowers/plans/2026-09-15-lottery-predictor.md` (~50 вхождений; сюда же входят 2 ссылки на скриншоты)
- Modify: `docs/superpowers/specs/2026-09-15-lottery-predictor-design.md:29,58`
- Modify: `docs/superpowers/specs/2026-09-16-github-release-workflow-design.md:62,67,79`
- Modify: `docs/superpowers/plans/2026-09-16-github-release-workflow.md` (8 вхождений)
- Rename: `docs/screenshots/01-generator.png` → `01-fortunata.png`; `docs/screenshots/03-generator-with-history.png` → `03-fortunata-with-history.png`

**ЗАПРЕЩЕНО:** выполнять sed по всей `docs/` или `find … -exec sed` — только четыре файла из списка. Спека `2026-09-16-fortunata-rename-design.md` и этот план в замену не входят.

- [ ] **Step 1: Заменить в четырёх файлах по списку**

```bash
sed -i 's|generator|fortunata|g' \
  docs/superpowers/plans/2026-09-15-lottery-predictor.md \
  docs/superpowers/plans/2026-09-16-github-release-workflow.md \
  docs/superpowers/specs/2026-09-15-lottery-predictor-design.md \
  docs/superpowers/specs/2026-09-16-github-release-workflow-design.md
```

- [ ] **Step 2: Переименовать скриншоты**

```bash
git mv docs/screenshots/01-generator.png docs/screenshots/01-fortunata.png
git mv docs/screenshots/03-generator-with-history.png docs/screenshots/03-fortunata-with-history.png
ls docs/screenshots/
```

Expected: `01-fortunata.png  02-admin.png  03-fortunata-with-history.png`.

- [ ] **Step 3: Проверить документацию**

```bash
grep -rn -i 'generator' docs/ -l
grep -n 'screenshots/0' docs/superpowers/plans/2026-09-15-lottery-predictor.md
```

Expected: grep находит только `docs/superpowers/specs/2026-09-16-fortunata-rename-design.md` и `docs/superpowers/plans/2026-09-17-fortunata-rename.md`; ссылки на скриншоты в плане — `01-fortunata.png` и `03-fortunata-with-history.png`.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: исторические планы/спеки и скриншоты — generator → fortunata"
```

### Task 7: Финальная верификация

- [ ] **Step 1: Grep по всему репозиторию**

```bash
grep -rn -i 'generator' . --exclude-dir=.git -l
```

Expected: ровно два файла — спека `2026-09-16-fortunata-rename-design.md` и план `2026-09-17-fortunata-rename.md`.

- [ ] **Step 2: Полный тестовый прогон**

```bash
go build ./... && go vet ./... && go test ./... && node --test web/parse.test.mjs
```

Expected: всё зелёное.

- [ ] **Step 3: Дымовой прогон бинарника (дефолтное имя БД)**

```bash
go build -o /tmp/fortunata-smoke ./cmd/server
ADMIN_PASSWORD=test /tmp/fortunata-smoke & SMOKE_PID=$!
sleep 1
curl -sf -o /dev/null http://localhost:8080/ && echo HTTP_OK
test -f fortunata.db && echo DB_DEFAULT_OK
kill $SMOKE_PID
rm -f fortunata.db fortunata.db-wal fortunata.db-shm /tmp/fortunata-smoke
```

Expected: `HTTP_OK` и `DB_DEFAULT_OK` (файл БД по умолчанию — `fortunata.db`; попадает под `*.db` в `.gitignore`).

- [ ] **Step 4: Дымовой прогон контейнера**

```bash
docker compose -f compose.local.yaml up -d --build
sleep 2 && curl -sf -o /dev/null http://localhost:8080/ && echo CONTAINER_OK
docker compose -f compose.local.yaml down
```

Expected: `CONTAINER_OK`; при первом старте в новом volume `fortunata-local-data` создаётся `/data/fortunata.db`.

### Task 8: PR, мерж, релиз

- [ ] **Step 1: Запушить ветку и создать PR**

```bash
git push -u origin rename-fortunata
gh pr create --title "refactor: переименование проекта generator → fortunata" --body "Заменяет все упоминания «generator» на «fortunata»: go-модуль и импорты, дефолт БД, Dockerfile, compose (сервис, traefik-метки, volume), релизный артефакт, package.json, .gitignore, документация, исторические доки и скриншоты. Релиз создаётся пайплайном при мерже."
```

- [ ] **Step 2: Дождаться зелёного CI**

```bash
gh pr checks --watch
```

Expected: оба чека (`test`, `docker`) — pass.

- [ ] **Step 3: Смержить squash-ом**

```bash
gh pr merge --squash --delete-branch
git switch main && git pull origin main
```

Expected: PR смержен; в `main` коммит вида `refactor: переименование проекта generator → fortunata (#N)`; релизный воркфлоу стартовал.

- [ ] **Step 4: Дождаться релизный воркфлоу и проверить релиз**

```bash
SHA=$(git rev-parse --short HEAD)
gh run watch $(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId') --exit-status
gh release view "v$(date -u +%Y.%m.%d)-$SHA" --json name,assets --jq '{name, assets: [.assets[].name]}'
docker manifest inspect ghcr.io/huhen/fortunata:latest > /dev/null && echo IMAGE_OK
```

Expected: воркфлоу зелёный; релиз `v2026.09.17-<sha>` с артефактом `fortunata-linux-amd64`; `IMAGE_OK`.

### Task 9: Пост-релиз — статус спеки

**Files:**
- Modify: `docs/superpowers/specs/2026-09-16-fortunata-rename-design.md:3`

- [ ] **Step 1: Перевести статус спеки в «выполнен»**

В `docs/superpowers/specs/2026-09-16-fortunata-rename-design.md` строку `Статус: утверждён` заменить на:

```markdown
Статус: выполнен
```

- [ ] **Step 2: Закоммитить и запушить в main (правило «только через PR» обходится админским токеном — прецедент: коммиты `docs:` в main)**

```bash
git add docs/superpowers/specs/2026-09-16-fortunata-rename-design.md
git commit -m "docs: спека переименования — выполнен"
git push origin main
```

---

## Самопроверка плана

- Покрытие спеки: модуль/импорты/БД (Task 2), Dockerfile+.gitignore+package.json (Task 3), compose (Task 4), release.yml+README+CLAUDE (Task 5), исторические доки и скриншоты (Task 6), все 5 пунктов «Проверка» (Task 7), релиз через PR (Task 8), статус «выполнен» (Task 9) — полное.
- Версия релиза: CalVer из даты мерж-коммита — `v2026.09.17-<sha>` (дата изменилась на 2026-09-17).
