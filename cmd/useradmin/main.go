// Command useradmin manages accounts and API keys directly in the database:
//
//	useradmin user -email admin@example.com -role admin   (password: USERADMIN_PASSWORD or first stdin line)
//	useradmin api-key -email admin@example.com -name ingestion -scope ingest   (prints the key once)
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	"pricing-platform/internal/repository"
	"pricing-platform/internal/service"
	"strings"
	"time"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: useradmin user -email E -role user|admin | api-key -email E -name N -scope read|ingest")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	email := fs.String("email", "", "account email")
	role := fs.String("role", "user", "user or admin")
	name := fs.String("name", "", "API key name")
	scope := fs.String("scope", "read", "API key scope: read or ingest")
	_ = fs.Parse(os.Args[2:])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	a := service.NewAuth(repository.New(conn), service.AuthConfig{})
	switch os.Args[1] {
	case "user":
		password := os.Getenv("USERADMIN_PASSWORD")
		if password == "" {
			fmt.Fprint(os.Stderr, "password: ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			password = strings.TrimRight(line, "\r\n")
		}
		id, err := a.SetUser(ctx, *email, password, *role)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "user %s (%s) saved\n", *email, id)
	case "api-key":
		k, err := a.IssueAPIKey(ctx, *email, *name, *scope)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "API key %s (%s, %s) created; it is not shown again\n", k.ID, k.Name, k.Scope)
		fmt.Println(k.Key)
	default:
		usage()
	}
}
