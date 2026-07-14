//go:build dqliteintegration

package bleephub

import (
	"os"
	"testing"
)

// TestDqliteRemotePersistence exercises the production dqlite transport
// against a live quorum. The Docker integration harness supplies the stable
// member addresses through BLEEPHUB_DQLITE_SERVERS.
func TestDqliteRemotePersistence(t *testing.T) {
	servers := os.Getenv("BLEEPHUB_DQLITE_SERVERS")
	if servers == "" {
		t.Fatal("BLEEPHUB_DQLITE_SERVERS is required for the dqlite integration test")
	}
	persistence, err := openDqlite(servers)
	if err != nil {
		t.Fatalf("open dqlite: %v", err)
	}
	t.Cleanup(func() { _ = persistence.db.Close() })
	if _, err := persistence.db.Exec(sqliteDialect.schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := persistence.Put("dqlite-test", "record", map[string]string{"value": "replicated"}); err != nil {
		t.Fatalf("write replicated record: %v", err)
	}
	entries, err := persistence.List("dqlite-test")
	if err != nil {
		t.Fatalf("read replicated record: %v", err)
	}
	if got := string(entries["record"]); got != `{"value":"replicated"}` {
		t.Fatalf("replicated record = %s", got)
	}
}
