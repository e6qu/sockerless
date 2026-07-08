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

type dbDialect struct {
	name      string
	schema    string // DDL to create tables
	putSQL    string // INSERT … ON CONFLICT upsert
	deleteSQL string
	listSQL   string
	getSQL    string
	setSQL    string
}

var (
	sqliteDialect = dbDialect{
		name: "sqlite",
		schema: `
CREATE TABLE IF NOT EXISTS kv (
	bucket TEXT NOT NULL,
	key    TEXT NOT NULL,
	value  BLOB NOT NULL,
	PRIMARY KEY (bucket, key)
);
CREATE TABLE IF NOT EXISTS counters (
	name  TEXT NOT NULL PRIMARY KEY,
	value INTEGER NOT NULL
);`,
		putSQL:    `INSERT INTO kv (bucket, key, value) VALUES (?, ?, ?) ON CONFLICT(bucket, key) DO UPDATE SET value = excluded.value`,
		deleteSQL: `DELETE FROM kv WHERE bucket = ? AND key = ?`,
		listSQL:   `SELECT key, value FROM kv WHERE bucket = ?`,
		getSQL:    `SELECT value FROM counters WHERE name = ?`,
		setSQL:    `INSERT INTO counters (name, value) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET value = excluded.value`,
	}
)

type Persistence struct {
	db      *sql.DB
	dialect dbDialect
	mu      sync.Mutex
}

func openSQLite(dataDir string) (*sql.DB, error) {
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
	return db, nil
}

func NewPersistence() (*Persistence, error) {
	if os.Getenv("BLEEPHUB_DATABASE_URL") != "" {
		return nil, fmt.Errorf("BLEEPHUB_DATABASE_URL is no longer supported; bleephub stores its own state in SQLite via BLEEPHUB_PERSIST=true and BLEEPHUB_DATA_DIR")
	}

	if os.Getenv("BLEEPHUB_PERSIST") != "true" {
		return nil, nil //nolint:nilnil // intentional: nil persistence = disabled
	}

	dataDir := os.Getenv("BLEEPHUB_DATA_DIR")
	if dataDir == "" {
		dataDir = "."
	}
	db, err := openSQLite(dataDir)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(sqliteDialect.schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	return &Persistence{db: db, dialect: sqliteDialect}, nil
}

func MustNewPersistence() *Persistence {
	p, err := NewPersistence()
	if err != nil {
		log.Fatalf("bleephub persistence configuration failed: %v", err)
	}
	return p
}

func (p *Persistence) MustPut(bucket, key string, v interface{}) {
	if err := p.Put(bucket, key, v); err != nil {
		log.Fatalf("bleephub persistence write %s/%s failed: %v", bucket, key, err)
	}
}

func (p *Persistence) MustDelete(bucket, key string) {
	if err := p.Delete(bucket, key); err != nil {
		log.Fatalf("bleephub persistence delete %s/%s failed: %v", bucket, key, err)
	}
}

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
	_, err = p.db.Exec(p.dialect.putSQL, bucket, key, raw)
	return err
}

func (p *Persistence) Delete(bucket, key string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(p.dialect.deleteSQL, bucket, key)
	return err
}

func (p *Persistence) List(bucket string) (map[string][]byte, error) {
	if p == nil {
		return nil, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rows, err := p.db.Query(p.dialect.listSQL, bucket)
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

func (p *Persistence) GetCounter(name string) (int64, error) {
	if p == nil {
		return 0, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var v int64
	err := p.db.QueryRow(p.dialect.getSQL, name).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

func (p *Persistence) SetCounter(name string, value int64) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(p.dialect.setSQL, name, value)
	return err
}

func (p *Persistence) Close() error {
	if p == nil {
		return nil
	}
	return p.db.Close()
}
