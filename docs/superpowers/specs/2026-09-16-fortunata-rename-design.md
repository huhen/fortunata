# Дизайн: переименование проекта generator → fortunata

Дата: 2026-09-16
Статус: утверждён

## Цель

При создании проекта было выбрано неудачное имя модуля и артефактов —
«generator». Заменить все упоминания на «fortunata» и выпустить релиз.
Брендинг веб-интерфейса уже Fortunata — там менять нечего.

Релиз создаётся существующим пайплайном автоматически: слитый PR в `main` →
`release.yml` публикует релиз `v<CalVer>-<sha>` с бинарником и образом в GHCR.
Отдельных ручных шагов для релиза нет.

## Решённые параметры

| Параметр | Значение |
|---|---|
| Имя Go-модуля | `fortunata` |
| Дефолтный файл БД | `fortunata.db` (было `generator.db`) |
| Миграция данных | не требуется — данных в проде ещё нет |
| Артефакт релиза | `dist/fortunata-linux-amd64` |
| `internal/generate` | не переименовывается — доменное понятие «генерация комбинаций», не имя проекта |
| `docs/superpowers/*` | не трогаются — исторические записи, фиксируют состояние на момент написания |

## Правки по файлам

### Go-модуль и код

- `go.mod`: `module generator` → `module fortunata`.
- Импорты `generator/internal/...` → `fortunata/internal/...` в 6 файлах:
  `cmd/server/main.go`, `internal/api/api.go`, `internal/api/draws.go`,
  `internal/api/auth_handlers.go`, `internal/api/generate.go`,
  `internal/api/api_test.go`.
- Дефолт БД `"generator.db"` → `"fortunata.db"` в `cmd/server/main.go`
  (значение и комментарий) и `cmd/server/main_test.go`.

### Docker и деплой

- `Dockerfile`: бинарник `/out/generator` → `/out/fortunata`;
  `COPY` и `ENTRYPOINT` → `/fortunata`; `ENV DB_PATH=/data/fortunata.db`.
- `compose.yaml`: сервис `generator` → `fortunata`; traefik-метки
  `traefik.http.routers.fortunata.*` и `traefik.http.services.fortunata.*`;
  `DB_PATH: /data/fortunata.db`. Volume `data` уже нейтральный — без изменений.
- `compose.local.yaml`: сервис → `fortunata`; `image: fortunata:local`;
  volume `generator-local-data` → `fortunata-local-data` (ссылка и объявление);
  `DB_PATH: /data/fortunata.db`.

### CI и прочее

- `release.yml`: `dist/generator-linux-amd64` → `dist/fortunata-linux-amd64`
  (шаг сборки и `files:`).
- `package.json`: `generator-web` → `fortunata-web`.
- GHCR-образ не меняется: имя берётся из репозитория
  (`ghcr.io/huhen/fortunata`) и уже корректное.

### Документация

- `README.md`: имя traefik-роутера; `/data/fortunata.db`; пример бэкапа
  `fortunata-db.tar.gz`; дефолт `DB_PATH` в таблице переменных.
- `CLAUDE.md`: дефолт `DB_PATH`.

## Краевые случаи

- **Старые локальные данные** — volume `fortunata-local-data` новый, прежний
  `generator-local-data` останется висеть неиспользуемым. Данных нет, вопрос
  снят; при желании удалить: `docker volume rm generator-local-data`.
- **Прод-деплой после обновления** — контейнер стартует с пустой БД
  `/data/fortunata.db` в существующем volume `data`. Данных нет, миграция не
  требуется.
- **Русское слово «генератор» в прозе** — описание проекта («генератор
  лотерейных билетов») — доменное понятие, не переименовывается.

## Проверка

1. `go build ./... && go test ./...`
2. `node --test web/parse.test.mjs`
3. `docker compose -f compose.local.yaml build` — образ собирается с новым
   именем бинарника.
4. `grep -ri generator` по репозиторию (исключая `docs/`) — 0 вхождений;
   подстрока «generator» в имени пакета `internal/generate` не встречается.
5. Дымовой прогон: `ADMIN_PASSWORD=test go run ./cmd/server`, создался
   `fortunata.db`.

## Вне рамок

- Переименование пакета `internal/generate` и русское слово «генератор» в прозе.
- Правки исторических записей в `docs/superpowers/*`.
- Ретроактивные правки старых релизов/тегов/артефактов на GitHub.
