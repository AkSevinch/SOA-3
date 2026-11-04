package booking

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	flightv1 "soa3/gen/go/flight/v1"
)

type Handler struct {
	svc *Service
}

func NewRouter(svc *Service) http.Handler {
	h := &Handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /flights", h.searchFlights)
	mux.HandleFunc("GET /flights/{id}", h.getFlight)
	mux.HandleFunc("POST /bookings", h.createBooking)
	mux.HandleFunc("GET /bookings", h.listBookings)
	mux.HandleFunc("GET /bookings/{id}", h.getBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.cancelBooking)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) searchFlights(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	origin := query.Get("origin")
	destination := query.Get("destination")
	if origin == "" || destination == "" {
		writeServiceError(w, fmt.Errorf("%w: origin and destination are required", ErrInvalidArgument))
		return
	}

	var date *time.Time
	if raw := query.Get("date"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			writeServiceError(w, fmt.Errorf("%w: invalid date", ErrInvalidArgument))
			return
		}
		date = &parsed
	}

	response, err := h.svc.SearchFlights(r.Context(), origin, destination, date)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	flights := make([]flightResponse, 0, len(response.GetFlights()))
	for _, f := range response.GetFlights() {
		flights = append(flights, toFlightResponse(f))
	}
	writeJSON(w, http.StatusOK, searchFlightsResponse{Flights: flights})
}

func (h *Handler) getFlight(w http.ResponseWriter, r *http.Request) {
	response, err := h.svc.GetFlight(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFlightResponse(response.GetFlight()))
}

func (h *Handler) createBooking(w http.ResponseWriter, r *http.Request) {
	var request createBookingRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeServiceError(w, fmt.Errorf("%w: %v", ErrInvalidArgument, err))
		return
	}

	created, err := h.svc.Create(r.Context(), CreateInput{
		UserID:         request.UserID,
		FlightID:       request.FlightID,
		PassengerName:  request.PassengerName,
		PassengerEmail: request.PassengerEmail,
		SeatCount:      request.SeatCount,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBookingResponse(created))
}

func (h *Handler) getBooking(w http.ResponseWriter, r *http.Request) {
	entity, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingResponse(entity))
}

func (h *Handler) cancelBooking(w http.ResponseWriter, r *http.Request) {
	entity, err := h.svc.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingResponse(entity))
}

func (h *Handler) listBookings(w http.ResponseWriter, r *http.Request) {
	bookings, err := h.svc.List(r.Context(), r.URL.Query().Get("user_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result := make([]bookingResponse, 0, len(bookings))
	for _, entity := range bookings {
		result = append(result, toBookingResponse(entity))
	}
	writeJSON(w, http.StatusOK, listBookingsResponse{Bookings: result})
}

type flightResponse struct {
	ID             string    `json:"id"`
	FlightNumber   string    `json:"flight_number"`
	AirlineCode    string    `json:"airline_code"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	TotalSeats     int32     `json:"total_seats"`
	AvailableSeats int32     `json:"available_seats"`
	PriceMinor     int64     `json:"price_minor"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
}

type searchFlightsResponse struct {
	Flights []flightResponse `json:"flights"`
}

type bookingResponse struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	FlightID        string     `json:"flight_id"`
	PassengerName   string     `json:"passenger_name"`
	PassengerEmail  string     `json:"passenger_email"`
	SeatCount       int32      `json:"seat_count"`
	TotalPriceMinor int64      `json:"total_price_minor"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CancelledAt     *time.Time `json:"cancelled_at,omitempty"`
}

type listBookingsResponse struct {
	Bookings []bookingResponse `json:"bookings"`
}

type createBookingRequest struct {
	UserID         string `json:"user_id"`
	FlightID       string `json:"flight_id"`
	PassengerName  string `json:"passenger_name"`
	PassengerEmail string `json:"passenger_email"`
	SeatCount      int32  `json:"seat_count"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func toFlightResponse(f *flightv1.Flight) flightResponse {
	return flightResponse{
		ID:             f.GetId(),
		FlightNumber:   f.GetFlightNumber(),
		AirlineCode:    f.GetAirlineCode(),
		Origin:         f.GetOrigin(),
		Destination:    f.GetDestination(),
		DepartureTime:  f.GetDepartureTime().AsTime(),
		ArrivalTime:    f.GetArrivalTime().AsTime(),
		TotalSeats:     f.GetTotalSeats(),
		AvailableSeats: f.GetAvailableSeats(),
		PriceMinor:     f.GetPriceMinor(),
		Currency:       f.GetCurrency(),
		Status:         strings.TrimPrefix(f.GetStatus().String(), "FLIGHT_STATUS_"),
	}
}

func toBookingResponse(b Booking) bookingResponse {
	return bookingResponse{
		ID:              b.ID,
		UserID:          b.UserID,
		FlightID:        b.FlightID,
		PassengerName:   b.PassengerName,
		PassengerEmail:  b.PassengerEmail,
		SeatCount:       b.SeatCount,
		TotalPriceMinor: b.TotalPriceMinor,
		Currency:        b.Currency,
		Status:          b.Status,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
		CancelledAt:     b.CancelledAt,
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case errors.Is(err, ErrNotConfirmed):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	default:
		writeGRPCError(w, err)
	}
}

func writeGRPCError(w http.ResponseWriter, err error) {
	switch status.Code(err) {
	case codes.InvalidArgument:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: status.Convert(err).Message()})
	case codes.NotFound:
		writeJSON(w, http.StatusNotFound, errorResponse{Error: status.Convert(err).Message()})
	case codes.ResourceExhausted, codes.AlreadyExists, codes.FailedPrecondition:
		writeJSON(w, http.StatusConflict, errorResponse{Error: status.Convert(err).Message()})
	case codes.Unauthenticated:
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: status.Convert(err).Message()})
	case codes.PermissionDenied:
		writeJSON(w, http.StatusForbidden, errorResponse{Error: status.Convert(err).Message()})
	case codes.Unavailable, codes.DeadlineExceeded:
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: status.Convert(err).Message()})
	default:
		log.Printf("http handler error: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
	}
}
