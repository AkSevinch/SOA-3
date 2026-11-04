# Retry вызовов Flight Service

## Политика

Booking Service повторяет gRPC-вызовы при временных ошибках.

| Параметр | Значение |
|---|---|
| Максимум попыток | 3 |
| Базовая задержка | 100мс |
| Задержки | 100мс, 200мс |
| Повторяемые коды | UNAVAILABLE, DEADLINE_EXCEEDED |
| Без повтора | INVALID_ARGUMENT, NOT_FOUND, RESOURCE_EXHAUSTED, остальные |

3 попытки означают первый вызов и два повтора.
Формула задержки: `base * 2^(attempt-1)`.
Пауза прерывается, если контекст отменён или истёк.

## Реализация

`internal/booking/retry.go`

- `RetryConfig{MaxAttempts, BaseDelay}`
- `DefaultRetryConfig()` возвращает 3 попытки и 100мс
- `UnaryRetryInterceptor` оборачивает `invoker`
- ошибка проверяется через `status.Code`
- после последней попытки возвращается последняя ошибка

`cmd/booking/main.go` подключает интерсепторы цепочкой:

```go
grpc.WithChainUnaryInterceptor(
    auth.UnaryClientInterceptor(cfg.FlightAPIKey),
    booking.UnaryRetryInterceptor(booking.DefaultRetryConfig()),
)
```

Порядок важен: `auth` внешний и добавляет ключ один раз, `retry` внутренний и повторяет вызов с тем же контекстом.

## Идемпотентность ReserveSeats

Повтор после потерянного ответа не должен создать вторую резервацию.

`internal/flight/repo.go`

- резервация ищется по `booking_id` в той же транзакции
- если она `ACTIVE` и совпадают `flight_id` и `seat_count`, возвращается как успех
- если параметры другие, возвращается `ALREADY_EXISTS`
- уникальный индекс `booking_id` остаётся второй линией защиты

## Идемпотентность ReleaseReservation

Повторная отмена также безопасна.

- если резервация уже `RELEASED`, возвращается успех
- места повторно не увеличиваются
- если резервации нет, возвращается `NOT_FOUND`

## Логи

```
retry /flight.v1.FlightService/ReserveSeats attempt 1/3 after 100ms: Unavailable
retry /flight.v1.FlightService/ReserveSeats attempt 2/3 after 200ms: Unavailable
```

## Тесты

```bash
make test
```

```bash
docker compose up -d
make test-integration
docker compose down -v
```

Юнит-тесты проверяют успех, повторы, лимит попыток, запрещённые коды и отмену контекста.
Интеграционные тесты проверяют идемпотентность резерва и отмены.
