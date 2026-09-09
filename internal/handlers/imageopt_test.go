package handlers

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int) int64 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// A gradient compresses less than flat colour, so the fixture is a
	// realistic size rather than a few hundred bytes.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: uint8((x * y) % 256), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	st, _ := os.Stat(path)
	return st.Size()
}

func dimsOf(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestMaxDimFor(t *testing.T) {
	cases := map[string]int{
		"uploads/services": maxIconDim,
		"uploads/social":   maxIconDim,
		"uploads/icons":    maxIconDim,
		"uploads/meme":     maxPhotoDim,
		"uploads":          maxPhotoDim,
		"":                 maxPhotoDim,
	}
	for dir, want := range cases {
		if got := maxDimFor(dir); got != want {
			t.Errorf("maxDimFor(%q) = %d, want %d", dir, got, want)
		}
	}
}

func TestOptimizeImageFile_DownscalesOversized(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.png")
	before := writePNG(t, p, 1200, 1096) // mirrors the 435 KiB offender

	changed, err := optimizeImageFile(p, 256)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected the file to be rewritten")
	}

	w, h := dimsOf(t, p)
	if w > 256 || h > 256 {
		t.Errorf("still oversized: %dx%d", w, h)
	}
	// Aspect ratio preserved: 1200x1096 -> longest side 256
	if w != 256 {
		t.Errorf("expected width 256 (longest side), got %d", w)
	}
	st, _ := os.Stat(p)
	if st.Size() >= before {
		t.Errorf("file did not shrink: %d -> %d", before, st.Size())
	}
	t.Logf("%d -> %d bytes (%dx%d)", before, st.Size(), w, h)
}

func TestOptimizeImageFile_LeavesSmallImagesAlone(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.png")
	before := writePNG(t, p, 100, 80)

	changed, err := optimizeImageFile(p, 256)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("small image should not be rewritten")
	}
	w, h := dimsOf(t, p)
	if w != 100 || h != 80 {
		t.Errorf("dimensions changed: %dx%d", w, h)
	}
	st, _ := os.Stat(p)
	if st.Size() != before {
		t.Errorf("size changed: %d -> %d", before, st.Size())
	}
}

func TestOptimizeImageFile_SkipsNonRaster(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.svg", "a.gif", "a.mp4", "a.webp", "a.ico"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("not a real image"), 0644); err != nil {
			t.Fatal(err)
		}
		changed, err := optimizeImageFile(p, 256)
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		if changed {
			t.Errorf("%s: should have been skipped", name)
		}
	}
}
