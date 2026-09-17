# Fortunata — предсказатель

Генератор билетов для лотереи «Fortunata» (7 чисел из 1–35 + 1 число из 1–54)
и архив прошедших розыгрышей. Часто выпадавшие числа получают повышенный
приоритет при генерации (вес = число появлений + 1).

## Возможности

- **Генератор** — N билетов (по умолчанию 10, до 100), семёрка по возрастанию, бонус отдельно.
- **Архив** — публичный просмотр прошедших розыгрышей.
- **Статистика** — частоты выпадения шаров: основные, затем бонусные, по убыванию; у каждого шара количество выпадений.
- **Редактирование** — добавление/изменение/удаление под паролем на странице `/admin` (без имён пользователей).
- Источник результатов: [timelottery.ru — архив розыгрышей Fortunata](https://timelottery.ru/arhiv/rezultaty-vseh-rozygryshej-fortunata/).

## Локальный запуск

### Вариант 1: локальный Go (нужен Go ≥ 1.25)

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
node --test web/parse.test.mjs   # парсер комбинации (Node ≥ 18)
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

3. Запуск (образ тянется из GHCR; обновление до свежего релиза —
   повторный запуск этой команды, `pull_policy: always`):

   ```bash
   docker compose up -d
   ```

Traefik должен иметь доступ к этой сети; роутер `fortunata` слушает домен
из `DOMAIN` на entrypoint `websecure` с certresolver из `TRAEFIK_CERTRESOLVER`.

## Замечания по деплою

- Образ сервиса тянется из GHCR. Пакет `fortunata` должен оставаться
  публичным (GitHub → Packages → fortunata → Package settings), иначе
  `docker compose pull` без `docker login` упадёт.
- Данные хранятся в named volume `data` (файл `/data/fortunata.db`).
  Бэкап: `docker run --rm -v <project>_data:/data -v $(pwd):/backup alpine \
  tar czf /backup/fortunata-db.tar.gz -C /data .` (имя volume —
  `docker volume ls`; архивируем весь каталог целиком — в WAL-режиме SQLite
  свежие записи живут и в `-wal` файле). Осторожно: `docker compose down -v` удаляет volume
  вместе с базой.
- Предполагается, что глобальный редирект http→https настроен на стороне
  вашего Traefik; compose-файл привязывает роутер только к `websecure`.
- `ADMIN_PASSWORD` и `SESSION_SECRET` передаются через env и видны в
  `docker inspect` — для single-host private-приложения это осознанный
  компромисс; секреты не логируются приложением.
- Логи контейнера ограничены (`json-file`, 10 МБ × 3 файла).

## Переменные окружения

| Переменная | Назначение | По умолчанию |
|---|---|---|
| `ADDR` | Адрес слушателя | `:8080` |
| `DB_PATH` | Путь к файлу SQLite | `fortunata.db` |
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
| GET | `/api/stats` | публично | частоты шаров: `{main: [...], bonus: [...]}` |
| POST | `/api/draws` | сессия | создать `{drawNo, numbers[7], bonus}` |
| PUT | `/api/draws/{no}` | сессия | заменить комбинацию |
| DELETE | `/api/draws/{no}` | сессия | удалить |
| POST | `/api/generate` | публично | `{count}` → `{tickets}` |
