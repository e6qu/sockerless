// Package emojiart generates bleephub's own artwork for the GitHub-custom
// emoji names in the emoji catalog (octocat, shipit, trollface, …).
//
// GitHub's artwork for these emoji is proprietary and not redistributable,
// so bleephub ships original images instead: a 72×72 rounded-square badge
// carrying the emoji name's initial letters on a background color derived
// from the name. Generation is fully deterministic — the same name always
// produces the same bytes — so the committed assets in emoji_assets.tar.gz
// are reproducible from this package alone.
package emojiart

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Size is the badge edge length in pixels, matching the 72×72 raster size
// GitHub-hosted emoji assets use.
const Size = 72

// cornerRadius is the rounded-square corner radius in pixels.
const cornerRadius = 14

// textScale is the integer nearest-neighbor upscale factor applied to the
// 7×13 bitmap font when rendering the initials.
const textScale = 4

// palette holds the badge background colors. The color for a name is
// palette[fnv1a32(name) % len(palette)] — stable across runs and platforms.
var palette = []color.NRGBA{
	{R: 0xB7, G: 0x1C, B: 0x1C, A: 0xFF}, // deep red
	{R: 0xAD, G: 0x14, B: 0x57, A: 0xFF}, // raspberry
	{R: 0x6A, G: 0x1B, B: 0x9A, A: 0xFF}, // violet
	{R: 0x45, G: 0x27, B: 0xA0, A: 0xFF}, // indigo
	{R: 0x28, G: 0x35, B: 0x93, A: 0xFF}, // royal blue
	{R: 0x15, G: 0x65, B: 0xC0, A: 0xFF}, // azure
	{R: 0x02, G: 0x77, B: 0xBD, A: 0xFF}, // cerulean
	{R: 0x00, G: 0x69, B: 0x5C, A: 0xFF}, // teal
	{R: 0x2E, G: 0x7D, B: 0x32, A: 0xFF}, // forest green
	{R: 0x55, G: 0x8B, B: 0x2F, A: 0xFF}, // olive
	{R: 0x8D, G: 0x6E, B: 0x63, A: 0xFF}, // taupe
	{R: 0xEF, G: 0x6C, B: 0x00, A: 0xFF}, // burnt orange
	{R: 0xD8, G: 0x43, B: 0x15, A: 0xFF}, // vermilion
	{R: 0x4E, G: 0x34, B: 0x2E, A: 0xFF}, // espresso
	{R: 0x37, G: 0x47, B: 0x4F, A: 0xFF}, // slate
	{R: 0x5D, G: 0x40, B: 0x37, A: 0xFF}, // umber
}

// BadgePNG renders the deterministic badge for a custom-emoji name and
// returns it PNG-encoded.
func BadgePNG(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("emojiart: empty emoji name")
	}
	img := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	bg := palette[nameHash(name)%uint32(len(palette))]
	fillRoundedSquare(img, bg)
	if err := drawInitials(img, initials(name)); err != nil {
		return nil, fmt.Errorf("emojiart: render %q: %w", name, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("emojiart: encode %q: %w", name, err)
	}
	return buf.Bytes(), nil
}

func nameHash(name string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}

// initials derives the badge letters from the emoji name: the first
// character of the first two underscore-separated words, or the first two
// characters of a single-word name.
func initials(name string) string {
	var words []string
	for _, w := range strings.Split(name, "_") {
		if w != "" {
			words = append(words, w)
		}
	}
	var letters string
	switch {
	case len(words) >= 2:
		letters = words[0][:1] + words[1][:1]
	case len(words) == 1 && len(words[0]) >= 2:
		letters = words[0][:2]
	case len(words) == 1:
		letters = words[0]
	default:
		letters = name[:1]
	}
	return strings.ToUpper(letters)
}

// fillRoundedSquare fills the whole image with c, leaving pixels outside the
// rounded-square outline fully transparent. The corner test is pure integer
// arithmetic so the mask is identical on every platform.
func fillRoundedSquare(img *image.NRGBA, c color.NRGBA) {
	const r = cornerRadius
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			// Distance from the nearest corner's circle center; pixels in a
			// corner square but outside its quarter circle stay transparent.
			cx, cy := -1, -1
			if x < r {
				cx = r - 1
			} else if x >= Size-r {
				cx = Size - r
			}
			if y < r {
				cy = r - 1
			} else if y >= Size-r {
				cy = Size - r
			}
			if cx >= 0 && cy >= 0 {
				dx, dy := x-cx, y-cy
				if dx*dx+dy*dy > r*r {
					continue
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
}

// drawInitials renders the letters with the fixed 7×13 bitmap font, scales
// them up by textScale with nearest-neighbor sampling, and centers the
// result on the badge.
func drawInitials(dst *image.NRGBA, letters string) error {
	face := basicfont.Face7x13
	w := len(letters) * face.Advance
	h := face.Height
	small := image.NewNRGBA(image.Rect(0, 0, w, h))
	d := font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}),
		Face: face,
		Dot:  fixed.P(0, face.Ascent),
	}
	d.DrawString(letters)

	scaledW, scaledH := w*textScale, h*textScale
	if scaledW > Size || scaledH > Size {
		return fmt.Errorf("initials %q do not fit the badge (%dx%d > %dx%d)", letters, scaledW, scaledH, Size, Size)
	}
	// The bitmap font has binary coverage, so scaled pixels are either fully
	// opaque letter pixels or untouched background.
	offX, offY := (Size-scaledW)/2, (Size-scaledH)/2
	for y := 0; y < scaledH; y++ {
		for x := 0; x < scaledW; x++ {
			if px := small.NRGBAAt(x/textScale, y/textScale); px.A != 0 {
				dst.SetNRGBA(offX+x, offY+y, px)
			}
		}
	}
	return nil
}
