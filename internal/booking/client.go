package booking

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightv1 "soa3/gen/go/flight/v1"
)

type FlightClient struct {
	client flightv1.FlightServiceClient
}

func NewFlightClient(conn grpc.ClientConnInterface) *FlightClient {
	return &FlightClient{client: flightv1.NewFlightServiceClient(conn)}
}

func (c *FlightClient) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) (*flightv1.SearchFlightsResponse, error) {
	req := &flightv1.SearchFlightsRequest{Origin: origin, Destination: destination}
	if date != nil {
		req.Date = timestamppb.New(*date)
	}
	return c.client.SearchFlights(ctx, req)
}

func (c *FlightClient) GetFlight(ctx context.Context, id string) (*flightv1.GetFlightResponse, error) {
	return c.client.GetFlight(ctx, &flightv1.GetFlightRequest{Id: id})
}

func (c *FlightClient) ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (*flightv1.ReserveSeatsResponse, error) {
	return c.client.ReserveSeats(ctx, &flightv1.ReserveSeatsRequest{
		FlightId:  flightID,
		BookingId: bookingID,
		SeatCount: seatCount,
	})
}

func (c *FlightClient) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error) {
	return c.client.ReleaseReservation(ctx, &flightv1.ReleaseReservationRequest{BookingId: bookingID})
}
