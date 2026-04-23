# Subscriptions Service

![CI](https://github.com/ponchik327/subscriptions-service/actions/workflows/ci.yml/badge.svg)

REST API сервис для учёта и агрегации онлайн-подписок пользователей.

## Быстрый старт

```bash
cp .env.example .env
docker compose up --build
```

Сервис поднимается на `http://localhost:8080`. Миграции применяются автоматически при старте.

## Эндпоинты

### Создать подписку
```bash
curl -X POST http://localhost:8080/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{
    "service_name": "Yandex Plus",
    "price": 400,
    "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
    "start_date": "07-2025"
  }'
```

### Получить подписку
```bash
curl http://localhost:8080/subscriptions/{id}
```

### Обновить подписку
```bash
curl -X PUT http://localhost:8080/subscriptions/{id} \
  -H 'Content-Type: application/json' \
  -d '{
    "service_name": "Yandex Plus",
    "price": 500,
    "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
    "start_date": "07-2025",
    "end_date": "12-2025"
  }'
```

### Удалить подписку
```bash
curl -X DELETE http://localhost:8080/subscriptions/{id}
```

### Список подписок
```bash
# Все (с пагинацией)
curl "http://localhost:8080/subscriptions?limit=20&offset=0"

# По пользователю
curl "http://localhost:8080/subscriptions?user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba"

# По сервису
curl "http://localhost:8080/subscriptions?service_name=Yandex+Plus"
```

### Агрегация (стоимость за период)
```bash
# Сумма всех подписок за 2025 год
curl "http://localhost:8080/subscriptions/summary?from=01-2025&to=12-2025"

# Сумма конкретного пользователя
curl "http://localhost:8080/subscriptions/summary?from=01-2025&to=12-2025&user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba"

# По сервису
curl "http://localhost:8080/subscriptions/summary?from=01-2025&to=12-2025&service_name=Netflix"
```

Ответ:
```json
{"total": 4800, "currency": "RUB", "from": "01-2025", "to": "12-2025"}
```

### Health check
```bash
curl http://localhost:8080/healthz
```

### Swagger UI
```
http://localhost:8080/swagger/index.html
```

## Переменные окружения

| Переменная | Описание | По умолчанию |
|---|---|---|
| `POSTGRES_DSN` | DSN подключения к PostgreSQL | **обязательна** |
| `HTTP_HOST` | Адрес для прослушивания | `0.0.0.0` |
| `HTTP_PORT` | Порт | `8080` |
| `HTTP_READ_TIMEOUT` | Таймаут чтения | `10s` |
| `HTTP_WRITE_TIMEOUT` | Таймаут записи | `10s` |
| `HTTP_SHUTDOWN_TIMEOUT` | Таймаут graceful shutdown | `5s` |
| `POSTGRES_MAX_CONNS` | Макс. соединений в пуле | `10` |
| `POSTGRES_MIGRATIONS_PATH` | Путь к миграциям | `file://migrations` |
| `LOG_LEVEL` | Уровень логирования (`debug`/`info`/`warn`/`error`) | `info` |
| `CONFIG_PATH` | Путь к yaml-конфигу | `./config/config.yaml` |

## Тестирование

```bash
# Юнит-тесты (быстрые, без Docker)
make test

# Интеграционные тесты (Postgres через testcontainers)
make test-integration

# E2E-тесты (полный стек через testcontainers)
make test-e2e

# Все сразу
make test-all
```

**Что тестируется на каждом уровне:**

- **Юниты** — тип `MonthYear` (парсинг/сериализация), хендлеры с мок-сервисом (валидация DTO, HTTP-коды), сервис (маппинг ошибок, валидация периода).
- **Интеграционные** — репозиторий на реальном Postgres: CRUD и полная матрица из 14 кейсов агрегации `/summary`.
- **E2E** — 6 сценариев через живой HTTP-стек: CRUD happy path, list+filter+pagination, summary, валидация, 404, healthz.

**Почему нет юнит-тестов для CRUD-методов сервиса:** `service.Create` — это `repo.Create` без логики. Тест с моком репозитория проверял бы только то, что метод вызывается, а не то, что он работает правильно. Такое покрытие даёт ложную уверенность. Эти пути покрываются handler-тестами (через мок сервиса) и E2E (через живой стек).

## Генерация

```bash
# Пересобрать моки (после изменения интерфейсов)
make mocks

# Пересобрать Swagger-документацию
make swag
```

## Development

### Требования

- Go 1.26.2+
- make
- Docker (для интеграционных и E2E тестов через testcontainers)

### Онбординг

```bash
make dev-setup
```

Устанавливает golangci-lint, gofumpt, goimports, govulncheck и lefthook, затем активирует pre-commit хуки.

### Полезные команды

| Команда | Описание |
|---|---|
| `make lint` | Запустить линтер |
| `make fmt` | Форматирование кода (gofumpt + goimports) |
| `make test` | Юнит-тесты |
| `make test-integration` | Интеграционные тесты (Postgres через testcontainers) |
| `make test-e2e` | E2E тесты (полный стек через testcontainers) |
| `make vuln` | Проверка уязвимостей (govulncheck) |
| `make tidy` | go mod tidy |

Pre-commit хуки (lint + unit tests) и pre-push хуки (race detector + govulncheck) активируются автоматически после `make dev-setup`.
