package assets

import (
	"bytes"
	"crypto/md5"
	"embed"
	"fmt"
	"path/filepath"
	"sync"
)

type Bundler struct {
	staticFS embed.FS
	once     sync.Once

	cssBundle []byte
	jsBundle  []byte
	cssHash   string
	jsHash    string

	potatoCSSBundle []byte
	potatoJSBundle  []byte
	potatoCSSHash   string
	potatoJSHash    string
}

var (
	cssFiles = []string{"main.css"}
	jsFiles  = []string{
		"app.js",
		"live.js",
		"music-player.js",
		"music-recents.js",
		"bike-map.js",
		"places-map.js",
		"greetings.js",
	}
	potatoCSSFiles = []string{"main.css"}
	potatoJSFiles  = []string{"app.js", "live.js", "music-recents.js"}
)

func NewBundler(staticFS embed.FS) *Bundler {
	return &Bundler{staticFS: staticFS}
}

func (b *Bundler) Build() {
	b.once.Do(func() {
		b.cssBundle, b.cssHash = b.bundle("static/css", cssFiles)
		b.jsBundle, b.jsHash = b.bundle("static/js", jsFiles)
		b.potatoCSSBundle, b.potatoCSSHash = b.bundle("static/css", potatoCSSFiles)
		b.potatoJSBundle, b.potatoJSHash = b.bundle("static/js", potatoJSFiles)
	})
}

func (b *Bundler) bundle(dir string, files []string) ([]byte, string) {
	var buf bytes.Buffer
	buf.WriteString("/* Bundled Assets */\n/* Bundled by: me! :3 */\n")

	for _, name := range files {
		data, err := b.staticFS.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("\n/* === %s === */\n", name))
		buf.Write(data)
		buf.WriteString("\n")
	}

	hash := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))[:8]
	return buf.Bytes(), hash
}

func (b *Bundler) CSSBundle() ([]byte, string)       { return b.cssBundle, b.cssHash }
func (b *Bundler) JSBundle() ([]byte, string)        { return b.jsBundle, b.jsHash }
func (b *Bundler) PotatoCSSBundle() ([]byte, string) { return b.potatoCSSBundle, b.potatoCSSHash }
func (b *Bundler) PotatoJSBundle() ([]byte, string)  { return b.potatoJSBundle, b.potatoJSHash }
