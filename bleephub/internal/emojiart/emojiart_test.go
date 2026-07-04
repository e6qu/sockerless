package emojiart

import (
	"bytes"
	"image/png"
	"testing"
)

// TestBadgePNGDeterministic verifies the generator is a pure function of
// the emoji name: repeated calls return identical bytes, so the committed
// assets in emoji_assets.tar.gz are reproducible.
func TestBadgePNGDeterministic(t *testing.T) {
	for _, name := range []string{"octocat", "shipit", "trollface", "rage1", "1st"} {
		first, err := BadgePNG(name)
		if err != nil {
			t.Fatalf("BadgePNG(%q): %v", name, err)
		}
		second, err := BadgePNG(name)
		if err != nil {
			t.Fatalf("BadgePNG(%q) second call: %v", name, err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("BadgePNG(%q) is not deterministic: %d vs %d bytes differ", name, len(first), len(second))
		}
		img, err := png.Decode(bytes.NewReader(first))
		if err != nil {
			t.Fatalf("BadgePNG(%q) output is not a valid PNG: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() != Size || b.Dy() != Size {
			t.Fatalf("BadgePNG(%q) is %dx%d, want %dx%d", name, b.Dx(), b.Dy(), Size, Size)
		}
	}
}

// TestBadgePNGDistinctNames guards against the artwork degenerating into a
// single shared image: names with different initials or hash buckets must
// render differently.
func TestBadgePNGDistinctNames(t *testing.T) {
	octocat, err := BadgePNG("octocat")
	if err != nil {
		t.Fatal(err)
	}
	shipit, err := BadgePNG("shipit")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(octocat, shipit) {
		t.Fatal("octocat and shipit render identical badges")
	}
}

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{
		"octocat":     "OC",
		"shipit":      "SH",
		"rage1":       "RA",
		"feelsgood":   "FE",
		"hurtrealbad": "HU",
	} {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBadgePNGEmptyName(t *testing.T) {
	if _, err := BadgePNG(""); err == nil {
		t.Fatal("BadgePNG(\"\") succeeded, want error")
	}
}
