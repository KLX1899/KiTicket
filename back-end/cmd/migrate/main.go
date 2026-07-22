// Command migrate applies immutable SQL migrations under an advisory lock.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	directory := flag.String("dir", "migrations", "directory containing *.up.sql files")
	seed := flag.String("seed", "", "optional seed SQL file to apply after migrations")
	flag.Parse()
	if err := run(*directory, *seed); err != nil {
		log.Printf("migration failed: %v", err)
		os.Exit(1)
	}
}

func run(directory, seedFile string) error {
	databaseURL := strings.TrimSpace(os.Getenv("POSTGRES_URL"))
	if databaseURL == "" {
		return errors.New("POSTGRES_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	defer func() { _ = connection.Close(context.Background()) }()
	if _, err := connection.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS migration;
		CREATE TABLE IF NOT EXISTS migration.schema_migrations (
			version text PRIMARY KEY,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(files)
	for _, file := range files {
		if err := apply(ctx, connection, file); err != nil {
			return err
		}
	}
	if seedFile != "" {
		content, err := os.ReadFile(seedFile)
		if err != nil {
			return fmt.Errorf("read seed: %w", err)
		}
		if _, err := connection.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply seed: %w", err)
		}
		log.Printf("applied seed %s", filepath.Base(seedFile))
	}
	return nil
}

func apply(ctx context.Context, connection *pgx.Conn, file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", file, err)
	}
	checksumBytes := sha256.Sum256(content)
	checksum := hex.EncodeToString(checksumBytes[:])
	version := filepath.Base(file)
	tx, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(482019260714)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	var savedChecksum string
	err = tx.QueryRow(ctx, `SELECT checksum FROM migration.schema_migrations WHERE version = $1`, version).Scan(&savedChecksum)
	switch {
	case err == nil && savedChecksum != checksum:
		return fmt.Errorf("applied migration %s checksum changed", version)
	case err == nil:
		return tx.Commit(ctx)
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("read migration ledger: %w", err)
	}
	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO migration.schema_migrations (version, checksum) VALUES ($1, $2)`, version, checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	log.Printf("applied migration %s", version)
	return nil
}
