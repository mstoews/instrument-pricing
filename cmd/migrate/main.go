package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	migrations "pricing-platform/db"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// Prefer a direct (non-pooled) URL for DDL; on Neon this is the host without "-pooler".
	dsn := os.Getenv("MIGRATE_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	c, e := pgx.Connect(ctx, dsn)
	if e != nil {
		log.Fatal(e)
	}
	defer c.Close(ctx)
	if e = run(ctx, c); e != nil {
		log.Fatal(e)
	}
}
func run(ctx context.Context, c *pgx.Conn) error {
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(715231)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migration (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	files, e := migrations.Files.ReadDir("migrations")
	if e != nil {
		return e
	}
	for _, f := range files {
		body, e := migrations.Files.ReadFile("migrations/" + f.Name())
		if e != nil {
			return e
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(body))
		var old string
		e = tx.QueryRow(ctx, "SELECT checksum FROM schema_migration WHERE name=$1", f.Name()).Scan(&old)
		if e == nil {
			if old != sum {
				return fmt.Errorf("migration checksum changed: %s", f.Name())
			}
			continue
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if _, e = tx.Exec(ctx, string(body)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migration(name,checksum) VALUES($1,$2)", f.Name(), sum); e != nil {
			return e
		}
		log.Printf("applied %s", f.Name())
	}
	return tx.Commit(ctx)
}
