package plugins

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buckket/go-blurhash"
)

func TestProjectTechIcons(t *testing.T) {
	for _, name := range []string{"BubbleTea", "chromium", "DRF", "ffmpeg", "Fyne", "Jupyter", "OSM", "OverpassTurbo"} {
		icon := lookupTechIcon(nil, name)
		if icon == "" {
			t.Errorf("missing icon for %s", name)
			continue
		}
		if _, err := os.Stat(filepath.Join("../..", strings.TrimPrefix(icon, "/"))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := lookupTechIcon(map[string]string{"jupyter": "/custom.svg"}, "Jupyter"); got != "/custom.svg" {
		t.Errorf("configured icon should override fallback: %s", got)
	}
}

func TestProjectPlaceholder(t *testing.T) {
	hash, err := blurhash.Encode(2, 2, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(projectPlaceholder(hash)), "data:image/png;base64,") {
		t.Fatal("valid blurhash did not produce a placeholder")
	}
	if projectPlaceholder("invalid") != "" || projectPlaceholder("") != "" {
		t.Fatal("invalid blurhash should not produce a placeholder")
	}
}
