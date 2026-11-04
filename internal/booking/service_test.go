package booking

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	flightv1 "soa3/gen/go/flight/v1"
)

const (
	testFlightID  = "00000000-0000-0000-0000-000000000001"
	testBookingID = "00000000-0000-0000-0000-000000000002"
)

type fakeFlight struct {
	getResponse   *flightv1.GetFlightResponse
	getErr        error
	reserveErr    error
	releaseErr    error
	releaseCalls  int
	releaseCtxErr error
}

func (f *fakeFlight) SearchFlights(context.Context, string, string, *time.Time) (*flightv1.SearchFlightsResponse, error) {
	return &flightv1.SearchFlightsResponse{}, nil
}

func (f *fakeFlight) GetFlight(context.Context, string) (*flightv1.GetFlightResponse, error) {
	return f.getResponse, f.getErr
}

func (f *fakeFlight) ReserveSeats(_ context.Context, flightID, bookingID string, seatCount int32) (*flightv1.ReserveSeatsResponse, error) {
	if f.reserveErr != nil {
		return nil, f.reserveErr
	}
	return &flightv1.ReserveSeatsResponse{
		ReservationId: testBookingID,
		BookingId:     bookingID,
		SeatCount:     seatCount,
		Status:        flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE,
	}, nil
}

func (f *fakeFlight) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error) {
	f.releaseCalls++
	f.releaseCtxErr = ctx.Err()
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	return &flightv1.ReleaseReservationResponse{
		BookingId: bookingID,
		Status:    flightv1.ReservationStatus_RESERVATION_STATUS_RELEASED,
	}, nil
}

type fakeRepo struct {
	createCalls int
	createErr   error
	created     Booking
	onCreate    func()

	getResult    Booking
	getErr       error
	cancelResult Booking
	cancelErr    error
	cancelCalls  int
}

func (r *fakeRepo) Create(_ context.Context, b Booking) (Booking, error) {
	r.createCalls++
	r.created = b
	if r.onCreate != nil {
		r.onCreate()
	}
	if r.createErr != nil {
		return Booking{}, r.createErr
	}
	return b, nil
}

func (r *fakeRepo) Get(context.Context, string) (Booking, error) {
	return r.getResult, r.getErr
}

func (r *fakeRepo) ListByUser(context.Context, string) ([]Booking, error) {
	return nil, nil
}

func (r *fakeRepo) Cancel(context.Context, string) (Booking, error) {
	r.cancelCalls++
	return r.cancelResult, r.cancelErr
}

func validFlightResponse() *flightv1.GetFlightResponse {
	return &flightv1.GetFlightResponse{
		Flight: &flightv1.Flight{
			Id:         testFlightID,
			PriceMinor: 750000,
			Currency:   "RUB",
		},
	}
}

func validInput() CreateInput {
	return CreateInput{
		UserID:         "user-1",
		FlightID:       testFlightID,
		PassengerName:  "Ivan Petrov",
		PassengerEmail: "ivan@example.com",
		SeatCount:      2,
	}
}

func TestCreateDoesNotCreateOnReserveError(t *testing.T) {
	repo := &fakeRepo{}
	flights := &fakeFlight{
		getResponse: validFlightResponse(),
		reserveErr:  status.Error(codes.ResourceExhausted, "not enough seats"),
	}
	service := NewService(repo, flights)

	if _, err := service.Create(context.Background(), validInput()); err == nil {
		t.Fatal("expected error")
	}
	if repo.createCalls != 0 {
		t.Fatalf("repo.Create called %d times", repo.createCalls)
	}
	if flights.releaseCalls != 0 {
		t.Fatalf("ReleaseReservation called %d times", flights.releaseCalls)
	}
}

func TestCreateCalculatesTotalPrice(t *testing.T) {
	repo := &fakeRepo{}
	flights := &fakeFlight{getResponse: validFlightResponse()}
	service := NewService(repo, flights)

	created, err := service.Create(context.Background(), validInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.TotalPriceMinor != 1500000 {
		t.Fatalf("total price = %d, want 1500000", created.TotalPriceMinor)
	}
	if created.Status != "CONFIRMED" {
		t.Fatalf("status = %q, want CONFIRMED", created.Status)
	}
	if repo.created.TotalPriceMinor != created.TotalPriceMinor {
		t.Fatalf("stored total price = %d", repo.created.TotalPriceMinor)
	}
}

func TestCreateCompensatesOnRepoError(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("db down")}
	flights := &fakeFlight{getResponse: validFlightResponse()}
	service := NewService(repo, flights)

	if _, err := service.Create(context.Background(), validInput()); err == nil {
		t.Fatal("expected error")
	}
	if flights.releaseCalls != 1 {
		t.Fatalf("ReleaseReservation called %d times, want 1", flights.releaseCalls)
	}
}

func TestCompensationIgnoresCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &fakeRepo{createErr: errors.New("db down")}
	repo.onCreate = cancel
	flights := &fakeFlight{getResponse: validFlightResponse()}
	service := NewService(repo, flights)

	if _, err := service.Create(ctx, validInput()); err == nil {
		t.Fatal("expected error")
	}
	if flights.releaseCalls != 1 {
		t.Fatalf("ReleaseReservation called %d times, want 1", flights.releaseCalls)
	}
	if flights.releaseCtxErr != nil {
		t.Fatalf("compensation context canceled: %v", flights.releaseCtxErr)
	}
}

func TestCancelSkipsReleaseWhenNotConfirmed(t *testing.T) {
	repo := &fakeRepo{getResult: Booking{ID: testBookingID, Status: "CANCELLED"}}
	flights := &fakeFlight{}
	service := NewService(repo, flights)

	_, err := service.Cancel(context.Background(), testBookingID)
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("error = %v, want ErrNotConfirmed", err)
	}
	if flights.releaseCalls != 0 {
		t.Fatalf("ReleaseReservation called %d times", flights.releaseCalls)
	}
	if repo.cancelCalls != 0 {
		t.Fatalf("repo.Cancel called %d times", repo.cancelCalls)
	}
}

func TestCancelReleasesThenCancels(t *testing.T) {
	repo := &fakeRepo{
		getResult:    Booking{ID: testBookingID, Status: "CONFIRMED"},
		cancelResult: Booking{ID: testBookingID, Status: "CANCELLED"},
	}
	flights := &fakeFlight{}
	service := NewService(repo, flights)

	cancelled, err := service.Cancel(context.Background(), testBookingID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if flights.releaseCalls != 1 {
		t.Fatalf("ReleaseReservation called %d times", flights.releaseCalls)
	}
	if repo.cancelCalls != 1 {
		t.Fatalf("repo.Cancel called %d times", repo.cancelCalls)
	}
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("status = %q", cancelled.Status)
	}
}
