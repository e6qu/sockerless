package bleephub

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistence_RoundTripAppsInstallationsTokensRepos(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	persistRoundTrip(t, func() (*Persistence, error) { return NewPersistence() })
}

func TestPersistence_PostgresRoundTrip(t *testing.T) {
	base := os.Getenv("BLEEPHUB_TEST_POSTGRES_URL")
	if base == "" {
		// CI provisions a postgres service and sets this; locally, run a
		// `postgres` container and export it to exercise the postgres backend.
		t.Skip("BLEEPHUB_TEST_POSTGRES_URL not set (set it to a base postgres DSN to run the postgres backend)")
	}

	// Create a unique throwaway database on the server so repeated/parallel runs
	// don't collide, then point BLEEPHUB_DATABASE_URL at it. (The Go test name is
	// a simple lowercase identifier — no further escaping needed.)
	dbName := "bleephub_test_" + strings.ToLower(strings.NewReplacer("/", "_", "-", "_").Replace(t.Name()))
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS "` + dbName + `"`); err != nil {
		t.Fatalf("drop pre-existing %q: %v", dbName, err)
	}
	if _, err := admin.Exec(`CREATE DATABASE "` + dbName + `"`); err != nil {
		t.Fatalf("create database %q: %v", dbName, err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP DATABASE IF EXISTS "` + dbName + `"`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse BLEEPHUB_TEST_POSTGRES_URL: %v", err)
	}
	u.Path = "/" + dbName
	t.Setenv("BLEEPHUB_DATABASE_URL", u.String())

	persistRoundTrip(t, func() (*Persistence, error) {
		p, err := NewPersistence()
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("expected persistence enabled")
		}
		return p, nil
	})
}

func persistRoundTrip(t *testing.T, open func() (*Persistence, error)) {
	t.Helper()

	p1, err := open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	user := st1.UsersByLogin["admin"]
	app := st1.CreateApp(user.ID, "Persist App", "desc", map[string]string{"issues": "write"}, []string{"push"})
	inst := st1.CreateInstallation(app.ID, "User", user.ID, user.Login, map[string]string{"issues": "write"}, nil)
	tok := st1.CreateInstallationToken(inst.ID, app.ID, map[string]string{"issues": "write"}, nil)
	st1.SuspendInstallation(inst.ID, user)
	oapp := st1.CreateOAuthApp(user.ID, "Persist OAuth", "", "", "")
	utsTok, _ := st1.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 60_000_000_000, true)
	repo := st1.CreateRepo(user, "persist-target", "", false)
	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	p2, err := open()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	if got := st2.UsersByLogin["admin"]; got == nil {
		t.Fatal("admin user did not persist")
	}

	got := st2.GetApp(app.ID)
	if got == nil {
		t.Fatalf("app %d did not persist", app.ID)
	}
	if got.Slug != app.Slug {
		t.Errorf("app slug round-trip: got %q want %q", got.Slug, app.Slug)
	}
	if got.Permissions["issues"] != "write" {
		t.Errorf("app permissions round-trip: got %v", got.Permissions)
	}

	if st2.GetAppBySlug(app.Slug) == nil {
		t.Error("AppsBySlug index missing after reload")
	}
	if st2.GetAppByClientID(app.ClientID) == nil {
		t.Error("AppsByClientID index missing after reload")
	}

	gotInst := st2.GetInstallation(inst.ID)
	if gotInst == nil {
		t.Fatal("installation did not persist")
	}
	if gotInst.SuspendedAt == nil {
		t.Error("installation SuspendedAt did not persist")
	}

	if gotTok, gotInstFromTok := st2.LookupInstallationToken(tok.Token); gotTok == nil || gotInstFromTok == nil {
		t.Error("installation token did not persist")
	}

	if got := st2.GetOAuthApp(oapp.ClientID); got == nil {
		t.Error("OAuth app did not persist")
	}

	if gotUts, _ := st2.LookupUserToServerToken(utsTok.Token); gotUts == nil {
		t.Error("user-to-server token did not persist")
	}

	if got := st2.GetRepo(user.Login, "persist-target"); got == nil {
		t.Fatal("repo did not persist")
	} else if got.ID != repo.ID {
		t.Errorf("repo ID round-trip: got %d want %d", got.ID, repo.ID)
	}
}

func TestPersistence_DisabledWhenEnvUnset(t *testing.T) {
	t.Setenv("BLEEPHUB_PERSIST", "")
	p, err := NewPersistence()
	if err != nil {
		t.Fatalf("NewPersistence: %v", err)
	}
	if p != nil {
		t.Error("expected nil persistence when BLEEPHUB_PERSIST unset")
	}
}

func TestPersistence_BadPathFailsLoud(t *testing.T) {
	// Point at a path that can't be created (under a regular file).
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", filepath.Join(blocker, "subdir"))

	if _, err := NewPersistence(); err == nil {
		t.Fatal("expected error when data dir cannot be created")
	}
}
