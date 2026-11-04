# Транзакционная целостность

## Резервирование мест

`internal/flight/repo.go:64`

- `BEGIN`
- `SELECT available_seats, status FROM flights WHERE id = $1 FOR UPDATE`
- проверка статуса `SCHEDULED`
- проверка `available_seats >= seat_count`
- проверка отсутствия резервации с тем же `booking_id`
- `UPDATE flights SET available_seats = available_seats - $1`
- `INSERT INTO seat_reservations`
- `COMMIT`

`defer tx.Rollback(ctx)` откатывает изменения при любой ошибке
`FOR UPDATE` блокирует строку рейса до конца транзакции
Второй параллельный запрос ждёт и видит уже обновлённое число мест

## Отмена резервирования

`internal/flight/repo.go:120`

- `BEGIN`
- `SELECT ... WHERE booking_id = $1 AND status = 'ACTIVE' FOR UPDATE`
- `UPDATE seat_reservations SET status = 'RELEASED'`
- `UPDATE flights SET available_seats = available_seats + $1`
- `COMMIT`

Возврат мест и смена статуса применяются вместе или не применяются вовсе

## Создание брони

`internal/booking/service.go:40`

1. `GetFlight`
2. `ReserveSeats`
3. `repo.Create` с `total_price_minor = seat_count * price_minor`

Если `ReserveSeats` вернул ошибку, `repo.Create` не вызывается
Если `repo.Create` упал, вызывается `ReleaseReservation` как компенсация
Компенсация использует `context.WithoutCancel` и таймаут 5 секунд
Это сохраняет возврат мест даже при отмене исходного HTTP-запроса

## Отмена брони

`internal/booking/service.go:102`

1. получить бронь
2. проверить статус `CONFIRMED`
3. вызвать `ReleaseReservation`
4. поставить `CANCELLED` в `booking_db`

Это два шага в разных сервисах
Если шаг 4 упадёт, места уже вернутся, а бронь останется `CONFIRMED`
Пункт 5 требует атомарность только для создания брони
Для отмены это сознательный best-effort в рамках задания

## Повторы

`ReserveSeats` идемпотентен по `booking_id`
Повтор с теми же `flight_id` и `seat_count` возвращает существующую резервацию
Повтор с другими параметрами даёт `ALREADY_EXISTS`
`ReleaseReservation` для уже `RELEASED` резервации возвращает успех
Поэтому retry не создаёт дубликатов и не возвращает места дважды

## Тесты

```bash
make test
```

```bash
docker compose up -d
make test-integration
docker compose down -v
```

Юнит-тесты проверяют:
- отсутствие `repo.Create` при ошибке `ReserveSeats`
- расчёт `total_price_minor`
- вызов компенсации
- работу компенсации при отменённом контексте

Интеграционные тесты проверяют:
- два параллельных резерва на последнее место
- ровно одну активную резервацию
- идемпотентность резерва и отмены
- возврат мест при отмене
- защиту от повторного `booking_id`
