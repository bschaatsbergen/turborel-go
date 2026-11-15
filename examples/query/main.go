// Query demonstrates a bound parameter using configuration from the environment.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/bschaatsbergen/turborel-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := query(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) // R-ERR-1: command exit status reports failure.
	}
}

func query(ctx context.Context) error {
	db, err := sql.Open("turborel", os.Getenv("TURBOREL_DSN"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }() // No transaction remains after the single autocommit query.
	var answer int
	if err := db.QueryRowContext(ctx, "VALUES (CAST(? AS INTEGER))", 42).Scan(&answer); err != nil {
		return err
	}
	fmt.Println(answer)
	return nil
}
