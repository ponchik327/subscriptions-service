# Subscriptions Service

![CI](https://github.com/ponchik327/subscriptions-service/actions/workflows/ci.yml/badge.svg)

REST API сервис для учёта и агрегации онлайн-подписок пользователей.

## Быстрый старт

**Через Docker Compose (всё в контейнерах):**
```bash
cp .env.example .env
docker compose up --build
```

**Локально (только БД в Docker, приложение нативно):**
```bash
cp .env.local.example .env.local   
make dev
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

### Prometheus-метрики
```
http://localhost:8080/metrics
```

## Конфигурация

Конфигурация разделена на два уровня:

- **`config/config.yaml`** — все настройки приложения (таймауты, пул соединений, уровень логов и т.д.). Хранится в репозитории, изменяется через коммит.
- **ENV-переменные** — только секреты и деплой-специфичные значения (DSN с паролем, путь к конфигу, OTLP-эндпоинт). ENV всегда переопределяет YAML.

### Обязательные ENV-переменные

| Переменная | Описание |
|---|---|
| `POSTGRES_DSN` | DSN подключения к PostgreSQL (содержит credentials) |

### Опциональные ENV-переменные

| Переменная | Описание | По умолчанию |
|---|---|---|
| `CONFIG_PATH` | Путь к yaml-конфигу | `./config/config.yaml` |
| `POSTGRES_MIGRATIONS_PATH` | Путь к миграциям | `file:///migrations` (абс., для Docker) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP-эндпоинт для трейсинга; трейсинг отключён если не задан | — |
| `OTEL_SERVICE_NAME` | Имя сервиса в трейсах | `subscriptions-service` |

> Остальные параметры (`HTTP_PORT`, `LOG_LEVEL`, таймауты, `POSTGRES_MAX_CONNS`) настраиваются в `config/config.yaml` и при необходимости переопределяются через соответствующие ENV-переменные (см. теги `env:` в `internal/config/config.go`).

## Observability

При запуске через `make docker-up` поднимаются два дополнительных сервиса:

| Сервис | URL | Назначение |
|---|---|---|
| Prometheus | `http://localhost:9090` | Scrape `/metrics` каждые 15 сек |
| Jaeger | `http://localhost:16686` | UI для просмотра трейсов |

**Включить трейсинг** (Docker): раскомментировать в `.env`:
```
OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4318
```

**Включить трейсинг** (локально):
```bash
docker run -d -p 16686:16686 -p 4318:4318 jaegertracing/all-in-one:latest
# добавить в .env.local:
# OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
```

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
- Docker с Compose v2.1+ (для локального запуска и тестов через testcontainers)

### Онбординг

```bash
make dev-setup
```

Устанавливает golangci-lint, gofumpt, goimports, govulncheck и lefthook, затем активирует pre-commit хуки.

### Полезные команды

| Команда | Описание |
|---|---|
| `make dev` | Поднять БД + собрать и запустить приложение локально |
| `make run` | Собрать и запустить приложение (БД должна быть уже запущена) |
| `make infra-up` | Поднять только PostgreSQL в Docker (без приложения) |
| `make infra-down` | Остановить PostgreSQL |
| `make docker-up` | Поднять полный стек: app + postgres + prometheus + jaeger |
| `make docker-down` | Остановить полный стек |
| `make migrate-up` | Применить миграции вручную |
| `make migrate-down` | Откатить миграции вручную |
| `make lint` | Запустить линтер |
| `make fmt` | Форматирование кода (gofumpt + goimports) |
| `make test` | Юнит-тесты |
| `make test-integration` | Интеграционные тесты (Postgres через testcontainers) |
| `make test-e2e` | E2E тесты (полный стек через testcontainers) |
| `make vuln` | Проверка уязвимостей (govulncheck) |
| `make tidy` | go mod tidy |

Pre-commit хуки (lint + unit tests) и pre-push хуки (race detector + govulncheck) активируются автоматически после `make dev-setup`.
