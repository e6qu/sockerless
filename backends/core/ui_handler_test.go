package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":       {Data: []byte(`<html><div id="root"></div></html>`)},
		"assets/main.js":   {Data: []byte(`console.log("app")`)},
		"assets/style.css": {Data: []byte(`body { margin: 0 }`)},
	}

	handler := SPAHandler(fsys, "/ui/")

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "serves index.html at root",
			path:       "/ui/",
			wantStatus: http.StatusOK,
			wantBody:   `<div id="root">`,
		},
		{
			name:       "serves static JS asset",
			path:       "/ui/assets/main.js",
			wantStatus: http.StatusOK,
			wantBody:   `console.log("app")`,
		},
		{
			name:       "serves static CSS asset",
			path:       "/ui/assets/style.css",
			wantStatus: http.StatusOK,
			wantBody:   `body { margin: 0 }`,
		},
		{
			name:       "falls back to index.html for unknown path",
			path:       "/ui/containers",
			wantStatus: http.StatusOK,
			wantBody:   `<div id="root">`,
		},
		{
			name:       "falls back to index.html for nested unknown path",
			path:       "/ui/some/deep/route",
			wantStatus: http.StatusOK,
			wantBody:   `<div id="root">`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			body, _ := io.ReadAll(rec.Body)
			if got := string(body); len(tt.wantBody) > 0 {
				if !contains(got, tt.wantBody) {
					t.Errorf("body = %q, want to contain %q", got, tt.wantBody)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
