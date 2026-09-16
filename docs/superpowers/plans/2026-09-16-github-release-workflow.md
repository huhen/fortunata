# GitHub Actions: релизы по MR и Docker-образы — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Настроить GitHub Actions: слитый PR в `main` → релиз с бинарником linux/amd64 + образ в GHCR; PR до мержа проверяются тестами и пробной сборкой образа; `compose.yaml` переключается на образ из реестра.

**Architecture:** Два воркфлоу: `ci.yml` (pull_request opened/synchronize/reopened → тесты + сборка образа без публикации) и `release.yml` (pull_request closed + `merged == true` → версия CalVer, бинарник, GitHub Release из PR, образ в GHCR с тегами `latest` и версией). Спецификация: `docs/superpowers/specs/2026-09-16-github-release-workflow-design.md`.

**Tech Stack:** GitHub Actions, GHCR, Docker Compose v2, Go toolchain, actionlint v1.7.9 (установлен локально, `/snap/bin/actionlint`).

**Версии действий** (проверены через GitHub API, 2026-09-16): `actions/checkout@v7`, `actions/setup-go@v7`, `actions/setup-node@v7`, `docker/setup-buildx-action@v4`, `docker/login-action@v4`, `docker/build-push-action@v7`, `softprops/action-gh-release@v3`.

**Окружение исполнителя:** `gh` авторизован как `huhen`; git-протокол ssh; локальные go 1.26, node 22, docker compose v2.40. Все команды — из корня репозитория `/home/usr1/coding/github/fortunata`.

**Важно:** работа идёт в ветке `ci/workflows`; после мержа её PR сам становится первым релизом — этим E2E-проверяется весь пайплайн. Тег `latest` и релиз публикуются реально.

---

### Task 1: Воркфлоу проверки PR — `.github/workflows/ci.yml`

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Создать ветку**

```bash
git checkout main && git checkout -b ci/workflows
```

Expected: `Switched to a new branch 'ci/workflows'`

- [ ] **Step 2: Создать файл `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  pull_request:
    branches: [main]
    types: [opened, synchronize, reopened]

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod

      - name: go vet
        run: go vet ./...

      - name: go test
        run: go test ./...

      - uses: actions/setup-node@v7
        with:
          node-version: 22

      - name: parse test
        run: node --test web/parse.test.mjs

  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: docker/setup-buildx-action@v4

      - name: Build image (no push)
        uses: docker/build-push-action@v7
        with:
          context: .
          platforms: linux/amd64
          push: false
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 3: Проверить actionlint**

```bash
actionlint .github/workflows/ci.yml
```

Expected: пустой вывод, exit-код 0.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: воркфлоу проверки PR (тесты и сборка образа)"
```

---

### Task 2: Воркфлоу релиза — `.github/workflows/release.yml`

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Создать файл `.github/workflows/release.yml`**

```yaml
name: Release

on:
  pull_request:
    branches: [main]
    types: [closed]

permissions:
  contents: write
  packages: write

concurrency:
  group: release-main
  cancel-in-progress: false

jobs:
  release:
    if: github.event.pull_request.merged == true && github.event.pull_request.head.repo.fork == false
    runs-on: ubuntu-latest
    env:
      IMAGE: ghcr.io/${{ github.repository }}
    steps:
      - uses: actions/checkout@v7
        with:
          ref: ${{ github.event.pull_request.merge_commit_sha }}

      - name: Версия (CalVer + хеш)
        run: echo "VERSION=v$(date -u +%Y.%m.%d)-$(git rev-parse --short HEAD)" >> "$GITHUB_ENV"

      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod

      - name: go test
        run: go test ./...

      - name: Сборка бинарника linux/amd64
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -trimpath -ldflags "-s -w" \
            -o dist/generator-linux-amd64 ./cmd/server

      - name: Описание релиза (тело PR, если пусто — заголовок)
        env:
          PR_BODY: ${{ github.event.pull_request.body }}
          PR_TITLE: ${{ github.event.pull_request.title }}
        run: |
          BODY="$PR_BODY"
          if [ -z "$BODY" ]; then BODY="$PR_TITLE"; fi
          printf '%s\n' "$BODY" > dist/release-notes.md

      - name: GitHub Release
        uses: softprops/action-gh-release@v3
        with:
          tag_name: ${{ env.VERSION }}
          name: ${{ github.event.pull_request.title }}
          body_path: dist/release-notes.md
          files: dist/generator-linux-amd64
          fail_on_unmatched_files: true

      - uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/setup-buildx-action@v4

      - name: Публикация образа в GHCR
        uses: docker/build-push-action@v7
        with:
          context: .
          platforms: linux/amd64
          push: true
          tags: |
            ${{ env.IMAGE }}:latest
            ${{ env.IMAGE }}:${{ env.VERSION }}
          labels: org.opencontainers.image.source=https://github.com/${{ github.repository }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 2: Проверить actionlint**

```bash
actionlint .github/workflows/release.yml
```

Expected: пустой вывод, exit-код 0.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: воркфлоу релиза по слитому PR (бинарник + образ в GHCR)"
```

---

### Task 3: Переключение `compose.yaml` на образ из реестра

**Files:**
- Modify: `compose.yaml:1-5`

- [ ] **Step 1: Заменить начало блока `services`**

Было:

```yaml
services:
  generator:
    build: .
    image: generator:latest
    restart: unless-stopped
```

Стало:

```yaml
services:
  generator:
    image: ghcr.io/huhen/fortunata:latest
    pull_policy: always
    restart: unless-stopped
```

Остальная часть файла (environment, volumes, networks, labels Traefik) не меняется.

- [ ] **Step 2: Валидировать `compose.yaml`**

```bash
DOMAIN=example.com TRAEFIK_CERTRESOLVER=letsencrypt ADMIN_PASSWORD=test SESSION_SECRET=test \
  docker compose -f compose.yaml config | grep -E 'image:|pull_policy:|build:'
```

Expected вывод — ровно две строки, строки с `build:` быть не должно:

```
    image: ghcr.io/huhen/fortunata:latest
    pull_policy: always
```

- [ ] **Step 3: Убедиться, что `compose.local.yaml` не задет**

```bash
docker compose -f compose.local.yaml config | grep -E 'image:|build:'
```

Expected: `image: generator:local` и `build:` присутствуют.

- [ ] **Step 4: Commit**

```bash
git add compose.yaml
git commit -m "build: compose.yaml переключён на образ ghcr.io/huhen/fortunata"
```

---

### Task 4: Пуш ветки и открытие PR

**Files:** нет новых — доставка изменений в GitHub.

- [ ] **Step 1: Пуш основного состояния и ветки**

```bash
git push origin main
git push -u origin ci/workflows
```

Expected: обе ветки запушены (в `main` — коммит со спекой `f3798ef`, в `ci/workflows` — три новых).

- [ ] **Step 2: Создать PR**

Заголовок PR станет заголовком релиза, тело — описанием релиза, поэтому текст содержательный:

```bash
gh pr create --base main --head ci/workflows \
  --title "CI: релизы по слитому PR и Docker-образы в GHCR" \
  --body-file - <<'EOF'
## Что сделано
- `ci.yml` — проверка каждого PR в `main`: go vet, go test, node --test, сборка Docker-образа без публикации
- `release.yml` — при слитии PR: релиз со статическим бинарником linux/amd64, версия `v<дата>-<хеш>`, описание из PR; публикация образа `ghcr.io/huhen/fortunata` с тегами `latest` и версионным
- `compose.yaml` — переход с локальной сборки на образ из GHCR (`pull_policy: always`)
EOF
```

Expected: ссылка на созданный PR (запомнить номер как `$PR`).

- [ ] **Step 3: Дождаться зелёного CI**

```bash
gh pr checks "$PR" --watch
```

Expected: `CI / test` и `CI / docker` — pass. Если red — чинить до мержа, не мержить.

---

### Task 5: Мерж PR и проверка релиза end-to-end

**Files:** нет новых — проверка публикуемого пайплайна.

- [ ] **Step 1: Смержить PR (squash)**

```bash
gh pr merge "$PR" --squash --delete-branch
```

Expected: PR смержен, ветка `ci/workflows` удалена. Событие closed+merged запускает `release.yml`.

- [ ] **Step 2: Дождаться завершения Release**

```bash
gh run list --workflow=release.yml --limit 1
gh run watch "$(gh run list --workflow=release.yml --limit 1 --json databaseId -q '.[0].databaseId')"
```

Expected: `completed`, `success`. Версия тега совпадает с шаблоном `v2026.MM.DD-<7 знаков хеша>`.

- [ ] **Step 3: Проверить релиз и артефакт**

```bash
gh release view --json tagName,name,body,assets
```

Expected: `name` = заголовок PR; `body` = тело PR; в `assets` — `generator-linux-amd64`.

- [ ] **Step 4: Проверить образ в GHCR**

```bash
docker manifest inspect ghcr.io/huhen/fortunata:latest > /dev/null && echo "latest: OK"
docker manifest inspect "ghcr.io/huhen/fortunata:$(gh release view --json tagName -q .tagName)" > /dev/null && echo "version tag: OK"
```

Expected: `latest: OK` и `version tag: OK`. Если `denied` — образ приватный: открыть на GitHub → Packages → `fortunata` → Package settings → Change visibility → Public (публикация через `GITHUB_TOKEN` обычно наследует публичность репозитория, но первый пакет может создаться приватным), затем повторить проверку.

- [ ] **Step 5: Проверить прод-компоуз с реестром**

```bash
DOMAIN=example.com TRAEFIK_CERTRESOLVER=letsencrypt ADMIN_PASSWORD=test SESSION_SECRET=test \
  docker compose -f compose.yaml pull
```

Expected: `Pulling generator... Image ghcr.io/huhen/fortunata:latest Pulled` (переменные здесь фиктивные, сервис не запускается — только проверяется, что путь до образа рабочий).

- [ ] **Step 6: Финальная ревизия**

```bash
git checkout main && git pull
ls .github/workflows/
```

Expected: ветка `main` содержит оба воркфлоу и новый `compose.yaml`; `git status` чист.

---

## Самопроверка плана

- **Покрытие спеки:** ci.yml (test+docker jobs) — Task 1; release.yml (версия, тесты, бинарник, Release из PR, GHCR, concurrency, права) — Task 2; compose.yaml — Task 3; actionlint — Tasks 1–2; сквозная проверка — Tasks 4–5. Краевые случаи из спеки покрыты конструкцией триггеров (`types: [closed]` + `merged == true`, хеш в версии, body_path с фолбэком).
- **Плейсхолдеров нет:** все шаги содержат полный код/команды и ожидаемый результат.
- **Консистентность:** `ghcr.io/huhen/fortunata`, `dist/generator-linux-amd64`, `VERSION`/`IMAGE` — одинаковы во всех задачах.
