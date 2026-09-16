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
| Документация (`docs/`) | обрабатывается тоже — механическая замена, чтобы `grep -ri generator` не давал разночтений |

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

### Исторические записи в `docs/` — тоже обрабатываются

Чтобы после переименования `grep -ri generator` по репозиторию не давал
разночтений, исторические документы приводятся в соответствие с новым
именем (механическая замена по латинице `generator` → `fortunata`;
кириллическое «генератор» и пакет `internal/generate` не совпадают с
образцом и остаются нетронутыми). Замена выполняется по фиксированному
списку файлов ниже — не рекурсивным обходом `docs/`, чтобы не задеть
планы и спеки, создаваемые в ходе самой работы:

- `docs/superpowers/plans/2026-09-15-lottery-predictor.md` — ~50 вхождений:
  имя модуля, импорты в листингах кода, `go mod init`, Dockerfile, compose,
  имена бинарника/БД/volume/traefik-меток, рабочий путь, имена скриншотов.
- `docs/superpowers/specs/2026-09-15-lottery-predictor-design.md` — 2
  вхождения: корень дерева проекта `generator/`, `/data/generator.db`.
- `docs/superpowers/specs/2026-09-16-github-release-workflow-design.md` — 4
  вхождения: имена артефакта `dist/…-linux-amd64` и старое имя образа.
- Скриншоты: `01-generator.png` → `01-fortunata.png`,
  `03-generator-with-history.png` → `03-fortunata-with-history.png`;
  обновить две ссылки на них в плане. Интерфейс на скриншотах уже
  брендирован «Fortunata» — переснимать не нужно.

Единственное исключение — сама эта спека: упоминания «generator» в ней —
её предмет (описание соответствия старое → новое), поэтому остаются.
После выполнения ей меняется статус на «выполнен».

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
4. `grep -ri generator` по репозиторию — вхождения остаются только в этой
   спеке (`2026-09-16-fortunata-rename-design.md`); подстрока «generator»
   в имени пакета `internal/generate` не встречается.
5. Дымовой прогон: `ADMIN_PASSWORD=test go run ./cmd/server`, создался
   `fortunata.db`.

## Вне рамок

- Переименование пакета `internal/generate` и русское слово «генератор» в прозе.
- Ретроактивные правки старых релизов/тегов/артефактов на GitHub.
