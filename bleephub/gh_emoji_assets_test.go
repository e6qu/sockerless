package bleephub

import (
	"bytes"
	"image/png"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/sockerless/bleephub/internal/emojiart"
)

// TestEmojiCatalogImagesAllServed walks the FULL catalog returned by
// GET /emojis and asserts every advertised image URL is actually served by
// this instance: 200, image/png, an immutable cache policy, and a valid
// 72×72 PNG. No sampling — an unserved catalog entry is a broken contract.
func TestEmojiCatalogImagesAllServed(t *testing.T) {
	resp := ghGet(t, "/api/v3/emojis", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("emojis status = %d", resp.StatusCode)
	}
	catalog := decodeJSON(t, resp)
	if len(catalog) < 1900 {
		t.Fatalf("emoji catalog has %d entries, want the full gemoji set (>= 1900)", len(catalog))
	}

	for name, v := range catalog {
		imageURL, ok := v.(string)
		if !ok {
			t.Fatalf("catalog[%q] = %v, want a string URL", name, v)
		}
		u, err := url.Parse(imageURL)
		if err != nil {
			t.Fatalf("catalog[%q] = %q: %v", name, imageURL, err)
		}
		if !strings.HasPrefix(imageURL, testBaseURL+"/images/icons/emoji/") {
			t.Fatalf("catalog[%q] = %q, want an instance-hosted /images/icons/emoji/ URL", name, imageURL)
		}

		img := ghGet(t, u.Path+"?"+u.RawQuery, "")
		body, _ := io.ReadAll(img.Body)
		img.Body.Close()
		if img.StatusCode != 200 {
			t.Fatalf("GET %s (emoji %q) = %d, want 200", u.Path, name, img.StatusCode)
		}
		if ct := img.Header.Get("Content-Type"); ct != "image/png" {
			t.Fatalf("GET %s (emoji %q) content-type = %q, want image/png", u.Path, name, ct)
		}
		if cc := img.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Fatalf("GET %s (emoji %q) cache-control = %q, want an immutable policy", u.Path, name, cc)
		}
		decoded, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatalf("GET %s (emoji %q): body is not a valid PNG: %v", u.Path, name, err)
		}
		if b := decoded.Bounds(); b.Dx() != 72 || b.Dy() != 72 {
			t.Fatalf("GET %s (emoji %q): image is %dx%d, want 72x72", u.Path, name, b.Dx(), b.Dy())
		}
	}
}

func TestEmojiImageUnknownPath404(t *testing.T) {
	for _, p := range []string{
		"/images/icons/emoji/unicode/ffffff.png",
		"/images/icons/emoji/notreal.png",
		"/images/icons/emoji/LICENSE-TWEMOJI",
		"/images/icons/emoji/unicode/",
	} {
		resp := ghGet(t, p, "")
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Errorf("GET %s = %d, want 404", p, resp.StatusCode)
		}
	}
}

// TestCustomEmojiArtworkMatchesGenerator pins the committed GitHub-custom
// emoji images (octocat, shipit, …) to the emojiart generator: for every
// custom entry in the catalog, the embedded asset must decode to exactly the
// pixels emojiart.BadgePNG produces for that name, keeping the committed
// archive reproducible from the in-repo generator.
func TestCustomEmojiArtworkMatchesGenerator(t *testing.T) {
	assets, err := loadEmojiAssets()
	if err != nil {
		t.Fatalf("load embedded emoji assets: %v", err)
	}
	custom := 0
	for _, line := range strings.Split(strings.TrimSpace(gemojiCatalog), "\n") {
		name, rawPath, found := strings.Cut(line, " ")
		if !found || strings.HasPrefix(rawPath, "unicode/") {
			continue
		}
		custom++
		embedded, ok := assets[strings.TrimSuffix(rawPath, "?v8")]
		if !ok {
			t.Fatalf("custom emoji %q missing from embedded assets", name)
		}
		generated, err := emojiart.BadgePNG(name)
		if err != nil {
			t.Fatalf("BadgePNG(%q): %v", name, err)
		}
		embeddedImg, err := png.Decode(bytes.NewReader(embedded))
		if err != nil {
			t.Fatalf("decode embedded %q: %v", name, err)
		}
		generatedImg, err := png.Decode(bytes.NewReader(generated))
		if err != nil {
			t.Fatalf("decode generated %q: %v", name, err)
		}
		if embeddedImg.Bounds() != generatedImg.Bounds() {
			t.Fatalf("custom emoji %q: embedded bounds %v != generated bounds %v",
				name, embeddedImg.Bounds(), generatedImg.Bounds())
		}
		b := embeddedImg.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if embeddedImg.At(x, y) != generatedImg.At(x, y) {
					t.Fatalf("custom emoji %q: pixel (%d,%d) differs between the embedded asset and the generator — regenerate emoji_assets.tar.gz", name, x, y)
				}
			}
		}
	}
	if custom == 0 {
		t.Fatal("catalog contains no custom emoji entries — catalog parse broken")
	}
}
