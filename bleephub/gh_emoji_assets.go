package bleephub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// emoji_assets.tar.gz holds every image the GET /emojis catalog advertises
// under <base>/images/icons/emoji/: Twemoji CC-BY 4.0 rasters for the
// unicode/<codepoints>.png paths and bleephub's own generated artwork for
// the GitHub-custom <name>.png paths (GitHub's originals are not
// redistributable). Regenerate with the emojigen tool after a catalog or
// Twemoji update; attribution rides inside the archive (LICENSE-TWEMOJI,
// ATTRIBUTION).
//
//go:generate go run ./internal/emojigen -catalog gemoji_catalog.txt -out emoji_assets.tar.gz

//go:embed emoji_assets.tar.gz
var emojiAssetsArchive []byte

// loadEmojiAssets unpacks the embedded archive once into a path → PNG map
// (only .png entries are served; the license/attribution files stay
// archive-internal).
var loadEmojiAssets = sync.OnceValues(func() (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(emojiAssetsArchive))
	if err != nil {
		return nil, fmt.Errorf("emoji assets archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	assets := make(map[string][]byte, 2048)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("emoji assets archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !strings.HasSuffix(hdr.Name, ".png") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("emoji assets archive %s: %w", hdr.Name, err)
		}
		assets[hdr.Name] = data
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("emoji assets archive contains no images")
	}
	return assets, nil
})

// handleGHEmojiImage serves the emoji images advertised by GET /emojis,
// matching GitHub Enterprise Server hosting the assets on the instance
// itself. The images are immutable (the catalog cache-busts with ?v8), so
// they carry a far-future immutable cache policy.
func (s *Server) handleGHEmojiImage(w http.ResponseWriter, r *http.Request) {
	assets, err := loadEmojiAssets()
	if err != nil {
		s.logger.Error().Err(err).Msg("embedded emoji assets are unreadable")
		http.Error(w, "emoji assets unavailable", http.StatusInternalServerError)
		return
	}
	data, ok := assets[r.PathValue("path")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}
