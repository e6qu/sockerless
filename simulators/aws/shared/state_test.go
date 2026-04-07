package simulator

import (
	"database/sql"
	"os"
	"testing"
)

type testItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func makeStores(t *testing.T) map[string]Store[testItem] {
	t.Helper()

	// SQLite store
	dir, err := os.MkdirTemp("", "sim-state-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	sqliteStore, err := NewSQLiteStore[testItem](db, "test_items")
	if err != nil {
		t.Fatal(err)
	}

	return map[string]Store[testItem]{
		"memory": NewStateStore[testItem](),
		"sqlite": sqliteStore,
	}
}

func TestStorePutGet(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("a", testItem{Name: "alpha", Value: 1})
			v, ok := store.Get("a")
			if !ok {
				t.Fatal("expected to find item")
			}
			if v.Name != "alpha" || v.Value != 1 {
				t.Errorf("got %+v", v)
			}
		})
	}
}

func TestStoreGetMissing(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			_, ok := store.Get("nonexistent")
			if ok {
				t.Error("expected not found")
			}
		})
	}
}

func TestStoreDelete(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("del", testItem{Name: "delete-me"})
			if !store.Delete("del") {
				t.Error("expected delete to return true")
			}
			if store.Delete("del") {
				t.Error("expected second delete to return false")
			}
			if _, ok := store.Get("del"); ok {
				t.Error("expected item gone after delete")
			}
		})
	}
}

func TestStoreList(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("x", testItem{Name: "x"})
			store.Put("y", testItem{Name: "y"})
			items := store.List()
			if len(items) != 2 {
				t.Errorf("expected 2 items, got %d", len(items))
			}
		})
	}
}

func TestStoreFilter(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("a", testItem{Name: "a", Value: 10})
			store.Put("b", testItem{Name: "b", Value: 20})
			store.Put("c", testItem{Name: "c", Value: 30})
			filtered := store.Filter(func(item testItem) bool { return item.Value > 15 })
			if len(filtered) != 2 {
				t.Errorf("expected 2 filtered items, got %d", len(filtered))
			}
		})
	}
}

func TestStoreLen(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			if store.Len() != 0 {
				t.Errorf("expected 0, got %d", store.Len())
			}
			store.Put("1", testItem{})
			store.Put("2", testItem{})
			if store.Len() != 2 {
				t.Errorf("expected 2, got %d", store.Len())
			}
		})
	}
}

func TestStoreUpdate(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("upd", testItem{Name: "original", Value: 1})
			if !store.Update("upd", func(item *testItem) {
				item.Value = 42
			}) {
				t.Error("expected update to return true")
			}
			v, _ := store.Get("upd")
			if v.Value != 42 {
				t.Errorf("expected 42, got %d", v.Value)
			}
		})
	}
}

func TestStoreUpdateMissing(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			if store.Update("nope", func(item *testItem) {}) {
				t.Error("expected update on missing key to return false")
			}
		})
	}
}

func TestStoreOverwrite(t *testing.T) {
	for name, store := range makeStores(t) {
		t.Run(name, func(t *testing.T) {
			store.Put("k", testItem{Name: "first", Value: 1})
			store.Put("k", testItem{Name: "second", Value: 2})
			v, _ := store.Get("k")
			if v.Name != "second" {
				t.Errorf("expected overwrite, got %q", v.Name)
			}
		})
	}
}

func TestMakeStoreMemory(t *testing.T) {
	s := MakeStore[testItem](nil, "unused")
	s.Put("a", testItem{Name: "a"})
	if s.Len() != 1 {
		t.Error("expected memory store to work")
	}
}

func TestMakeStoreSQLite(t *testing.T) {
	dir, _ := os.MkdirTemp("", "sim-make-*")
	defer os.RemoveAll(dir)
	db, _ := OpenDB(dir)
	defer db.Close()

	s := MakeStore[testItem](db, "make_test")
	s.Put("b", testItem{Name: "b"})
	if s.Len() != 1 {
		t.Error("expected sqlite store to work")
	}
}

func TestSQLitePersistence(t *testing.T) {
	dir, _ := os.MkdirTemp("", "sim-persist-*")
	defer os.RemoveAll(dir)

	// Write with first connection
	db1, _ := OpenDB(dir)
	s1 := MakeStore[testItem](db1, "persist_test")
	s1.Put("key1", testItem{Name: "persisted", Value: 99})
	db1.Close()

	// Read with second connection
	db2, _ := OpenDB(dir)
	defer db2.Close()
	s2 := MakeStore[testItem](db2, "persist_test")
	v, ok := s2.Get("key1")
	if !ok {
		t.Fatal("expected to find persisted item after reopen")
	}
	if v.Name != "persisted" || v.Value != 99 {
		t.Errorf("got %+v", v)
	}
}

func TestSQLiteStoreWithNilDB(t *testing.T) {
	s := MakeStore[testItem]((*sql.DB)(nil), "whatever")
	// Should fall back to memory
	s.Put("x", testItem{Name: "x"})
	if _, ok := s.Get("x"); !ok {
		t.Error("memory fallback should work with nil db")
	}
}
