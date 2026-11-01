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
	for _, name := range []string{"BubbleTea", "chromium", "DRF", "ffmpeg", "Fyne", "Jupyter", "OSM", "OverpassTurbo", "WebAssembly", "OpenGL", "PlatformIO", "ESP8266", "Raspberry Pi", "SQLite", "NumPy", "Pandas", "OpenCV", "WebRTC"} {
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

func TestDistinctRelatedTechnologyIcons(t *testing.T) {
	if got := techIconURL(map[string]interface{}{"name": "pgvector", "icon": "postgresql"}); got != "/static/icons/pgvector.svg" {
		t.Fatalf("pgvector legacy icon = %s", got)
	}
	if got := techIconURL(map[string]interface{}{"name": "Docker Compose", "icon": "docker"}); got != "/static/icons/docker-compose.svg" {
		t.Fatalf("compose legacy icon = %s", got)
	}
	if got := techIconURL(map[string]interface{}{"name": "DRF", "icon": "django"}); got != "/static/icons/drf-badge.svg" {
		t.Fatalf("DRF legacy icon = %s", got)
	}
	if got := techIconURL(map[string]interface{}{"name": "pgvector", "iconPath": "/custom.svg", "icon": "postgresql"}); got != "/custom.svg" {
		t.Fatalf("custom icon overridden: %s", got)
	}
	if got := lookupTechIcon(nil, "DRF"); got == "/static/icons/django.svg" || got == "" {
		t.Fatalf("DRF should have a distinct icon: %s", got)
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

func TestProjectsSortedByUpdatedDate(t *testing.T) {
	st := storage.New(t.TempDir())
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Wait)
	cfg := st.GetPluginConfig("projects")
	cfg.Settings["projects"] = []interface{}{
		map[string]interface{}{"name": "Old", "updated": "2024-01-01"},
		map[string]interface{}{"name": "Undated"},
		map[string]interface{}{"name": "Newest", "updated": "2026-11-01"},
		map[string]interface{}{"name": "Middle", "updated": "2025-06-01"},
	}
	if err := st.SetPluginConfig("projects", cfg); err != nil {
		t.Fatal(err)
	}
	html, err := NewProjectsPlugin(st, nil).Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	last := -1
	for _, name := range []string{"Newest", "Middle", "Old", "Undated"} {
		pos := strings.Index(html, `class="project-title">`+name+`</h3>`)
		if pos <= last {
			t.Fatalf("%s is missing or out of order", name)
		}
		last = pos
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
