package bleephub

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/canonical/go-dqlite/v3/client"
	"github.com/canonical/go-dqlite/v3/driver"
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

type persistencePut struct {
	bucket string
	key    string
	value  interface{}
}

// PutBatch commits related records in one SQLite transaction. Callers update
// their in-memory indexes only after this returns successfully.
func (p *Persistence) PutBatch(entries ...persistencePut) error {
	if p == nil {
		return nil
	}
	type encodedPut struct {
		bucket string
		key    string
		raw    []byte
	}
	encoded := make([]encodedPut, 0, len(entries))
	for _, entry := range entries {
		raw, err := json.Marshal(entry.value)
		if err != nil {
			return fmt.Errorf("marshal %s/%s: %w", entry.bucket, entry.key, err)
		}
		encoded = append(encoded, encodedPut{bucket: entry.bucket, key: entry.key, raw: raw})
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	for _, entry := range encoded {
		if _, err := tx.Exec(p.dialect.putSQL, entry.bucket, entry.key, entry.raw); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (p *Persistence) DeleteBatch(entries ...persistencePut) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := tx.Exec(p.dialect.deleteSQL, entry.bucket, entry.key); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
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

// openDqlite connects to the durable dqlite quorum using its stable private
// addresses. The dqlite driver discovers the current leader from this seed
// set and refreshes its membership knowledge from the quorum itself.
func openDqlite(addresses string) (*sql.DB, error) {
	store := client.NewInmemNodeStore()
	servers := make([]client.NodeInfo, 0, 3)
	for _, address := range strings.Split(addresses, ",") {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		servers = append(servers, client.NodeInfo{Address: address})
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("BLEEPHUB_DQLITE_SERVERS must contain at least one dqlite server address")
	}
	if err := store.Set(context.Background(), servers); err != nil {
		return nil, fmt.Errorf("configure dqlite server set: %w", err)
	}

	dqliteDriver, err := driver.New(store,
		driver.WithDialFunc(dqliteHTTPDial),
		driver.WithAttemptTimeout(5*time.Second),
		driver.WithConnectionBackoffFactor(100*time.Millisecond),
		driver.WithConnectionBackoffCap(time.Second),
		driver.WithRetryLimit(12),
	)
	if err != nil {
		return nil, fmt.Errorf("create dqlite driver: %w", err)
	}
	connector, err := dqliteDriver.OpenConnector("bleephub")
	if err != nil {
		return nil, fmt.Errorf("open dqlite connector: %w", err)
	}
	db := sql.OpenDB(connector)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping dqlite: %w", err)
	}
	return db, nil
}

// dqliteHTTPDial opens the dqlite HTTP-upgrade transport exposed by each
// stable network-load-balancer endpoint. The upgrade keeps the dqlite wire
// protocol private while allowing Amazon ECS tasks to retain stable advertised
// member addresses across replacement and scale-to-zero restarts.
func dqliteHTTPDial(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial dqlite %s: %w", address, err)
	}

	requestURL, err := url.Parse("http://" + address + "/dqlite")
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("build dqlite upgrade request: %w", err)
	}
	request := &http.Request{Method: http.MethodGet, URL: requestURL, Host: requestURL.Host, Header: make(http.Header)}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "dqlite")
	if err := request.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write dqlite upgrade request: %w", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read dqlite upgrade response: %w", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(response.Header.Get("Upgrade"), "dqlite") {
		_ = response.Body.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("dqlite endpoint %s did not accept protocol upgrade: %s", address, response.Status)
	}
	return conn, nil
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

	var (
		db  *sql.DB
		err error
	)
	if addresses := strings.TrimSpace(os.Getenv("BLEEPHUB_DQLITE_SERVERS")); addresses != "" {
		db, err = openDqlite(addresses)
	} else {
		db, err = openSQLite(dataDir)
	}
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(sqliteDialect.schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("persistence schema: %w", err)
	}
	dialect := sqliteDialect
	if os.Getenv("BLEEPHUB_DQLITE_SERVERS") != "" {
		dialect.name = "dqlite"
	}
	return &Persistence{db: db, dialect: dialect}, nil
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
