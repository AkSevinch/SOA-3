GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: proto-tools proto lint build test test-integration

# Установка инструментов
proto-tools:
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Генерация стабов
proto:
	buf generate

# Проверка контракта
lint:
	buf lint

# Сборка
build:
	go build ./...

# Unit тесты
test:
	go test ./...

# Интеграционные тесты, нужны запущенные flight-db и redis sentinel
test-integration:
	TEST_FLIGHT_DB_DSN=postgres://flight:flight@localhost:5433/flight_db?sslmode=disable \
	TEST_REDIS_SENTINEL_ADDRS=localhost:26379 \
	TEST_REDIS_MASTER_NAME=mymaster \
		go test ./internal/flight -count=1
