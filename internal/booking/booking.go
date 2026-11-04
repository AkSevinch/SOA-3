package booking

import (
	"errors"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("booking not found")
	ErrNotConfirmed    = errors.New("booking is not confirmed")
)

type Booking struct {
	ID              string
	UserID          string
	FlightID        string
	PassengerName   string
	PassengerEmail  string
	SeatCount       int32
	TotalPriceMinor int64
	Currency        string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CancelledAt     *time.Time
}

type CreateInput struct {
	UserID         string
	FlightID       string
	PassengerName  string
	PassengerEmail string
	SeatCount      int32
}
