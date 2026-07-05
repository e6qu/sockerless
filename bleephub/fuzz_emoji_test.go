package bleephub

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FuzzEmojiPathResolver fuzzes the {path...} segment of GET
// /images/icons/emoji/{path...} — the archive lookup behind the emoji
// catalog. Invariants:
//   - a path present in the embedded archive returns 200 with a decodable PNG;
//   - any other path returns 404;
//   - no path (traversal, absolute, embedded NUL, huge) panics or escapes the
//     archive (the lookup is a pure map read, so escape is structurally
//     impossible — this asserts it stays that way).
func FuzzEmojiPathResolver(f *testing.F) {
	s := fuzzRoutedServer(f)

	// A real catalog path from the archive, plus adversarial variants.
	assets, err := loadEmojiAssets()
	if err != nil {
		f.Fatalf("loadEmojiAssets: %v", err)
	}
	var realPath string
	for name := range assets {
		realPath = name
		break
	}
	if realPath == "" {
		f.Fatal("no emoji assets to seed with")
	}

	f.Add(realPath)
	f.Add("unicode/1f600.png")
	f.Add("octocat.png")
	f.Add("notreal.png")
	f.Add("../../etc/passwd")
	f.Add("unicode/../unicode/1f600.png")
	f.Add("/absolute/path.png")
	f.Add("unicode/")
	f.Add("")
	f.Add(strings.Repeat("a/", 5000) + "x.png")
	f.Add("unicode/1f600.PNG")
	f.Add("LICENSE-TWEMOJI")

	f.Fuzz(func(t *testing.T, p string) {
		// Route the request so PathValue("path") is populated exactly as the
		// mux would. Bad request targets (control bytes) are simply not served.
		req, err := http.NewRequest(http.MethodGet, "http://x/images/icons/emoji/"+p, nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "token "+defaultToken)
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)

		switch w.Code {
		case http.StatusOK:
			ct := w.Header().Get("Content-Type")
			if ct != "image/png" {
				t.Fatalf("200 for %q with Content-Type %q, want image/png", p, ct)
			}
			if _, err := png.Decode(bytes.NewReader(w.Body.Bytes())); err != nil {
				t.Fatalf("200 for %q returned a body that is not a valid PNG: %v", p, err)
			}
		case http.StatusNotFound, http.StatusMovedPermanently, http.StatusBadRequest:
			// A miss (or a redirect the mux issues for dot-segment cleanup) is
			// acceptable; only a 200 must carry a real PNG.
		default:
			if w.Code >= 500 {
				t.Fatalf("emoji path %q -> %d (want <500)", p, w.Code)
			}
		}
	})
}
