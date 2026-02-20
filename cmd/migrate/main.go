package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var (
		dbURL = flag.String("db", os.Getenv("AIHUB_DATABASE_URL"), "Postgres connection string")
		dir   = flag.String("dir", "migrations", "Migrations directory")
	)
	flag.Parse()

	if *dbURL == "" {
		log.Fatal("missing -db or AIHUB_DATABASE_URL")
	}

	db, err := sql.Open("pgx", *dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := ensureMigrationsTable(db); err != nil {
		log.Fatalf("migrations table: %v", err)
	}

	files, err := listSQLFiles(*dir)
	if err != nil {
		log.Fatalf("list migrations: %v", err)
	}
	for _, p := range files {
		if err := applyMigrationFile(db, p); err != nil {
			log.Fatalf("apply %s: %v", p, err)
		}
	}

	log.Printf("migrations applied from %s", *dir)
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		create table if not exists schema_migrations (
			filename text primary key,
			applied_at timestamptz not null default now()
		)
	`)
	return err
}

func listSQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

func applyMigrationFile(db *sql.DB, path string) error {
	base := filepath.Base(path)

	var exists bool
	if err := db.QueryRow(`select exists(select 1 from schema_migrations where filename=$1)`, base).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sqlText := strings.TrimSpace(string(b))
	if sqlText == "" {
		return fmt.Errorf("empty migration")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(sqlText); err != nil {
		return err
	}
	if _, err := tx.Exec(`insert into schema_migrations (filename) values ($1)`, base); err != nil {
		return err
	}
	return tx.Commit()
}
