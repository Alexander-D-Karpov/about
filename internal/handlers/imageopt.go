package handlers

import (
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// Upload size caps. Lighthouse flagged a 1200x1096 PNG served into a 32x29 box
// (435 KiB for ~9% of the page weight), so icon-ish directories get a tight cap
// while content images keep enough resolution for a full-width, 2x display.
const (
	maxIconDim  = 256
	maxPhotoDim = 1600

	jpegQuality = 85
)

// iconDirs are upload targets whose images are only ever rendered as small
// icons (32px or less on screen).
var iconDirs = []string{"services", "social", "icons"}

// maxDimFor returns the longest-side cap for an upload target directory.
func maxDimFor(targetDir string) int {
	clean := strings.ToLower(strings.Trim(filepath.ToSlash(targetDir), "/"))
	for _, d := range iconDirs {
		if clean == d || strings.HasSuffix(clean, "/"+d) {
			return maxIconDim
		}
	}
	return maxPhotoDim
}

// optimizeImageFile downscales path in place if its longest side exceeds
// maxDim. It reports whether the file was rewritten.
//
// Only JPEG and PNG are touched:
//   - GIF is skipped because re-encoding would flatten animation
//   - SVG is vector and already small
//   - WebP has no encoder in the standard library or x/image
//   - video formats are not images
//
// The rewrite goes through a temp file and is only committed if the result is
// actually smaller, so a pathological re-encode can never inflate a file.
func optimizeImageFile(path string, maxDim int) (bool, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png":
	default:
		return false, nil
	}

	src, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		// A corrupt or misnamed file is not fatal to the upload: keep the
		// bytes the user sent rather than failing their request.
		log.Printf("imageopt: decode %s failed, leaving as-is: %v", path, err)
		return false, nil
	}

	b := src.Bounds()
	if b.Dx() <= maxDim && b.Dy() <= maxDim {
		return false, nil
	}

	dst := imaging.Fit(src, maxDim, maxDim, imaging.Lanczos)

	tmp := path + ".opt"
	if err := encodeLike(tmp, path, dst); err != nil {
		os.Remove(tmp)
		return false, err
	}

	oldInfo, err := os.Stat(path)
	if err != nil {
		os.Remove(tmp)
		return false, err
	}
	newInfo, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return false, err
	}
	if newInfo.Size() >= oldInfo.Size() {
		os.Remove(tmp)
		return false, nil
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false, err
	}
	log.Printf("imageopt: %s %dx%d -> %dx%d, %d -> %d bytes",
		filepath.Base(path), b.Dx(), b.Dy(),
		dst.Bounds().Dx(), dst.Bounds().Dy(), oldInfo.Size(), newInfo.Size())
	return true, nil
}

// encodeLike writes img to dstPath using the format implied by srcPath's
// extension, so a .png stays a .png and its URL keeps working.
func encodeLike(dstPath, srcPath string, img image.Image) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(srcPath)) {
	case ".jpg", ".jpeg":
		return jpeg.Encode(f, img, &jpeg.Options{Quality: jpegQuality})
	default:
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		return enc.Encode(f, img)
	}
}
