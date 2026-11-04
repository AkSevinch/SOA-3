# Аутентификация gRPC

## Подход

API Key передаётся в gRPC metadata

Заголовок: `x-api-key`

Ключ задаётся переменной окружения `FLIGHT_API_KEY` в обоих сервисах
Flight Service проверяет ключ, Booking Service добавляет его к каждому вызову

REST API Booking Service остаётся публичным
Защищён только внутренний gRPC-канал между сервисами

## Реализация

`internal/auth/auth.go`

- `UnaryServerInterceptor` проверяет unary-вызовы FlightService
- `StreamServerInterceptor` проверяет stream-вызовы, включая reflection
- `UnaryClientInterceptor` добавляет `x-api-key` в исходящий контекст
- сравнение через `crypto/subtle.ConstantTimeCompare`
- ключ не логируется

`cmd/flight/main.go` подключает серверные интерсепторы
`cmd/booking/main.go` подключает клиентский интерсептор

## Ошибки

Нет заголовка или ключ неверный:

```
UNAUTHENTICATED: invalid api key
```

Пустой `FLIGHT_API_KEY` считается ошибкой конфигурации
Сервис не стартует без ключа

## Переменные окружения

| Переменная | Сервис | Назначение |
|---|---|---|
| FLIGHT_API_KEY | flight-service | ожидаемый ключ |
| FLIGHT_API_KEY | booking-service | ключ для исходящих вызовов |

В `docker-compose.yml` значение по умолчанию `dev-flight-key`
Его можно переопределить:

```bash
FLIGHT_API_KEY=my-secret docker compose up --build
```

## Примеры

```bash
grpcurl -plaintext \
  -H 'x-api-key: dev-flight-key' \
  -d '{"origin":"SVO","destination":"LED"}' \
  localhost:50051 flight.v1.FlightService/SearchFlights
```

```bash
grpcurl -plaintext \
  -H 'x-api-key: dev-flight-key' \
  -d '{"id":"00000000-0000-0000-0000-000000000001"}' \
  localhost:50051 flight.v1.FlightService/GetFlight
```

Без заголовка:

```bash
grpcurl -plaintext localhost:50051 list
```

вернёт `UNAUTHENTICATED`, потому что reflection тоже защищён stream-интерсептором

## Проверка

```bash
docker compose up -d --build
curl -X POST http://localhost:8080/bookings -H 'Content-Type: application/json' -d '{...}'
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -H 'x-api-key: dev-flight-key' localhost:50051 list
docker compose down -v
```
