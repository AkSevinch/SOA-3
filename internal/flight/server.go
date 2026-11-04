package flight

import (
	"context"
	"errors"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightv1 "soa3/gen/go/flight/v1"
)

type Server struct {
	flightv1.UnimplementedFlightServiceServer
	svc *Service
}

func NewServer(svc *Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	var date *time.Time
	if req.GetDate() != nil {
		value := req.GetDate().AsTime()
		date = &value
	}
	flights, err := s.svc.SearchFlights(ctx, req.GetOrigin(), req.GetDestination(), date)
	if err != nil {
		return nil, toGRPCError(err)
	}
	result := make([]*flightv1.Flight, 0, len(flights))
	for _, f := range flights {
		result = append(result, toProtoFlight(f))
	}
	return &flightv1.SearchFlightsResponse{Flights: result}, nil
}

func (s *Server) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	f, err := s.svc.GetFlight(ctx, req.GetId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &flightv1.GetFlightResponse{Flight: toProtoFlight(f)}, nil
}

func (s *Server) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	reservation, err := s.svc.ReserveSeats(ctx, req.GetFlightId(), req.GetBookingId(), req.GetSeatCount())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &flightv1.ReserveSeatsResponse{
		ReservationId:  reservation.ID,
		BookingId:      reservation.BookingID,
		SeatCount:      reservation.SeatCount,
		AvailableSeats: reservation.AvailableSeats,
		Status:         toProtoReservationStatus(reservation.Status),
	}, nil
}

func (s *Server) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	reservation, err := s.svc.ReleaseReservation(ctx, req.GetBookingId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &flightv1.ReleaseReservationResponse{
		ReservationId: reservation.ID,
		BookingId:     reservation.BookingID,
		ReleasedSeats: reservation.SeatCount,
		Status:        toProtoReservationStatus(reservation.Status),
	}, nil
}

func toProtoFlight(f Flight) *flightv1.Flight {
	return &flightv1.Flight{
		Id:             f.ID,
		FlightNumber:   f.FlightNumber,
		AirlineCode:    f.AirlineCode,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime),
		ArrivalTime:    timestamppb.New(f.ArrivalTime),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		PriceMinor:     f.PriceMinor,
		Currency:       f.Currency,
		Status:         toProtoFlightStatus(f.Status),
	}
}

func toProtoFlightStatus(value string) flightv1.FlightStatus {
	switch value {
	case "SCHEDULED":
		return flightv1.FlightStatus_FLIGHT_STATUS_SCHEDULED
	case "DEPARTED":
		return flightv1.FlightStatus_FLIGHT_STATUS_DEPARTED
	case "CANCELLED":
		return flightv1.FlightStatus_FLIGHT_STATUS_CANCELLED
	case "COMPLETED":
		return flightv1.FlightStatus_FLIGHT_STATUS_COMPLETED
	default:
		return flightv1.FlightStatus_FLIGHT_STATUS_UNSPECIFIED
	}
}

func toProtoReservationStatus(value string) flightv1.ReservationStatus {
	switch value {
	case "ACTIVE":
		return flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	case "RELEASED":
		return flightv1.ReservationStatus_RESERVATION_STATUS_RELEASED
	case "EXPIRED":
		return flightv1.ReservationStatus_RESERVATION_STATUS_EXPIRED
	default:
		return flightv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED
	}
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrNotEnoughSeats):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, ErrFlightNotScheduled):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrReservationExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrReservationNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		log.Printf("flight service error: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
