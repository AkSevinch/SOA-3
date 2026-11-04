package config

import (
	"testing"
	"time"
)

func setBookingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BOOKING_DB_DSN", "postgres://booking:booking@localhost:5434/booking_db")
	t.Setenv("FLIGHT_API_KEY", "key")
	t.Setenv("CIRCUIT_BREAKER_FAILURE_THRESHOLD", "")
	t.Setenv("CIRCUIT_BREAKER_OPEN_TIMEOUT", "")
	t.Setenv("CIRCUIT_BREAKER_WINDOW", "")
}

func TestLoadBookingCircuitBreakerDefaults(t *testing.T) {
	setBookingEnv(t)

	cfg, err := LoadBooking()
	if err != nil {
		t.Fatalf("load booking: %v", err)
	}
	if cfg.CircuitBreakerFailureThreshold != 5 {
		t.Fatalf("threshold = %d, want 5", cfg.CircuitBreakerFailureThreshold)
	}
	if cfg.CircuitBreakerOpenTimeout != 15*time.Second {
		t.Fatalf("timeout = %s, want 15s", cfg.CircuitBreakerOpenTimeout)
	}
	if cfg.CircuitBreakerWindow != 30*time.Second {
		t.Fatalf("window = %s, want 30s", cfg.CircuitBreakerWindow)
	}
}

func TestLoadBookingCircuitBreakerOverrides(t *testing.T) {
	setBookingEnv(t)
	t.Setenv("CIRCUIT_BREAKER_FAILURE_THRESHOLD", "7")
	t.Setenv("CIRCUIT_BREAKER_OPEN_TIMEOUT", "5s")
	t.Setenv("CIRCUIT_BREAKER_WINDOW", "1m")

	cfg, err := LoadBooking()
	if err != nil {
		t.Fatalf("load booking: %v", err)
	}
	if cfg.CircuitBreakerFailureThreshold != 7 {
		t.Fatalf("threshold = %d, want 7", cfg.CircuitBreakerFailureThreshold)
	}
	if cfg.CircuitBreakerOpenTimeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", cfg.CircuitBreakerOpenTimeout)
	}
	if cfg.CircuitBreakerWindow != time.Minute {
		t.Fatalf("window = %s, want 1m", cfg.CircuitBreakerWindow)
	}
}

func TestLoadBookingCircuitBreakerInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "threshold not a number", key: "CIRCUIT_BREAKER_FAILURE_THRESHOLD", value: "abc"},
		{name: "threshold zero", key: "CIRCUIT_BREAKER_FAILURE_THRESHOLD", value: "0"},
		{name: "timeout invalid", key: "CIRCUIT_BREAKER_OPEN_TIMEOUT", value: "soon"},
		{name: "timeout zero", key: "CIRCUIT_BREAKER_OPEN_TIMEOUT", value: "0s"},
		{name: "window invalid", key: "CIRCUIT_BREAKER_WINDOW", value: "bad"},
		{name: "window negative", key: "CIRCUIT_BREAKER_WINDOW", value: "-1s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBookingEnv(t)
			t.Setenv(test.key, test.value)

			if _, err := LoadBooking(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
