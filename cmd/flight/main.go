package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	flightmigrations "soa3/db/migrations/flight"
	flightv1 "soa3/gen/go/flight/v1"
	"soa3/internal/auth"
	"soa3/internal/config"
	"soa3/internal/flight"
)

func main() {
	cfg, err := config.LoadFlight()
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

	cache := flight.NewCache(cfg.RedisSentinelAddrs, cfg.RedisMasterName)
	defer cache.Close()
	if err := cache.Ping(ctx); err != nil {
		log.Printf("redis ping: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	repo := flight.NewRepo(pool)
	service := flight.NewService(repo, cache)
	server := grpc.NewServer(
		grpc.UnaryInterceptor(auth.UnaryServerInterceptor(cfg.APIKey)),
		grpc.StreamInterceptor(auth.StreamServerInterceptor(cfg.APIKey)),
	)
	flightv1.RegisterFlightServiceServer(server, flight.NewServer(service))
	reflection.Register(server)

	go func() {
		log.Printf("flight service listening on %s", cfg.GRPCAddr)
		if err := server.Serve(listener); err != nil {
			log.Printf("grpc serve: %v", err)
		}
	}()

	<-ctx.Done()
	server.GracefulStop()
	log.Println("flight service stopped")
}

func migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(flightmigrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}
