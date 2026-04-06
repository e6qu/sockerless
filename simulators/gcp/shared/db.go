package simulator

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// OpenDB opens a SQLite database at the given path with WAL mode enabled.
// Creates the directory and file if they don't exist.
func OpenDB(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "simulator.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// WAL mode for concurrent reads during writes
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}

	return db, nil
}
