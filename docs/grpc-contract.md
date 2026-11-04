# gRPC контракт Flight Service

Контракт: `proto/flight/v1/flight.proto`
Пакет: `flight.v1`
Go пакет: `soa3/gen/go/flight/v1`

## Генерация

```bash
make proto-tools
make proto
```

Стабы пишутся в `gen/go/flight/v1`

## Методы

| RPC | Запрос | Ответ | Назначение |
|---|---|---|---|
| SearchFlights | origin, destination, optional date | список рейсов | поиск рейсов SCHEDULED |
| GetFlight | id | рейс | получение рейса |
| ReserveSeats | flight_id, seat_count, booking_id | reservation_id, available_seats, status | атомарное резервирование мест |
| ReleaseReservation | booking_id | reservation_id, released_seats, status | возврат мест |

## Ошибки

| RPC | Код | Условие |
|---|---|---|
| SearchFlights | INVALID_ARGUMENT | пустые origin или destination |
| GetFlight | NOT_FOUND | рейс не найден |
| GetFlight | INVALID_ARGUMENT | пустой id |
| ReserveSeats | INVALID_ARGUMENT | seat_count <= 0 |
| ReserveSeats | NOT_FOUND | рейс не найден |
| ReserveSeats | RESOURCE_EXHAUSTED | недостаточно мест |
| ReserveSeats | FAILED_PRECONDITION | рейс не SCHEDULED |
| ReserveSeats | ALREADY_EXISTS | повторный booking_id |
| ReleaseReservation | NOT_FOUND | активная резервация не найдена |
| любой | UNAUTHENTICATED | нет или неверные credentials, для п.6 |

## Идемпотентность

ReserveSeats принимает booking_id
Повторный вызов с тем же booking_id, flight_id и seat_count возвращает существующую ACTIVE резервацию
Другие параметры дают ALREADY_EXISTS
ReleaseReservation идемпотентен: для RELEASED резервации возвращается успех без повторного возврата мест

## Аутентификация

Все методы защищены API Key в metadata
Заголовок: `x-api-key`
Переменная окружения: `FLIGHT_API_KEY`
Подробнее: [auth.md](auth.md)

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
  -d '{"flight_id":"f1","seat_count":2,"booking_id":"b1"}' \
  localhost:50051 flight.v1.FlightService/ReserveSeats
```

```bash
grpcurl -plaintext \
  -H 'x-api-key: dev-flight-key' \
  -d '{"booking_id":"b1"}' \
  localhost:50051 flight.v1.FlightService/ReleaseReservation
```
