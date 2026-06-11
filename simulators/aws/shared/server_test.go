package simulator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// RegisterUI must not panic the mux when a simulated service already owns
// the API root: S3's ListBuckets registers "GET /{$}", and the root belongs
// to the API surface — the UI stays reachable at /ui/.
func TestRegisterUICoexistsWithAPIRoot(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := NewServer(Config{Provider: "aws", LogLevel: "disabled"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-root"))
	})
	ui := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>ui</html>")}}
	srv.RegisterUI(ui) // must not panic on the duplicate "GET /{$}"

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "api-root" {
		t.Fatalf("API root must keep serving: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>ui</html>" {
		t.Fatalf("UI must serve at /ui/: %d %q", rec.Code, rec.Body.String())
	}
}

// Without a service on the API root, RegisterUI's redirect applies.
func TestRegisterUIRedirectsBareRoot(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, err := NewServer(Config{Provider: "aws", LogLevel: "disabled"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.RegisterUI(fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ui")}})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/ui/" {
		t.Fatalf("bare root must redirect to /ui/: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
