package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Flight struct {
	DBDSN              string
	GRPCAddr           string
	APIKey             string
	RedisSentinelAddrs []string
	RedisMasterName    string
}

type Booking struct {
	DBDSN                          string
	HTTPAddr                       string
	FlightGRPCAddr                 string
	FlightAPIKey                   string
	CircuitBreakerFailureThreshold int
	CircuitBreakerOpenTimeout      time.Duration
	CircuitBreakerWindow           time.Duration
}

func LoadFlight() (Flight, error) {
	cfg := Flight{
		DBDSN:              env("FLIGHT_DB_DSN", ""),
		GRPCAddr:           env("GRPC_ADDR", ":50051"),
		APIKey:             env("FLIGHT_API_KEY", ""),
		RedisSentinelAddrs: splitAddrs(env("REDIS_SENTINEL_ADDRS", "localhost:26379")),
		RedisMasterName:    env("REDIS_MASTER_NAME", "mymaster"),
	}
	if cfg.DBDSN == "" {
		return Flight{}, fmt.Errorf("FLIGHT_DB_DSN is required")
	}
	if cfg.APIKey == "" {
		return Flight{}, fmt.Errorf("FLIGHT_API_KEY is required")
	}
	if len(cfg.RedisSentinelAddrs) == 0 {
		return Flight{}, fmt.Errorf("REDIS_SENTINEL_ADDRS is required")
	}
	return cfg, nil
}

func LoadBooking() (Booking, error) {
	failureThreshold, err := envInt("CIRCUIT_BREAKER_FAILURE_THRESHOLD", 5)
	if err != nil {
		return Booking{}, err
	}
	openTimeout, err := envDuration("CIRCUIT_BREAKER_OPEN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Booking{}, err
	}
	window, err := envDuration("CIRCUIT_BREAKER_WINDOW", 30*time.Second)
	if err != nil {
		return Booking{}, err
	}

	cfg := Booking{
		DBDSN:                          env("BOOKING_DB_DSN", ""),
		HTTPAddr:                       env("HTTP_ADDR", ":8080"),
		FlightGRPCAddr:                 env("FLIGHT_GRPC_ADDR", "localhost:50051"),
		FlightAPIKey:                   env("FLIGHT_API_KEY", ""),
		CircuitBreakerFailureThreshold: failureThreshold,
		CircuitBreakerOpenTimeout:      openTimeout,
		CircuitBreakerWindow:           window,
	}
	if cfg.DBDSN == "" {
		return Booking{}, fmt.Errorf("BOOKING_DB_DSN is required")
	}
	if cfg.FlightAPIKey == "" {
		return Booking{}, fmt.Errorf("FLIGHT_API_KEY is required")
	}
	if cfg.CircuitBreakerFailureThreshold <= 0 {
		return Booking{}, fmt.Errorf("CIRCUIT_BREAKER_FAILURE_THRESHOLD must be positive")
	}
	if cfg.CircuitBreakerOpenTimeout <= 0 {
		return Booking{}, fmt.Errorf("CIRCUIT_BREAKER_OPEN_TIMEOUT must be positive")
	}
	if cfg.CircuitBreakerWindow <= 0 {
		return Booking{}, fmt.Errorf("CIRCUIT_BREAKER_WINDOW must be positive")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	value := env(key, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := env(key, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func splitAddrs(value string) []string {
	parts := strings.Split(value, ",")
	addrs := make([]string, 0, len(parts))
	for _, part := range parts {
		if addr := strings.TrimSpace(part); addr != "" {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}
