# CLAUDE.md

## Проект

Fortunata — генератор лотерейных билетов (7 чисел 1–35 + бонус 1–54) и архив
розыгрышей. Go-бэкенд, ванильный JS-фронтенд без сборки, зашитый в бинарник.

## Команды

```bash
ADMIN_PASSWORD=test go run ./cmd/server   # запуск, http://localhost:8080
go test ./...                             # тесты бэкенда
node --test web/parse.test.mjs            # тест парсера (Node ≥ 18)
docker compose -f compose.local.yaml up --build -d   # Docker без Traefik (пароль: test123)
```

## Архитектура

- `cmd/server/` — точка входа: конфиг из env, HTTP-сервер, graceful shutdown
- `internal/api/` — HTTP-хендлеры
- `internal/auth/` — cookie-сессии
- `internal/store/` — SQLite (`modernc.org/sqlite`, чистый Go, без cgo), WAL-режим
- `internal/generate/` — частотно-взвешенная генерация (вес = появления + 1),
  RNG-интерфейс для детерминированных тестов
- `internal/llm/` — клиент llama.cpp (OpenAI-совместимый) для AI-генерации:
  англоязычный промт, парсинг ответа (`<think>`, заборы), валидация и до 2
  доборов невалидных комбинаций
- `internal/timelottery/` — парсер страницы архива timelottery.ru для
  `POST /api/sync` (ищет строки данных по содержимому, устойчив к редизайну)
- `internal/version/` — версия сборки (`dev` по умолчанию), прошивается
  `-ldflags -X` в release.yml и Dockerfile, отдаётся `GET /api/version`
- `web/` — статика, встраивается через `//go:embed` в `web/embed.go`

## Окружение

`ADMIN_PASSWORD` обязателен (без него сервер не стартует). Остальные: `ADDR`
(`:8080`), `DB_PATH` (`fortunata.db`), `SESSION_SECRET` (пусто = случайный),
`COOKIE_SECURE` (`false`; за HTTPS — `true`), `LLM_BASE_URL` + `LLM_MODEL`
(задаются вместе; пусто = AI-генерация выключена), `LLM_API_KEY` (опционален).

## Gotchas

- Правки в `web/` требуют пересборки бинаря — файлы зашиты через embed
- Числа комбинации всегда хранятся по возрастанию; валидация дублируется
  CHECK-ограничениями в SQLite-схеме
- Бэкап SQLite: архивировать весь каталог целиком (свежие записи живут в `-wal`)
- `docker compose down -v` удаляет volume вместе с базой
