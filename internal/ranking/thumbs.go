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

const thumbSize = 200

func generateThumbnail(srcPath, thumbDir string) (string, error) {
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return "", err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, format, err := image.DecodeConfig(f)
	if err != nil {
		return "", fmt.Errorf("unsupported image format: %w", err)
	}
	_ = format

	img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		return "", err
	}

	thumb := imaging.Fill(img, thumbSize, thumbSize, imaging.Center, imaging.Lanczos)

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
