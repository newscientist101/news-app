package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"

	_ "modernc.org/sqlite"
)

//go:generate sqlc generate

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrationPattern matches files like "001-base.sql", "002-news-app.sql"
var migrationPattern = regexp.MustCompile(`^(\d{3})-.*\.sql$`)

// Open opens an sqlite database and prepares pragmas suitable for a small web app.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := configurePragmas(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func configurePragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=wal",
		"PRAGMA busy_timeout=1000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// RunMigrations executes database migrations in numeric order (NNN-*.sql).
func RunMigrations(db *sql.DB) error {
	migrations, err := listMigrationFiles()
	if err != nil {
		return err
	}

	executed, err := getExecutedMigrations(db)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		num := parseMigrationNumber(m)
		if executed[num] {
			continue
		}
		if err := executeMigration(db, m); err != nil {
			return fmt.Errorf("execute %s: %w", m, err)
		}
		slog.Info("db: applied migration", "file", m, "number", num)
	}
	return nil
}

// listMigrationFiles returns sorted migration filenames from the embedded FS.
func listMigrationFiles() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []string
	for _, e := range entries {
		if !e.IsDir() && migrationPattern.MatchString(e.Name()) {
			migrations = append(migrations, e.Name())
		}
	}
	sort.Strings(migrations)
	return migrations, nil
}

// getExecutedMigrations returns a set of migration numbers that have been run.
func getExecutedMigrations(db *sql.DB) (map[int]bool, error) {
	executed := make(map[int]bool)

	// Check if migrations table exists
	var exists int
	err := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&exists)
	if err == sql.ErrNoRows {
		slog.Info("db: migrations table not found; running all migrations")
		return executed, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check migrations table: %w", err)
	}

	// Load executed migration numbers
	rows, err := db.Query("SELECT migration_number FROM migrations")
	if err != nil {
		return nil, fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan migration number: %w", err)
		}
		executed[n] = true
	}
	return executed, rows.Err()
}

// parseMigrationNumber extracts the number from a migration filename.
// Assumes filename matches migrationPattern.
func parseMigrationNumber(filename string) int {
	match := migrationPattern.FindStringSubmatch(filename)
	if len(match) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(match[1])
	return n
}

// executeMigration reads and executes a single migration file.
func executeMigration(db *sql.DB, filename string) error {
	content, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}
