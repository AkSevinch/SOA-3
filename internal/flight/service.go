package flight

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

type repository interface {
	SearchFlights(ctx context.Context, origin, destination string, date *time.Time) ([]Flight, error)
	GetFlight(ctx context.Context, id string) (Flight, error)
	ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (Reservation, error)
	ReleaseReservation(ctx context.Context, bookingID string) (Reservation, error)
}

type cacheStore interface {
	GetFlight(ctx context.Context, id string) (Flight, bool, error)
	SetFlight(ctx context.Context, f Flight) error
	GetSearch(ctx context.Context, origin, destination string, date *time.Time) ([]Flight, bool, error)
	SetSearch(ctx context.Context, origin, destination string, date *time.Time, flights []Flight) error
	InvalidateFlight(ctx context.Context, id string) error
	InvalidateSearch(ctx context.Context) error
}

type Service struct {
	repo  repository
	cache cacheStore
}

func NewService(repo repository, cache cacheStore) *Service {
	return &Service{repo: repo, cache: cache}
}

func (s *Service) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) ([]Flight, error) {
	origin = strings.ToUpper(strings.TrimSpace(origin))
	destination = strings.ToUpper(strings.TrimSpace(destination))
	if origin == "" || destination == "" {
		return nil, fmt.Errorf("%w: origin and destination are required", ErrInvalidArgument)
	}

	flights, found, err := s.cache.GetSearch(ctx, origin, destination, date)
	if err != nil {
		log.Printf("cache get search: %v", err)
	} else if found {
		return flights, nil
	}

	flights, err = s.repo.SearchFlights(ctx, origin, destination, date)
	if err != nil {
		return nil, err
	}
	if err := s.cache.SetSearch(ctx, origin, destination, date, flights); err != nil {
		log.Printf("cache set search: %v", err)
	}
	return flights, nil
}

func (s *Service) GetFlight(ctx context.Context, id string) (Flight, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Flight{}, fmt.Errorf("%w: invalid flight id", ErrInvalidArgument)
	}

	f, found, err := s.cache.GetFlight(ctx, id)
	if err != nil {
		log.Printf("cache get flight %s: %v", id, err)
	} else if found {
		return f, nil
	}

	f, err = s.repo.GetFlight(ctx, id)
	if err != nil {
		return Flight{}, err
	}
	if err := s.cache.SetFlight(ctx, f); err != nil {
		log.Printf("cache set flight %s: %v", id, err)
	}
	return f, nil
}

func (s *Service) ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (Reservation, error) {
	if seatCount <= 0 {
		return Reservation{}, fmt.Errorf("%w: seat_count must be positive", ErrInvalidArgument)
	}
	if _, err := uuid.Parse(flightID); err != nil {
		return Reservation{}, fmt.Errorf("%w: invalid flight id", ErrInvalidArgument)
	}
	if _, err := uuid.Parse(bookingID); err != nil {
		return Reservation{}, fmt.Errorf("%w: invalid booking id", ErrInvalidArgument)
	}

	reservation, err := s.repo.ReserveSeats(ctx, flightID, bookingID, seatCount)
	if err != nil {
		return Reservation{}, err
	}
	s.invalidate(ctx, flightID)
	return reservation, nil
}

func (s *Service) ReleaseReservation(ctx context.Context, bookingID string) (Reservation, error) {
	if _, err := uuid.Parse(bookingID); err != nil {
		return Reservation{}, fmt.Errorf("%w: invalid booking id", ErrInvalidArgument)
	}

	reservation, err := s.repo.ReleaseReservation(ctx, bookingID)
	if err != nil {
		return Reservation{}, err
	}
	s.invalidate(ctx, reservation.FlightID)
	return reservation, nil
}

func (s *Service) invalidate(ctx context.Context, flightID string) {
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.cache.InvalidateFlight(cacheCtx, flightID); err != nil {
		log.Printf("cache invalidate flight %s: %v", flightID, err)
	}
	if err := s.cache.InvalidateSearch(cacheCtx); err != nil {
		log.Printf("cache invalidate search: %v", err)
	}
}
