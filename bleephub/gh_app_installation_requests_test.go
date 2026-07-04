package bleephub

import (
	"net/http"
	"testing"
	"time"
)

// TestAppInstallationRequests exercises GET /app/installation-requests with
// real GitHub App JWT authentication. bleephub installations are created
// directly by their owners, so no pending requests exist and the list is
// empty; unauthenticated callers get 401.
func TestAppInstallationRequests(t *testing.T) {
	app := testServer.store.CreateApp(1, "Installation Requests App", "", nil, nil)
	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", testBaseURL+"/api/v3/app/installation-requests", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	requests := decodeJSONArray(t, resp)
	if len(requests) != 0 {
		t.Fatalf("expected no pending installation requests, got %d", len(requests))
	}

	// Without an app JWT the endpoint requires authentication.
	resp2, err := http.Get(testBaseURL + "/api/v3/app/installation-requests")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Fatalf("unauthenticated status = %d, want 401", resp2.StatusCode)
	}
}
