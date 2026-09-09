// Command mediaopt downscales oversized images already sitting in the media
// directory. The upload handler caps new uploads, but files uploaded before
// that existed are still full-size - Lighthouse found a 1200x1096 PNG being
// served into a 32x29 icon box.
//
// Dry-run by default. Pass -apply to actually rewrite files.
//
//	go run ./cmd/mediaopt -root ./media
//	go run ./cmd/mediaopt -root ./media -apply
package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"

	_ "image/gif"
)

const jpegQuality = 85

type candidate struct {
	path    string
	w, h    int
	size    int64
	maxDim  int
	newW    int
	newH    int
	newSize int64
}

func main() {
	root := flag.String("root", "media", "media directory to scan")
	apply := flag.Bool("apply", false, "actually rewrite files (default is a dry run)")
	iconDim := flag.Int("icon-dim", 256, "longest side for icon directories (services, social, icons)")
	photoDim := flag.Int("photo-dim", 1600, "longest side for everything else")
	flag.Parse()

	var cands []candidate
	var scanned int

	err := filepath.WalkDir(*root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries shouldn't abort the whole walk
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".jpg", ".jpeg", ".png":
		default:
			return nil
		}
		scanned++

		maxDim := *photoDim
		lower := strings.ToLower(filepath.ToSlash(p))
		for _, dir := range []string{"/services/", "/social/", "/icons/"} {
			if strings.Contains(lower, dir) {
				maxDim = *iconDim
				break
			}
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			return nil
		}
		if cfg.Width <= maxDim && cfg.Height <= maxDim {
			return nil
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil
		}
		cands = append(cands, candidate{path: p, w: cfg.Width, h: cfg.Height, size: st.Size(), maxDim: maxDim})
		return nil
	})
	if err != nil {
		log.Fatalf("walk %s: %v", *root, err)
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].size > cands[j].size })

	var totalBefore, totalAfter int64
	for i := range cands {
		c := &cands[i]
		if *apply {
			if err := rewrite(c); err != nil {
				fmt.Printf("  FAIL  %s: %v\n", c.path, err)
				continue
			}
		} else {
			if err := estimate(c); err != nil {
				fmt.Printf("  FAIL  %s: %v\n", c.path, err)
				continue
			}
		}
		totalBefore += c.size
		totalAfter += c.newSize
		fmt.Printf("  %-58s %5dx%-5d -> %4dx%-4d  %8s -> %8s\n",
			trim(c.path, 58), c.w, c.h, c.newW, c.newH, human(c.size), human(c.newSize))
	}

	fmt.Printf("\nscanned %d raster files, %d oversized\n", scanned, len(cands))
	fmt.Printf("total %s -> %s  (saves %s)\n", human(totalBefore), human(totalAfter), human(totalBefore-totalAfter))
	if !*apply {
		fmt.Println("\nDRY RUN - nothing written. Re-run with -apply to rewrite these files.")
	}
}

// resizeTo produces the downscaled image and its encoded bytes.
func resizeTo(c *candidate) (image.Image, []byte, error) {
	src, err := imaging.Open(c.path, imaging.AutoOrientation(true))
	if err != nil {
		return nil, nil, err
	}
	dst := imaging.Fit(src, c.maxDim, c.maxDim, imaging.Lanczos)

	tmp, err := os.CreateTemp("", "mediaopt-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.Remove(tmp.Name())

	switch strings.ToLower(filepath.Ext(c.path)) {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(tmp, dst, &jpeg.Options{Quality: jpegQuality})
	default:
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		err = enc.Encode(tmp, dst)
	}
	if err != nil {
		tmp.Close()
		return nil, nil, err
	}
	tmp.Close()

	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return nil, nil, err
	}
	return dst, b, nil
}

func estimate(c *candidate) error {
	dst, b, err := resizeTo(c)
	if err != nil {
		return err
	}
	c.newW, c.newH = dst.Bounds().Dx(), dst.Bounds().Dy()
	c.newSize = int64(len(b))
	if c.newSize >= c.size {
		c.newSize = c.size // wouldn't be rewritten
	}
	return nil
}

func rewrite(c *candidate) error {
	dst, b, err := resizeTo(c)
	if err != nil {
		return err
	}
	c.newW, c.newH = dst.Bounds().Dx(), dst.Bounds().Dy()
	if int64(len(b)) >= c.size {
		c.newSize = c.size
		return nil // never inflate a file
	}
	// Write alongside then rename, so a crash can't truncate the original.
	tmp := c.path + ".opt"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		return err
	}
	c.newSize = int64(len(b))
	return nil
}

func human(n int64) string {
	switch {
	case n > 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n > 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n+1:]
}
