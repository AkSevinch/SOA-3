package flight

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const flightColumns = `id::text, flight_number, airline_code, origin, destination, departure_time, arrival_time, total_seats, available_seats, price_minor, currency, status::text`

type scanner interface {
	Scan(dest ...any) error
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) ([]Flight, error) {
	query := `SELECT ` + flightColumns + ` FROM flights WHERE origin = $1 AND destination = $2 AND status = 'SCHEDULED'`
	args := []any{origin, destination}
	if date != nil {
		query += ` AND (departure_time AT TIME ZONE 'UTC')::date = $3::date`
		args = append(args, date.Format("2006-01-02"))
	}
	query += ` ORDER BY departure_time`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	flights := make([]Flight, 0)
	for rows.Next() {
		f, err := scanFlight(rows)
		if err != nil {
			return nil, err
		}
		flights = append(flights, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return flights, nil
}

func (r *Repo) GetFlight(ctx context.Context, id string) (Flight, error) {
	f, err := scanFlight(r.pool.QueryRow(ctx, `SELECT `+flightColumns+` FROM flights WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Flight{}, ErrNotFound
	}
	return f, err
}

func (r *Repo) ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback(ctx)

	var available int32
	var status string
	err = tx.QueryRow(ctx, `SELECT available_seats, status::text FROM flights WHERE id = $1::uuid FOR UPDATE`, flightID).Scan(&available, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	existing, err := scanReservation(tx.QueryRow(ctx, `
		SELECT id::text, flight_id::text, booking_id::text, seat_count, status::text
		FROM seat_reservations
		WHERE booking_id = $1::uuid`, bookingID,
	))
	switch {
	case err == nil:
		if existing.Status == "ACTIVE" && existing.FlightID == flightID && existing.SeatCount == seatCount {
			existing.AvailableSeats = available
			return existing, nil
		}
		return Reservation{}, ErrReservationExists
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return Reservation{}, err
	}

	if status != "SCHEDULED" {
		return Reservation{}, ErrFlightNotScheduled
	}
	if available < seatCount {
		return Reservation{}, ErrNotEnoughSeats
	}

	_, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats - $1, updated_at = now() WHERE id = $2::uuid`, seatCount, flightID)
	if err != nil {
		return Reservation{}, err
	}

	reservation, err := scanReservation(tx.QueryRow(ctx, `
		INSERT INTO seat_reservations (flight_id, booking_id, seat_count, status)
		VALUES ($1::uuid, $2::uuid, $3, 'ACTIVE')
		RETURNING id::text, flight_id::text, booking_id::text, seat_count, status::text`,
		flightID, bookingID, seatCount,
	))
	if isUniqueViolation(err) {
		return Reservation{}, ErrReservationExists
	}
	if err != nil {
		return Reservation{}, err
	}
	reservation.AvailableSeats = available - seatCount

	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	return reservation, nil
}

func (r *Repo) ReleaseReservation(ctx context.Context, bookingID string) (Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback(ctx)

	reservation, err := scanReservation(tx.QueryRow(ctx, `
		SELECT id::text, flight_id::text, booking_id::text, seat_count, status::text
		FROM seat_reservations
		WHERE booking_id = $1::uuid
		FOR UPDATE`, bookingID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrReservationNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	if reservation.Status == "RELEASED" {
		return reservation, nil
	}
	if reservation.Status != "ACTIVE" {
		return Reservation{}, ErrReservationNotFound
	}

	_, err = tx.Exec(ctx, `UPDATE seat_reservations SET status = 'RELEASED', updated_at = now() WHERE id = $1::uuid`, reservation.ID)
	if err != nil {
		return Reservation{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats + $1, updated_at = now() WHERE id = $2::uuid`, reservation.SeatCount, reservation.FlightID)
	if err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	reservation.Status = "RELEASED"
	return reservation, nil
}

func scanReservation(row scanner) (Reservation, error) {
	var r Reservation
	err := row.Scan(&r.ID, &r.FlightID, &r.BookingID, &r.SeatCount, &r.Status)
	return r, err
}

func scanFlight(row scanner) (Flight, error) {
	var f Flight
	err := row.Scan(
		&f.ID,
		&f.FlightNumber,
		&f.AirlineCode,
		&f.Origin,
		&f.Destination,
		&f.DepartureTime,
		&f.ArrivalTime,
		&f.TotalSeats,
		&f.AvailableSeats,
		&f.PriceMinor,
		&f.Currency,
		&f.Status,
	)
	return f, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
