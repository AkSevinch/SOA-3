package flight

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	flightmigrations "soa3/db/migrations/flight"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_FLIGHT_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_FLIGHT_DB_DSN is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	goose.SetBaseFS(flightmigrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close migration db: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func insertTestFlight(t *testing.T, pool *pgxpool.Pool, totalSeats, availableSeats int32) string {
	t.Helper()

	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO flights (id, flight_number, airline_code, origin, destination, departure_time, arrival_time, total_seats, available_seats, price_minor, currency, status)
		VALUES ($1::uuid, $2, 'SU', 'SVO', 'LED', now() + interval '10 days', now() + interval '10 days 2 hours', $3, $4, 100000, 'RUB', 'SCHEDULED')`,
		id, "T"+uuid.NewString()[:6], totalSeats, availableSeats,
	)
	if err != nil {
		t.Fatalf("insert flight: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM seat_reservations WHERE flight_id = $1::uuid`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM flights WHERE id = $1::uuid`, id)
	})
	return id
}

func TestRepoReserveSeatsConcurrentLastSeat(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 1, 1)

	bookings := []string{uuid.NewString(), uuid.NewString()}
	start := make(chan struct{})
	errs := make(chan error, len(bookings))

	for _, bookingID := range bookings {
		go func(id string) {
			<-start
			_, err := repo.ReserveSeats(ctx, flightID, id, 1)
			errs <- err
		}(bookingID)
	}
	close(start)

	var success, exhausted int
	for range bookings {
		err := <-errs
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrNotEnoughSeats):
			exhausted++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success != 1 || exhausted != 1 {
		t.Fatalf("success = %d, exhausted = %d, want 1 and 1", success, exhausted)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 0 {
		t.Fatalf("available_seats = %d, want 0", available)
	}

	var active int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM seat_reservations WHERE flight_id = $1::uuid AND status = 'ACTIVE'`, flightID).Scan(&active); err != nil {
		t.Fatalf("query reservations: %v", err)
	}
	if active != 1 {
		t.Fatalf("active reservations = %d, want 1", active)
	}
}

func TestRepoReleaseReservationRestoresSeats(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 2, 2)
	bookingID := uuid.NewString()

	if _, err := repo.ReserveSeats(ctx, flightID, bookingID, 1); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 1 {
		t.Fatalf("available_seats = %d after reserve, want 1", available)
	}

	reservation, err := repo.ReleaseReservation(ctx, bookingID)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if reservation.Status != "RELEASED" {
		t.Fatalf("reservation status = %q, want RELEASED", reservation.Status)
	}

	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 2 {
		t.Fatalf("available_seats = %d after release, want 2", available)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM seat_reservations WHERE booking_id = $1::uuid`, bookingID).Scan(&status); err != nil {
		t.Fatalf("query reservation: %v", err)
	}
	if status != "RELEASED" {
		t.Fatalf("stored status = %q, want RELEASED", status)
	}
}

func TestRepoReserveSeatsIdempotent(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 5, 5)
	bookingID := uuid.NewString()

	first, err := repo.ReserveSeats(ctx, flightID, bookingID, 2)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	second, err := repo.ReserveSeats(ctx, flightID, bookingID, 2)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("reservation ids differ: %s and %s", first.ID, second.ID)
	}
	if second.Status != "ACTIVE" {
		t.Fatalf("status = %q, want ACTIVE", second.Status)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 3 {
		t.Fatalf("available_seats = %d, want 3", available)
	}

	var count int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM seat_reservations WHERE booking_id = $1::uuid`, bookingID).Scan(&count); err != nil {
		t.Fatalf("query reservations: %v", err)
	}
	if count != 1 {
		t.Fatalf("reservations = %d, want 1", count)
	}
}

func TestRepoReserveSeatsIdempotentWhenSoldOut(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 2, 2)
	bookingID := uuid.NewString()

	first, err := repo.ReserveSeats(ctx, flightID, bookingID, 2)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	second, err := repo.ReserveSeats(ctx, flightID, bookingID, 2)
	if err != nil {
		t.Fatalf("second reserve after sold out: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("reservation ids differ: %s and %s", first.ID, second.ID)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 0 {
		t.Fatalf("available_seats = %d, want 0", available)
	}
}

func TestRepoReserveSeatsRejectsDifferentDuplicate(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 5, 5)
	bookingID := uuid.NewString()

	if _, err := repo.ReserveSeats(ctx, flightID, bookingID, 2); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if _, err := repo.ReserveSeats(ctx, flightID, bookingID, 3); !errors.Is(err, ErrReservationExists) {
		t.Fatalf("second reserve error = %v, want ErrReservationExists", err)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 3 {
		t.Fatalf("available_seats = %d, want 3", available)
	}
}

func TestRepoReleaseReservationIdempotent(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	flightID := insertTestFlight(t, pool, 2, 2)
	bookingID := uuid.NewString()

	if _, err := repo.ReserveSeats(ctx, flightID, bookingID, 1); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := repo.ReleaseReservation(ctx, bookingID); err != nil {
		t.Fatalf("first release: %v", err)
	}
	second, err := repo.ReleaseReservation(ctx, bookingID)
	if err != nil {
		t.Fatalf("second release: %v", err)
	}
	if second.Status != "RELEASED" {
		t.Fatalf("status = %q, want RELEASED", second.Status)
	}

	var available int32
	if err := pool.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id = $1::uuid`, flightID).Scan(&available); err != nil {
		t.Fatalf("query seats: %v", err)
	}
	if available != 2 {
		t.Fatalf("available_seats = %d, want 2", available)
	}
}
