package bleephub

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // SQLite driver — pure Go, no CGO
)

// Phase 153 (P153.12) — SQLite persistence for bleephub state.
//
// Strategy: KV-style table `kv(bucket, key, value)` where value is the
// JSON-encoded entity. Memory-first design — on startup, persistence layer
// loads every row of each bucket into the corresponding in-memory map, then
// every Create/Update/Delete writes through to disk. Reads are always served
// from the in-memory map (RWMutex-protected).
//
// Gated on `BLEEPHUB_PERSIST=true`. The on-disk path defaults to
// `${BLEEPHUB_DATA_DIR}/bleephub.db` or `./bleephub.db` if the env is unset.
//
// Fail-loud invariant (BUG-985/986 pattern): if the operator requested
// persistence and the DB can't be opened, server startup `log.Fatalf`s
// instead of silently falling back to in-memory.
//
// Git storage (go-git in-memory) is NOT persisted in this phase —
// switching to `filesystem.Storage` is a separate refactor.

type Persistence struct {
	db *sql.DB
	mu sync.Mutex // serialises writes (sqlite WAL handles concurrent reads fine)
}

// NewPersistence opens (or creates) the bleephub SQLite database. Returns
// nil + nil if persistence is disabled (BLEEPHUB_PERSIST != "true").
func NewPersistence() (*Persistence, error) {
	if os.Getenv("BLEEPHUB_PERSIST") != "true" {
		return nil, nil //nolint:nilnil // intentional: nil persistence = disabled
	}

	dataDir := os.Getenv("BLEEPHUB_DATA_DIR")
	if dataDir == "" {
		dataDir = "."
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	dbPath := filepath.Join(dataDir, "bleephub.db")

	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}

	// Single KV table. bucket+key form the composite primary key; value is JSON.
	const schema = `
CREATE TABLE IF NOT EXISTS kv (
	bucket TEXT NOT NULL,
	key    TEXT NOT NULL,
	value  BLOB NOT NULL,
	PRIMARY KEY (bucket, key)
);
CREATE TABLE IF NOT EXISTS counters (
	name  TEXT NOT NULL PRIMARY KEY,
	value INTEGER NOT NULL
);
`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Persistence{db: db}, nil
}

// MustNewPersistence is NewPersistence with the BUG-985 fail-loud behaviour:
// persistence requested + open failure → log.Fatalf.
func MustNewPersistence() *Persistence {
	p, err := NewPersistence()
	if err != nil {
		log.Fatalf("BLEEPHUB_PERSIST=true requested but persistence failed: %v", err)
	}
	return p
}

// Put writes/replaces a JSON-encoded entity under (bucket, key).
func (p *Persistence) Put(bucket, key string, v interface{}) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s/%s: %w", bucket, key, err)
	}
	_, err = p.db.Exec(`INSERT INTO kv (bucket, key, value) VALUES (?, ?, ?)
		ON CONFLICT(bucket, key) DO UPDATE SET value = excluded.value`,
		bucket, key, raw)
	return err
}

// Delete removes (bucket, key). Returns nil if the row didn't exist.
func (p *Persistence) Delete(bucket, key string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(`DELETE FROM kv WHERE bucket = ? AND key = ?`, bucket, key)
	return err
}

// List returns every (key, raw) pair in a bucket, suitable for boot-time load.
func (p *Persistence) List(bucket string) (map[string][]byte, error) {
	if p == nil {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rows, err := p.db.Query(`SELECT key, value FROM kv WHERE bucket = ?`, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	out := map[string][]byte{}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// GetCounter returns the named counter (atomic increments via NextCounter).
func (p *Persistence) GetCounter(name string) (int64, error) {
	if p == nil {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var v int64
	err := p.db.QueryRow(`SELECT value FROM counters WHERE name = ?`, name).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

// SetCounter writes a counter value (used during boot to seed Next*ID from
// `max(id) + 1` after loading rows from kv).
func (p *Persistence) SetCounter(name string, value int64) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(`INSERT INTO counters (name, value) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value`, name, value)
	return err
}

// Close flushes + closes the underlying connection.
func (p *Persistence) Close() error {
	if p == nil {
		return nil
	}
	return p.db.Close()
}
