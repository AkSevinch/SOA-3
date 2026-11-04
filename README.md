# SOA-3

Flight Booking: REST + gRPC + PostgreSQL + Redis

## Сервисы

| Сервис | Порт | Назначение |
|---|---|---|
| booking-service | 8080 | REST API для клиентов |
| flight-service | 50051 | gRPC API для рейсов |
| flight-db | 5433 | PostgreSQL Flight |
| booking-db | 5434 | PostgreSQL Booking |
| redis-master | внутренний | Мастер Redis |
| redis-replica | внутренний | Реплика Redis |
| redis-sentinel | 26379 | Sentinel для failover |

## Запуск

```bash
docker compose up --build
```

Миграции применяются автоматически при старте сервисов

## REST API

| Метод | Путь |
|---|---|
| GET | `/health` |
| GET | `/flights?origin=SVO&destination=LED&date=2026-12-01` |
| GET | `/flights/{id}` |
| POST | `/bookings` |
| GET | `/bookings/{id}` |
| POST | `/bookings/{id}/cancel` |
| GET | `/bookings?user_id=user-1` |

Пример создания брони

```bash
curl -X POST http://localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "user-1",
    "flight_id": "00000000-0000-0000-0000-000000000001",
    "passenger_name": "Ivan Petrov",
    "passenger_email": "ivan@example.com",
    "seat_count": 2
  }'
```

Пример поиска

```bash
curl 'http://localhost:8080/flights?origin=SVO&destination=LED'
```

## gRPC

Все методы защищены API Key в metadata
Заголовок `x-api-key`, значение из `FLIGHT_API_KEY`

```bash
grpcurl -plaintext -H 'x-api-key: dev-flight-key' localhost:50051 list
grpcurl -plaintext -H 'x-api-key: dev-flight-key' \
  -d '{"origin":"SVO","destination":"LED"}' \
  localhost:50051 flight.v1.FlightService/SearchFlights
```

## Документация

- [gRPC контракт](docs/grpc-contract.md)
- [Аутентификация gRPC](docs/auth.md)
- [Retry вызовов Flight Service](docs/retry.md)
- [Circuit Breaker](docs/circuit-breaker.md)
- [Кеширование в Redis](docs/caching.md)
- [Отказоустойчивый Redis](docs/redis-ha.md)
- [ER-диаграмма](docs/er-diagram.md)
- [Транзакционная целостность](docs/transactions.md)

## Тесты

```bash
make test
```

```bash
docker compose up -d
make test-integration
docker compose down -v
```
