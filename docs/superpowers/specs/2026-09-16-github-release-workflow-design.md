# Дизайн: GitHub Actions — релизы и Docker-образы

Дата: 2026-09-16
Статус: утверждён

## Цель

Настроить GitHub Actions для репозитория `huhen/fortunata`:

1. При слитии PR в `main` — автоматический релиз: статический бинарник
   linux/amd64 + описание изменений из PR.
2. При слитии PR в `main` — публикация Docker-образа в GHCR.
3. Проверка PR (тесты, сборка образа) до мержа.
4. `compose.yaml` переключается на образ из реестра.

## Решённые параметры

| Параметр | Значение |
|---|---|
| Триггер релиза | слитый PR в `main` (`types: [closed]` + `github.event.pull_request.merged == true`) |
| Версия | CalVer: `v$(git show -s --format='%cd' --date=format:'%Y.%m.%d' HEAD)-$(git rev-parse --short HEAD)` (дата из коммита), напр. `v2026.09.16-8d5d1d5` |
| Docker-реестр | GHCR, образ `ghcr.io/huhen/fortunata`; видимость: первый пакет может создаться приватным — проверить и переключить в Public (GitHub → Packages → fortunata → Package settings), иначе `docker compose pull` без логина не сработает |
| Бинарник | только linux/amd64, статический (`CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`) |
| Структура | два воркфлоу: `ci.yml` (проверка) и `release.yml` (публикация) |

Секреты настраивать не нужно: используется встроенный `GITHUB_TOKEN`.

## `ci.yml` — проверка PR

Триггер: `pull_request` с `branches: [main]`, `types: [opened, synchronize, reopened]`.
Права: `contents: read`. Runs-on: `ubuntu-latest`.

Job `test`:
1. `actions/checkout`
2. `actions/setup-go` с `go-version-file: go.mod` (кэш модулей встроен)
3. `go vet ./...`
4. `go test ./...`
5. `actions/setup-node` с `node-version: 22`
6. `node --test web/parse.test.mjs`

Job `docker`:
1. `actions/checkout`
2. `docker/setup-buildx-action`
3. `docker/build-push-action`: `push: false`, `platforms: linux/amd64`, кэш
   `type=gha` (`cache-to: type=gha,mode=max,ignore-error=true`) — контроль, что
   образ собирается, без публикации.

## `release.yml` — публикация

Триггер: `pull_request` с `branches: [main]`, `types: [closed]`; весь job — под
условием `github.event.pull_request.merged == true`.
Права: `contents: write` (тег + релиз), `packages: write` (GHCR).
Concurrence: группа `release-main`, `cancel-in-progress: false` — чтобы два
быстрых мержа не соревновались за тег `latest`; `queue: max` — ожидающие
запуски встают в очередь и не отменяются.

Job `release`, шаги:
1. Версия: `VERSION="v$(git show -s --format='%cd' --date=format:'%Y.%m.%d' HEAD)-$(git rev-parse --short HEAD)"` — дата берётся из коммита, версия детерминирована.
2. `actions/setup-go` + `go test ./...` (повторный прогон — релиз только из
   зелёного кода).
3. Сборка: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath
   -ldflags "-s -w" -o dist/fortunata-linux-amd64 ./cmd/server`.
4. GitHub Release (`softprops/action-gh-release`):
   - `tag_name` = `VERSION`;
   - **заголовок релиза = версия** (`VERSION`);
   - **текст релиза = заголовок PR** заголовком-разделом, ниже — тело PR (если непустое);
   - прикреплён `dist/fortunata-linux-amd64`.
5. `docker/login-action`: registry `ghcr.io`, username `github.actor`,
   password `secrets.GITHUB_TOKEN`.
6. `docker/build-push-action`: `push: true`, `platforms: linux/amd64`,
   теги `ghcr.io/huhen/fortunata:latest` и `ghcr.io/huhen/fortunata:$VERSION`,
   лейбл `org.opencontainers.image.source=https://github.com/huhen/fortunata`,
   кэш `type=gha` (`cache-to` с `mode=max,ignore-error=true`).

## Правки в compose

`compose.yaml` (прод, Traefik):
- убрать `build: .`;
- `image: fortunata:latest` → `image: ghcr.io/huhen/fortunata:latest`;
- добавить `pull_policy: always` — `docker compose up -d` всегда тянет свежий
  `latest`.

`compose.local.yaml` — без изменений (локальная сборка `build: .`).

## Краевые случаи

- **Прямой push в `main` мимо PR** — релиза нет: релиз строго PR-управляемый.
- **Два мержа в один день** — версии различаются коротким хешем, коллизий нет.
- **Повторный запуск** — версия детерминирована коммитом (дата из коммита), поэтому перезапуск в любой день указывает на ту же версию: `action-gh-release` обновляет существующий релиз и перезаливает артефакт (не падает).
- **Три и более быстрых мержа подряд** — `queue: max` держит ожидающие релизные запуски в очереди (отмены нет), `latest` публикуется последовательно.
- **PR из форка** — релиз пропускается (`head.repo.fork == false` в guard'е): `GITHUB_TOKEN` для форков read-only, публикация всё равно невозможна.
- **Пустое тело PR** — текст релиза состоит из заголовка PR.

## Проверка

- Локальная валидация синтаксиса воркфлоу (`actionlint`).
- Сквозной прогон: тестовый PR → срабатывает `ci.yml`; мерж тестового PR →
  срабатывает `release.yml`, создаётся релиз с бинарником и образ в GHCR;
  `docker compose -f compose.yaml pull` успешно тянет образ.

## Вне рамок

- Мульти-арх (arm64) для бинарника и образа.
- Инжект версии в бинарник через ldflags `main.version`.
- Публикация в Docker Hub.
