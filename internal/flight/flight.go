package flight

import (
	"errors"
	"time"
)

var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("flight not found")
	ErrNotEnoughSeats      = errors.New("not enough seats")
	ErrFlightNotScheduled  = errors.New("flight is not scheduled")
	ErrReservationExists   = errors.New("reservation already exists")
	ErrReservationNotFound = errors.New("reservation not found")
)

type Flight struct {
	ID             string
	FlightNumber   string
	AirlineCode    string
	Origin         string
	Destination    string
	DepartureTime  time.Time
	ArrivalTime    time.Time
	TotalSeats     int32
	AvailableSeats int32
	PriceMinor     int64
	Currency       string
	Status         string
}

type Reservation struct {
	ID             string
	FlightID       string
	BookingID      string
	SeatCount      int32
	AvailableSeats int32
	Status         string
}
