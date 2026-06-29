package ranking

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

const thumbW = 300
const thumbH = 300

func generateThumbnail(srcPath, thumbDir string) (string, error) {
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, _, err := image.DecodeConfig(f); err != nil {
		return "", fmt.Errorf("unsupported image format: %w", err)
	}

	img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		return "", err
	}

	thumb := imaging.Fit(img, thumbW, thumbH, imaging.Lanczos)

	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	thumbName := name + "_thumb.jpg"
	thumbPath := filepath.Join(thumbDir, thumbName)

	if err := imaging.Save(thumb, thumbPath); err != nil {
		return "", err
	}

	return thumbName, nil
}
