package flight

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testCache(t *testing.T) *Cache {
	t.Helper()

	addrs := os.Getenv("TEST_REDIS_SENTINEL_ADDRS")
	if addrs == "" {
		t.Skip("TEST_REDIS_SENTINEL_ADDRS is not set")
	}
	masterName := os.Getenv("TEST_REDIS_MASTER_NAME")
	if masterName == "" {
		masterName = "mymaster"
	}

	cache := NewCache(strings.Split(addrs, ","), masterName)
	t.Cleanup(func() {
		_ = cache.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cache.Ping(ctx); err != nil {
		t.Fatalf("ping redis: %v", err)
	}
	return cache
}

func TestCacheFlight(t *testing.T) {
	cache := testCache(t)
	ctx := context.Background()
	id := uuid.NewString()
	flight := Flight{
		ID:            id,
		FlightNumber:  "SU1234",
		Origin:        "SVO",
		Destination:   "LED",
		DepartureTime: time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC),
		PriceMinor:    750000,
		Currency:      "RUB",
		Status:        "SCHEDULED",
	}
	t.Cleanup(func() {
		_ = cache.InvalidateFlight(context.Background(), id)
	})

	if err := cache.SetFlight(ctx, flight); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, found, err := cache.GetFlight(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found {
		t.Fatal("flight not found in cache")
	}
	if got.FlightNumber != flight.FlightNumber || got.PriceMinor != flight.PriceMinor {
		t.Fatalf("flight = %+v", got)
	}

	ttl, err := cache.client.TTL(ctx, flightKey(id)).Result()
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > cacheTTL {
		t.Fatalf("ttl = %v, want 0 < ttl <= %v", ttl, cacheTTL)
	}

	if err := cache.InvalidateFlight(ctx, id); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	_, found, err = cache.GetFlight(ctx, id)
	if err != nil {
		t.Fatalf("get after invalidate: %v", err)
	}
	if found {
		t.Fatal("flight still in cache")
	}
}

func TestCacheSearch(t *testing.T) {
	cache := testCache(t)
	ctx := context.Background()
	origin := "T" + uuid.NewString()[:2]
	destination := "L" + uuid.NewString()[:2]
	date := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	flights := []Flight{{ID: uuid.NewString(), FlightNumber: "SU1"}}
	t.Cleanup(func() {
		_ = cache.InvalidateSearch(context.Background())
	})

	if err := cache.SetSearch(ctx, origin, destination, &date, flights); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := cache.SetSearch(ctx, origin, destination, nil, flights); err != nil {
		t.Fatalf("set without date: %v", err)
	}

	got, found, err := cache.GetSearch(ctx, origin, destination, &date)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found || len(got) != 1 {
		t.Fatalf("found = %v, flights = %d", found, len(got))
	}

	ttl, err := cache.client.TTL(ctx, searchKey(origin, destination, &date)).Result()
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > cacheTTL {
		t.Fatalf("ttl = %v, want 0 < ttl <= %v", ttl, cacheTTL)
	}

	if err := cache.InvalidateSearch(ctx); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	for _, d := range []*time.Time{&date, nil} {
		_, found, err := cache.GetSearch(ctx, origin, destination, d)
		if err != nil {
			t.Fatalf("get after invalidate: %v", err)
		}
		if found {
			t.Fatal("search still in cache")
		}
	}
}
