# URL Shortener

Сервис коротких ссылок с асинхронной записью кликов.

## Что умеет

- `POST /links` — создать короткую ссылку (с опциональным TTL)
- `GET /{code}` — редирект на исходный URL + запись клика
- `GET /links/{code}/stats` — статистика по ссылке
- `GET /healthz` — healthcheck

## Стек

- **Go 1.27** — бэкенд
- **Postgres 16** — хранилище
- **Docker Compose** — поднимает БД
- **Каналы + воркер** — клики пишутся батчами в фоне
- **Graceful shutdown** — при `Ctrl+C` клики из очереди дописываются в БД

## Как запустить

1. Скопируй `.env.example` → `.env`, поправь при необходимости:
   ```powershell
   Copy-Item .env.example .env
   ```

2. Подними Postgres:
   ```powershell
   docker compose up -d
   ```

3. Запусти приложение:
   ```powershell
   go run ./cmd/api
   ```

4. Проверь:
   ```powershell
   curl.exe -i http://localhost:8081/healthz
   ```

## Тесты

```powershell
go test ./... -cover
```

Покрытие `internal/handler` — 88%.

## Пример использования

```powershell
# Создать ссылку
$resp = Invoke-RestMethod -Method Post -Uri http://localhost:8081/links `
  -ContentType "application/json" `
  -Body (@{url="https://go.dev/doc/"} | ConvertTo-Json)
$resp | ConvertTo-Json

# Перейти по коду
Invoke-WebRequest "http://localhost:8081/$($resp.code)" -MaximumRedirection 0 -ErrorAction SilentlyContinue

# Статистика
Invoke-RestMethod "http://localhost:8081/links/$($resp.code)/stats" | ConvertTo-Json
```