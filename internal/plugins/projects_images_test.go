package plugins

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/buckket/go-blurhash"
)

func TestProjectTechIcons(t *testing.T) {
	for _, name := range []string{"BubbleTea", "chromium", "DRF", "ffmpeg", "Fyne", "Jupyter", "OSM", "OverpassTurbo", "WebAssembly", "PlatformIO", "ESP8266", "Raspberry Pi"} {
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

func TestProjectImageOriginalFallback(t *testing.T) {
	st := storage.New(t.TempDir())
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Wait)
	cfg := st.GetPluginConfig("projects")
	cfg.Settings["projects"] = []interface{}{
		map[string]interface{}{"name": "Original", "image": "/thumb.webp", "image_original": "/original.png"},
		map[string]interface{}{"name": "Fallback", "image": "/fallback.webp"},
	}
	if err := st.SetPluginConfig("projects", cfg); err != nil {
		t.Fatal(err)
	}
	html, err := NewProjectsPlugin(st, nil).Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-original="/original.png"`, `data-original="/fallback.webp"`, `src="/thumb.webp"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
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
