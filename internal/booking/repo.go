package booking

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const bookingColumns = `id::text, user_id, flight_id::text, passenger_name, passenger_email, seat_count, total_price_minor, currency, status::text, created_at, updated_at, cancelled_at`

type scanner interface {
	Scan(dest ...any) error
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, b Booking) (Booking, error) {
	return scanBooking(r.pool.QueryRow(ctx, `
		INSERT INTO bookings (id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price_minor, currency, status)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, 'CONFIRMED')
		RETURNING `+bookingColumns,
		b.ID, b.UserID, b.FlightID, b.PassengerName, b.PassengerEmail, b.SeatCount, b.TotalPriceMinor, b.Currency,
	))
}

func (r *Repo) Get(ctx context.Context, id string) (Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	return b, err
}

func (r *Repo) ListByUser(ctx context.Context, userID string) ([]Booking, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bookings := make([]Booking, 0)
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bookings, nil
}

func (r *Repo) Cancel(ctx context.Context, id string) (Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx, `
		UPDATE bookings
		SET status = 'CANCELLED', cancelled_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status = 'CONFIRMED'
		RETURNING `+bookingColumns, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotConfirmed
	}
	return b, err
}

func scanBooking(row scanner) (Booking, error) {
	var b Booking
	err := row.Scan(
		&b.ID,
		&b.UserID,
		&b.FlightID,
		&b.PassengerName,
		&b.PassengerEmail,
		&b.SeatCount,
		&b.TotalPriceMinor,
		&b.Currency,
		&b.Status,
		&b.CreatedAt,
		&b.UpdatedAt,
		&b.CancelledAt,
	)
	return b, err
}
