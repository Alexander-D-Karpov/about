package assets

import (
	"bytes"
	"crypto/md5"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Bundler struct {
	staticFS  embed.FS
	cssBundle []byte
	jsBundle  []byte
	cssHash   string
	jsHash    string
	once      sync.Once

	potatoCSSBundle []byte
	potatoJSBundle  []byte
	potatoCSSHash   string
	potatoJSHash    string
}

func NewBundler(staticFS embed.FS) *Bundler {
	return &Bundler{staticFS: staticFS}
}

func (b *Bundler) Build() {
	b.once.Do(func() {
		b.cssBundle, b.cssHash = b.bundleDir("static/css", ".css", []string{"main.css"}, []string{"potato.css", "admin.css"})
		b.jsBundle, b.jsHash = b.bundleDir("static/js", ".js", []string{"websocket.js", "windowManager.js"}, []string{"potato-windowManager.js", "admin.js"})

		b.potatoCSSBundle, b.potatoCSSHash = b.bundleDir("static/css", ".css", []string{"potato.css"}, []string{"main.css", "admin.css"})
		b.potatoJSBundle, b.potatoJSHash = b.bundleDir("static/js", ".js", []string{"websocket.js", "potato-windowManager.js"}, []string{"windowManager.js", "admin.js"})
	})
}

func writeHeader(buf bytes.Buffer) bytes.Buffer {
	buf.WriteString("/* Bundled Assets */\n")
	buf.WriteString("/* Bundled by: me! :3 */\n")
	return buf
}

func (b *Bundler) bundleDir(dir, ext string, priorityFirst, exclude []string) ([]byte, string) {
	var buf bytes.Buffer
	buf = writeHeader(buf)

	files, err := b.listFiles(dir, ext)
	if err != nil {
		return nil, ""
	}

	excludeMap := make(map[string]bool)
	for _, e := range exclude {
		excludeMap[e] = true
	}

	priorityMap := make(map[string]int)
	for i, p := range priorityFirst {
		priorityMap[p] = i
	}

	sort.Slice(files, func(i, j int) bool {
		pi, okI := priorityMap[files[i]]
		pj, okJ := priorityMap[files[j]]

		if okI && okJ {
			return pi < pj
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}

		isPluginI := strings.HasPrefix(files[i], "plugin-")
		isPluginJ := strings.HasPrefix(files[j], "plugin-")
		if isPluginI != isPluginJ {
			return !isPluginI
		}

		return files[i] < files[j]
	})

	for _, name := range files {
		if excludeMap[name] {
			continue
		}

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

func (b *Bundler) bundlePotato(dir, ext string, include []string) ([]byte, string) {
	var buf bytes.Buffer
	buf = writeHeader(buf)

	for _, name := range include {
		data, err := b.staticFS.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}

		buf.WriteString(fmt.Sprintf("\n/* === %s === */\n", name))
		buf.Write(data)
		buf.WriteString("\n")
	}

	files, _ := b.listFiles(dir, ext)
	includeMap := make(map[string]bool)
	for _, i := range include {
		includeMap[i] = true
	}

	for _, name := range files {
		if includeMap[name] {
			continue
		}

		if strings.Contains(name, "plugin-") && strings.HasPrefix(filepath.Dir(name), "plugins") {
			data, err := b.staticFS.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			buf.WriteString(fmt.Sprintf("\n/* === %s === */\n", name))
			buf.Write(data)
			buf.WriteString("\n")
		}
	}

	hash := fmt.Sprintf("%x", md5.Sum(buf.Bytes()))[:8]
	return buf.Bytes(), hash
}

func (b *Bundler) listFiles(dir, ext string) ([]string, error) {
	var files []string

	err := fs.WalkDir(b.staticFS, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) == ext {
			relPath, _ := filepath.Rel(dir, path)
			files = append(files, relPath)
		}

		return nil
	})

	return files, err
}

func (b *Bundler) CSSBundle() ([]byte, string)       { return b.cssBundle, b.cssHash }
func (b *Bundler) JSBundle() ([]byte, string)        { return b.jsBundle, b.jsHash }
func (b *Bundler) PotatoCSSBundle() ([]byte, string) { return b.potatoCSSBundle, b.potatoCSSHash }
func (b *Bundler) PotatoJSBundle() ([]byte, string)  { return b.potatoJSBundle, b.potatoJSHash }
