package simulator

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

// SQLiteStore is a persistent implementation of Store backed by SQLite.
// Each instance maps to a single table with (key TEXT, value BLOB) schema.
type SQLiteStore[T any] struct {
	db    *sql.DB
	table string
	mu    sync.Mutex // serialize writes (SQLite is single-writer)
}

// NewSQLiteStore creates a persistent store backed by a SQLite table.
// Creates the table if it doesn't exist.
func NewSQLiteStore[T any](db *sql.DB, table string) (*SQLiteStore[T], error) {
	_, err := db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %q (key TEXT PRIMARY KEY, value BLOB)`, table))
	if err != nil {
		return nil, fmt.Errorf("create table %s: %w", table, err)
	}
	return &SQLiteStore[T]{db: db, table: table}, nil
}

func (s *SQLiteStore[T]) Get(id string) (T, bool) {
	var data []byte
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT value FROM %q WHERE key = ?`, s.table), id).Scan(&data)
	if err != nil {
		var zero T
		return zero, false
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		var zero T
		return zero, false
	}
	return v, true
}

func (s *SQLiteStore[T]) Put(id string, item T) {
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO %q (key, value) VALUES (?, ?)`, s.table),
		id, data)
}

func (s *SQLiteStore[T]) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(
		fmt.Sprintf(`DELETE FROM %q WHERE key = ?`, s.table), id)
	if err != nil {
		return false
	}
	n, _ := result.RowsAffected()
	return n > 0
}

func (s *SQLiteStore[T]) List() []T {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT value FROM %q`, s.table))
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var result []T
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		result = append(result, v)
	}
	return result
}

func (s *SQLiteStore[T]) Filter(fn func(T) bool) []T {
	all := s.List()
	var result []T
	for _, v := range all {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

func (s *SQLiteStore[T]) Len() int {
	var count int
	_ = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, s.table)).Scan(&count)
	return count
}

func (s *SQLiteStore[T]) Update(id string, fn func(*T)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var data []byte
	err := s.db.QueryRow(
		fmt.Sprintf(`SELECT value FROM %q WHERE key = ?`, s.table), id).Scan(&data)
	if err != nil {
		return false
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return false
	}
	fn(&v)
	updated, err := json.Marshal(v)
	if err != nil {
		return false
	}
	_, _ = s.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO %q (key, value) VALUES (?, ?)`, s.table),
		id, updated)
	return true
}

// MakeStore returns a SQLiteStore if db is non-nil, or a MemoryStore otherwise.
func MakeStore[T any](db *sql.DB, table string) Store[T] {
	if db != nil {
		s, err := NewSQLiteStore[T](db, table)
		if err == nil {
			return s
		}
		// Fall back to memory on error
	}
	return NewStateStore[T]()
}
