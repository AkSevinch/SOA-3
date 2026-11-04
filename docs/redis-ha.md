# Отказоустойчивый Redis

## Топология

В `docker-compose.yml` Redis запущен по схеме Sentinel:

| Сервис | Роль | Порт |
|---|---|---|
| `redis-master` | мастер, принимает запись | 172.30.99.10:6379 внутри сети |
| `redis-replica` | реплика, читает данные мастера | 172.30.99.11:6379 внутри сети |
| `redis-sentinel` | следит за мастером и выполняет failover | 26379 |

Узлы Redis имеют статические адреса в сети compose `172.30.99.0/24`.
Это важно после остановки мастера: Sentinel продолжает работать с IP и не зависит
от DNS-имени, которое исчезает вместе с контейнером.
Если подсеть занята, измените её в `docker-compose.yml` и адреса в `sentinel.conf`.

Sentinel знает мастер по имени `mymaster`.
При падении мастера Sentinel повышает реплику и сообщает клиентам новый адрес.

## Конфигурация Sentinel

`deploy/sentinel/sentinel.conf`

```
port 26379
dir /tmp
protected-mode no
sentinel monitor mymaster 172.30.99.10 6379 1
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 10000
sentinel parallel-syncs mymaster 1
```

- `down-after-milliseconds 5000`: мастер считается недоступным через 5 секунд
- `failover-timeout 10000`: таймаут одной попытки failover
- `parallel-syncs 1`: реплики синхронизируются по одной

Sentinel переписывает свой конфиг при изменении топологии.
Шаблон монтируется только для чтения, а рабочий конфиг хранится в томе `sentinel-data`.
Поэтому после перезапуска Sentinel помнит актуального мастера.
Том удаляется командой `docker compose down -v`.

## Подключение приложения

`internal/flight/cache.go`

```go
redis.NewFailoverClient(&redis.FailoverOptions{
	MasterName:    masterName,
	SentinelAddrs: sentinelAddrs,
})
```

Приложение не подключается к мастеру напрямую.
Оно спрашивает Sentinel текущий адрес мастера и автоматически переподключается после failover.

Переменные окружения Flight Service:

```
REDIS_SENTINEL_ADDRS=redis-sentinel:26379
REDIS_MASTER_NAME=mymaster
```

Несколько Sentinel можно передать через запятую:

```
REDIS_SENTINEL_ADDRS=sentinel-1:26379,sentinel-2:26379,sentinel-3:26379
```

## Failover

1. `redis-master` перестаёт отвечать
2. через 5 секунд Sentinel помечает мастер как `sdown`
3. Sentinel выбирает реплику и выполняет `REPLICAOF NO ONE`
4. бывший мастер при возврате становится репликой нового мастера
5. приложение получает новый адрес через Sentinel

Во время переключения возможны кратковременные ошибки Redis.
Кеш работает fail-open: запрос уходит в PostgreSQL, сервис продолжает отвечать.

Данные кеша при failover могут потеряться.
Для кеша это допустимо, источник истины это PostgreSQL.

## Проверка

Текущий мастер:

```bash
docker compose exec redis-sentinel redis-cli -p 26379 sentinel get-master-addr-by-name mymaster
```

Роли узлов:

```bash
docker compose exec redis-master redis-cli ROLE
docker compose exec redis-replica redis-cli ROLE
```

Проверка failover:

```bash
docker compose stop redis-master
sleep 10
docker compose exec redis-sentinel redis-cli -p 26379 sentinel get-master-addr-by-name mymaster
curl http://localhost:8080/flights/00000000-0000-0000-0000-000000000001
docker compose start redis-master
sleep 15
docker compose exec redis-master redis-cli ROLE
```

После остановки мастера Sentinel должен показать адрес `172.30.99.11`.
После запуска `redis-master` возвращается как реплика, но Sentinel переключает
его не сразу: нужно подождать несколько секунд, пока он снова станет доступен.

## Ограничения

Конфигурация рассчитана на учебное задание:

- один Sentinel
- одна реплика
- нет пароля Redis

Для продакшена нужно минимум три Sentinel, несколько реплик и авторизация.
Такая схема защищает от падения одного узла Redis, но не от потери кворума Sentinel.
