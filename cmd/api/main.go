package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"os"
	"os/signal"
	"pricing-platform/internal/repository"
	"pricing-platform/internal/service"
	"pricing-platform/internal/transport/rest"
	pricing "pricing-platform/pricing/src"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		log.Fatal(e)
	}
	// Neon suspends idle computes; recycle idle connections before the compute
	// drops them, and allow a cold start (a few seconds) on the first ping.
	cfg.MaxConnIdleTime = 4 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		log.Fatal(e)
	}
	defer pool.Close()
	ping, cancel := context.WithTimeout(ctx, 20*time.Second)
	e = pool.Ping(ping)
	cancel()
	if e != nil {
		log.Fatal(e)
	}
	addr := os.Getenv("HTTP_ADDR")
	if port := os.Getenv("PORT"); addr == "" && port != "" {
		addr = ":" + port // container platforms such as Cloud Run inject PORT
	}
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	repo := repository.New(pool)
	server := &http.Server{Addr: addr, Handler: rest.Handler(service.New(repo), service.NewPricer(pricing.Engine{}), service.NewSecurities(repo)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(c)
	}()
	log.Printf("API listening on %s", addr)
	if e = server.ListenAndServe(); !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
