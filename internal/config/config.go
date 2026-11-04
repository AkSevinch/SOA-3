package config

import (
	"fmt"
	"os"
	"strings"
)

type Flight struct {
	DBDSN              string
	GRPCAddr           string
	APIKey             string
	RedisSentinelAddrs []string
	RedisMasterName    string
}

type Booking struct {
	DBDSN          string
	HTTPAddr       string
	FlightGRPCAddr string
	FlightAPIKey   string
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
	cfg := Booking{
		DBDSN:          env("BOOKING_DB_DSN", ""),
		HTTPAddr:       env("HTTP_ADDR", ":8080"),
		FlightGRPCAddr: env("FLIGHT_GRPC_ADDR", "localhost:50051"),
		FlightAPIKey:   env("FLIGHT_API_KEY", ""),
	}
	if cfg.DBDSN == "" {
		return Booking{}, fmt.Errorf("BOOKING_DB_DSN is required")
	}
	if cfg.FlightAPIKey == "" {
		return Booking{}, fmt.Errorf("FLIGHT_API_KEY is required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
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
