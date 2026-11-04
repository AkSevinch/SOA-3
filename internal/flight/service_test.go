package flight

import (
	"context"
	"errors"
	"testing"
	"time"
)

const (
	testFlightID  = "00000000-0000-0000-0000-000000000001"
	testBookingID = "00000000-0000-0000-0000-000000000002"
)

type fakeRepo struct {
	searchResult []Flight
	searchErr    error
	searchCalls  int

	getResult Flight
	getErr    error
	getCalls  int

	reserveResult Reservation
	reserveErr    error
	reserveCalls  int
	onReserve     func()

	releaseResult Reservation
	releaseErr    error
	releaseCalls  int
}

func (r *fakeRepo) SearchFlights(context.Context, string, string, *time.Time) ([]Flight, error) {
	r.searchCalls++
	return r.searchResult, r.searchErr
}

func (r *fakeRepo) GetFlight(context.Context, string) (Flight, error) {
	r.getCalls++
	return r.getResult, r.getErr
}

func (r *fakeRepo) ReserveSeats(context.Context, string, string, int32) (Reservation, error) {
	r.reserveCalls++
	if r.onReserve != nil {
		r.onReserve()
	}
	return r.reserveResult, r.reserveErr
}

func (r *fakeRepo) ReleaseReservation(context.Context, string) (Reservation, error) {
	r.releaseCalls++
	return r.releaseResult, r.releaseErr
}

type fakeCache struct {
	flight       Flight
	flightFound  bool
	flightErr    error
	flightGets   int
	flightSets   int
	setFlightErr error

	search       []Flight
	searchFound  bool
	searchErr    error
	searchGets   int
	searchSets   int
	setSearchErr error

	invalidateFlightCalls  int
	invalidateFlightErr    error
	invalidatedFlightID    string
	invalidateFlightCtxErr error
	invalidateSearchCalls  int
	invalidateSearchErr    error
	invalidateSearchCtxErr error
}

func (c *fakeCache) GetFlight(context.Context, string) (Flight, bool, error) {
	c.flightGets++
	return c.flight, c.flightFound, c.flightErr
}

func (c *fakeCache) SetFlight(context.Context, Flight) error {
	c.flightSets++
	return c.setFlightErr
}

func (c *fakeCache) GetSearch(context.Context, string, string, *time.Time) ([]Flight, bool, error) {
	c.searchGets++
	return c.search, c.searchFound, c.searchErr
}

func (c *fakeCache) SetSearch(context.Context, string, string, *time.Time, []Flight) error {
	c.searchSets++
	return c.setSearchErr
}

func (c *fakeCache) InvalidateFlight(ctx context.Context, id string) error {
	c.invalidateFlightCalls++
	c.invalidatedFlightID = id
	c.invalidateFlightCtxErr = ctx.Err()
	return c.invalidateFlightErr
}

func (c *fakeCache) InvalidateSearch(ctx context.Context) error {
	c.invalidateSearchCalls++
	c.invalidateSearchCtxErr = ctx.Err()
	return c.invalidateSearchErr
}

func TestGetFlightCacheHit(t *testing.T) {
	repo := &fakeRepo{}
	cache := &fakeCache{flight: Flight{ID: testFlightID, FlightNumber: "SU1"}, flightFound: true}
	service := NewService(repo, cache)

	got, err := service.GetFlight(context.Background(), testFlightID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FlightNumber != "SU1" {
		t.Fatalf("flight = %+v", got)
	}
	if repo.getCalls != 0 {
		t.Fatalf("repo calls = %d, want 0", repo.getCalls)
	}
	if cache.flightSets != 0 {
		t.Fatalf("cache sets = %d, want 0", cache.flightSets)
	}
}

func TestGetFlightCacheMiss(t *testing.T) {
	repo := &fakeRepo{getResult: Flight{ID: testFlightID, FlightNumber: "SU2"}}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	got, err := service.GetFlight(context.Background(), testFlightID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.FlightNumber != "SU2" {
		t.Fatalf("flight = %+v", got)
	}
	if repo.getCalls != 1 {
		t.Fatalf("repo calls = %d, want 1", repo.getCalls)
	}
	if cache.flightSets != 1 {
		t.Fatalf("cache sets = %d, want 1", cache.flightSets)
	}
}

func TestGetFlightFallsBackOnCacheError(t *testing.T) {
	repo := &fakeRepo{getResult: Flight{ID: testFlightID}}
	cache := &fakeCache{flightErr: errors.New("redis down")}
	service := NewService(repo, cache)

	if _, err := service.GetFlight(context.Background(), testFlightID); err != nil {
		t.Fatalf("get: %v", err)
	}
	if repo.getCalls != 1 {
		t.Fatalf("repo calls = %d, want 1", repo.getCalls)
	}
}

func TestSearchFlightsCacheHit(t *testing.T) {
	repo := &fakeRepo{}
	cache := &fakeCache{search: []Flight{{ID: testFlightID}}, searchFound: true}
	service := NewService(repo, cache)

	flights, err := service.SearchFlights(context.Background(), "svo", "led", nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(flights) != 1 {
		t.Fatalf("flights = %d, want 1", len(flights))
	}
	if repo.searchCalls != 0 {
		t.Fatalf("repo calls = %d, want 0", repo.searchCalls)
	}
}

func TestSearchFlightsCacheMiss(t *testing.T) {
	repo := &fakeRepo{searchResult: []Flight{{ID: testFlightID}}}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	flights, err := service.SearchFlights(context.Background(), "svo", "led", nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(flights) != 1 {
		t.Fatalf("flights = %d, want 1", len(flights))
	}
	if repo.searchCalls != 1 {
		t.Fatalf("repo calls = %d, want 1", repo.searchCalls)
	}
	if cache.searchSets != 1 {
		t.Fatalf("cache sets = %d, want 1", cache.searchSets)
	}
}

func TestReserveSeatsInvalidatesCache(t *testing.T) {
	repo := &fakeRepo{reserveResult: Reservation{ID: "r1", FlightID: testFlightID}}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	if _, err := service.ReserveSeats(context.Background(), testFlightID, testBookingID, 2); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if cache.invalidateFlightCalls != 1 {
		t.Fatalf("invalidate flight calls = %d, want 1", cache.invalidateFlightCalls)
	}
	if cache.invalidatedFlightID != testFlightID {
		t.Fatalf("invalidated id = %q", cache.invalidatedFlightID)
	}
	if cache.invalidateSearchCalls != 1 {
		t.Fatalf("invalidate search calls = %d, want 1", cache.invalidateSearchCalls)
	}
}

func TestReserveSeatsErrorDoesNotInvalidateCache(t *testing.T) {
	repo := &fakeRepo{reserveErr: ErrNotEnoughSeats}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	if _, err := service.ReserveSeats(context.Background(), testFlightID, testBookingID, 2); !errors.Is(err, ErrNotEnoughSeats) {
		t.Fatalf("error = %v", err)
	}
	if cache.invalidateFlightCalls != 0 || cache.invalidateSearchCalls != 0 {
		t.Fatal("cache was invalidated")
	}
}

func TestReleaseReservationInvalidatesCache(t *testing.T) {
	repo := &fakeRepo{releaseResult: Reservation{ID: "r1", FlightID: testFlightID}}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	if _, err := service.ReleaseReservation(context.Background(), testBookingID); err != nil {
		t.Fatalf("release: %v", err)
	}
	if cache.invalidateFlightCalls != 1 {
		t.Fatalf("invalidate flight calls = %d, want 1", cache.invalidateFlightCalls)
	}
	if cache.invalidateSearchCalls != 1 {
		t.Fatalf("invalidate search calls = %d, want 1", cache.invalidateSearchCalls)
	}
}

func TestInvalidationIgnoresCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &fakeRepo{
		reserveResult: Reservation{ID: "r1", FlightID: testFlightID},
		onReserve:     cancel,
	}
	cache := &fakeCache{}
	service := NewService(repo, cache)

	if _, err := service.ReserveSeats(ctx, testFlightID, testBookingID, 1); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if cache.invalidateFlightCtxErr != nil {
		t.Fatalf("invalidate flight context canceled: %v", cache.invalidateFlightCtxErr)
	}
	if cache.invalidateSearchCtxErr != nil {
		t.Fatalf("invalidate search context canceled: %v", cache.invalidateSearchCtxErr)
	}
}
