# Кеширование в Redis

## Что кешируется

Flight Service кеширует чтение, чтобы не ходить в PostgreSQL на каждый запрос.

| Данные | Ключ | TTL |
|---|---|---|
| Рейс | `flight:{id}` | 5 минут |
| Поиск | `search:{origin}:{destination}:{date}` | 5 минут |

Если дата не передана, в ключе используется `any`.

Значения хранятся в JSON.
Все ключи создаются с TTL, бесконечного кеша нет.

## Cache-Aside

`internal/flight/service.go`

Чтение:

1. проверить Redis
2. при попадании вернуть данные
3. при промахе прочитать PostgreSQL
4. записать результат в Redis с TTL

Логи:

```
cache hit flight 00000000-0000-0000-0000-000000000001
cache miss flight 00000000-0000-0000-0000-000000000001
cache set flight ... ttl 5m0s
```

## Инвалидация

После успешных мутаций кеш удаляется:

- `ReserveSeats`: `flight:{id}` и все `search:*`
- `ReleaseReservation`: `flight:{id}` и все `search:*`

Поисковые ключи удаляются через `SCAN` и `DEL`, без блокирующего `KEYS`.

Инвалидация выполняется в отдельном контексте `context.WithoutCancel` с таймаутом 5 секунд.
Поэтому отключение клиента после успешной транзакции не оставляет устаревший кеш.

В контракте нет метода `UpdateFlight`, поэтому других мутаций для инвалидации нет.

## Если Redis недоступен

Кеш работает по принципу fail-open:

- ошибка чтения из кеша: запрос идёт в PostgreSQL
- ошибка записи в кеш: ответ всё равно возвращается
- ошибка инвалидации: пишется в лог, устаревшие данные живут не дольше TTL

Сервис не падает без Redis.
При старте ошибка `PING` только логируется.

## Настройка

Переменные окружения:

```
REDIS_SENTINEL_ADDRS=redis-sentinel:26379
REDIS_MASTER_NAME=mymaster
```

`REDIS_SENTINEL_ADDRS` принимает несколько адресов через запятую.
По умолчанию `localhost:26379` и `mymaster`.

В `docker-compose.yml` Redis поднимается в отказоустойчивой конфигурации:
мастер, реплика и Sentinel, подробнее в `docs/redis-ha.md`.
Flight Service подключается к Sentinel и стартует после того, как Sentinel
увидит мастера.

## Проверка

```bash
docker compose up -d --build
curl http://localhost:8080/flights/00000000-0000-0000-0000-000000000001
curl http://localhost:8080/flights/00000000-0000-0000-0000-000000000001
```

В логах Flight Service первый запрос даст `cache miss`, второй `cache hit`.

TTL:

```bash
docker compose exec redis-master redis-cli TTL flight:00000000-0000-0000-0000-000000000001
```

Если мастер уже переключился на реплику, используйте актуальный адрес из Sentinel:

```bash
docker compose exec redis-sentinel redis-cli -p 26379 sentinel get-master-addr-by-name mymaster
```

Ключ после бронирования:

```bash
docker compose exec redis-master redis-cli EXISTS flight:00000000-0000-0000-0000-000000000001
```

После `POST /bookings` команда должна вернуть `0`.

## Тесты

```bash
make test
```

```bash
docker compose up -d
make test-integration
docker compose down -v
```

Юнит-тесты проверяют hit, miss, fallback при ошибке Redis и инвалидацию.
Интеграционные тесты с `TEST_REDIS_SENTINEL_ADDRS` и `TEST_REDIS_MASTER_NAME`
проверяют реальные ключи, TTL и удаление через Sentinel.
