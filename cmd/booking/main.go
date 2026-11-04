package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bookingmigrations "soa3/db/migrations/booking"
	"soa3/internal/auth"
	"soa3/internal/booking"
	"soa3/internal/config"
)

func main() {
	cfg, err := config.LoadBooking()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := migrate(cfg.DBDSN); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
	defer dialCancel()
	conn, err := grpc.DialContext(dialCtx, cfg.FlightGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			auth.UnaryClientInterceptor(cfg.FlightAPIKey),
			booking.UnaryRetryInterceptor(booking.DefaultRetryConfig()),
		),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("connect flight service: %v", err)
	}
	defer conn.Close()

	repo := booking.NewRepo(pool)
	client := booking.NewFlightClient(conn)
	service := booking.NewService(repo, client)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           booking.NewRouter(service),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("booking service listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("booking service stopped")
}

func migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(bookingmigrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}
