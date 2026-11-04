package booking

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	flightv1 "soa3/gen/go/flight/v1"
)

type flightAPI interface {
	SearchFlights(ctx context.Context, origin, destination string, date *time.Time) (*flightv1.SearchFlightsResponse, error)
	GetFlight(ctx context.Context, id string) (*flightv1.GetFlightResponse, error)
	ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (*flightv1.ReserveSeatsResponse, error)
	ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error)
}

type bookingRepo interface {
	Create(ctx context.Context, b Booking) (Booking, error)
	Get(ctx context.Context, id string) (Booking, error)
	ListByUser(ctx context.Context, userID string) ([]Booking, error)
	Cancel(ctx context.Context, id string) (Booking, error)
}

type Service struct {
	repo    bookingRepo
	flights flightAPI
}

func NewService(repo bookingRepo, flights flightAPI) *Service {
	return &Service{repo: repo, flights: flights}
}

func (s *Service) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) (*flightv1.SearchFlightsResponse, error) {
	origin = strings.ToUpper(strings.TrimSpace(origin))
	destination = strings.ToUpper(strings.TrimSpace(destination))
	if origin == "" || destination == "" {
		return nil, fmt.Errorf("%w: origin and destination are required", ErrInvalidArgument)
	}
	return s.flights.SearchFlights(ctx, origin, destination, date)
}

func (s *Service) GetFlight(ctx context.Context, id string) (*flightv1.GetFlightResponse, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: invalid flight id", ErrInvalidArgument)
	}
	return s.flights.GetFlight(ctx, id)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Booking, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerEmail = strings.TrimSpace(input.PassengerEmail)

	if input.UserID == "" {
		return Booking{}, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	if _, err := uuid.Parse(input.FlightID); err != nil {
		return Booking{}, fmt.Errorf("%w: invalid flight id", ErrInvalidArgument)
	}
	if input.PassengerName == "" {
		return Booking{}, fmt.Errorf("%w: passenger_name is required", ErrInvalidArgument)
	}
	if input.PassengerEmail == "" {
		return Booking{}, fmt.Errorf("%w: passenger_email is required", ErrInvalidArgument)
	}
	if input.SeatCount <= 0 {
		return Booking{}, fmt.Errorf("%w: seat_count must be positive", ErrInvalidArgument)
	}

	bookingID := uuid.NewString()

	flightResponse, err := s.flights.GetFlight(ctx, input.FlightID)
	if err != nil {
		return Booking{}, err
	}
	selectedFlight := flightResponse.GetFlight()
	if selectedFlight == nil {
		return Booking{}, fmt.Errorf("%w: empty flight response", ErrInvalidArgument)
	}

	if _, err := s.flights.ReserveSeats(ctx, input.FlightID, bookingID, input.SeatCount); err != nil {
		return Booking{}, err
	}

	created, err := s.repo.Create(ctx, Booking{
		ID:              bookingID,
		UserID:          input.UserID,
		FlightID:        input.FlightID,
		PassengerName:   input.PassengerName,
		PassengerEmail:  input.PassengerEmail,
		SeatCount:       input.SeatCount,
		TotalPriceMinor: int64(input.SeatCount) * selectedFlight.GetPriceMinor(),
		Currency:        selectedFlight.GetCurrency(),
		Status:          "CONFIRMED",
	})
	if err != nil {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, releaseErr := s.flights.ReleaseReservation(releaseCtx, bookingID)
		cancel()
		if releaseErr != nil {
			log.Printf("release reservation for booking %s: %v", bookingID, releaseErr)
		}
		return Booking{}, err
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, id string) (Booking, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Booking{}, fmt.Errorf("%w: invalid booking id", ErrInvalidArgument)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Cancel(ctx context.Context, id string) (Booking, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Booking{}, fmt.Errorf("%w: invalid booking id", ErrInvalidArgument)
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Booking{}, err
	}
	if current.Status != "CONFIRMED" {
		return Booking{}, ErrNotConfirmed
	}
	if _, err := s.flights.ReleaseReservation(ctx, current.ID); err != nil {
		return Booking{}, err
	}
	return s.repo.Cancel(ctx, id)
}

func (s *Service) List(ctx context.Context, userID string) ([]Booking, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	return s.repo.ListByUser(ctx, userID)
}
