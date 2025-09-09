package plugins

import (
	"context"
	"html/template"
	"math/rand"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type MemePlugin struct {
	storage     *storage.Storage
	hub         *stream.Hub
	currentMeme *Meme
	lastUpdate  time.Time
	rng         *rand.Rand
}

type Meme struct {
	Text     string `json:"text"`
	Image    string `json:"image"`
	Type     string `json:"type"`
	Source   string `json:"source"`
	Category string `json:"category"`
}

func NewMemePlugin(storage *storage.Storage, hub *stream.Hub) *MemePlugin {
	source := rand.NewSource(time.Now().UnixNano())
	plugin := &MemePlugin{
		storage: storage,
		hub:     hub,
		rng:     rand.New(source),
	}

	plugin.selectRandomMeme()
	plugin.lastUpdate = time.Now()

	return plugin
}

func (p *MemePlugin) Name() string { return "meme" }

func (p *MemePlugin) Render(ctx context.Context) (string, error) {
	p.selectRandomMeme()

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	showMeme := p.getConfigBool(settings, "ui.showMeme", true)
	if !showMeme || p.currentMeme == nil {
		return "", nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Random Meme")

	tmpl := `
	<div class="meme-section section" id="meme-section">
		<div class="meme-header">
			<h3>{{.SectionTitle}}</h3>
		</div>
		
		<div class="meme-content">
			{{if eq .Meme.Type "image"}}
			<div class="meme-image">
				<img src="{{.Meme.Image}}" alt="{{.Meme.Text}}" loading="lazy">
				{{if .Meme.Text}}<p class="meme-caption">{{.Meme.Text}}</p>{{end}}
			</div>
			{{else if eq .Meme.Type "gif"}}
			<div class="meme-gif">
				<img src="{{.Meme.Image}}" alt="{{.Meme.Text}}" loading="lazy">
				{{if .Meme.Text}}<p class="meme-caption">{{.Meme.Text}}</p>{{end}}
			</div>
			{{else}}
			<div class="meme-text">
				<p class="meme-quote">{{.Meme.Text}}</p>
				{{if .Meme.Source}}<p class="meme-source">— {{.Meme.Source}}</p>{{end}}
			</div>
			{{end}}
		</div>
	</div>`

	data := struct {
		SectionTitle string
		Meme         *Meme
	}{
		SectionTitle: sectionTitle,
		Meme:         p.currentMeme,
	}

	t, err := template.New("meme").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *MemePlugin) UpdateData(ctx context.Context) error {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	autoRefresh := p.getConfigBool(settings, "ui.autoRefresh", false)
	refreshInterval := p.getConfigInt(settings, "ui.refreshInterval", 300)

	if autoRefresh && time.Since(p.lastUpdate) > time.Duration(refreshInterval)*time.Second {
		p.selectRandomMeme()
		p.broadcastNewMeme()
		p.lastUpdate = time.Now()
	}
	return nil
}

func (p *MemePlugin) selectRandomMeme() {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	memes, ok := settings["memes"].([]interface{})
	if !ok || len(memes) == 0 {
		memes = p.getDefaultMemes()
	}
	if len(memes) == 0 {
		return
	}

	memeIndex := p.rng.Intn(len(memes))
	memeData := memes[memeIndex]

	memeMap, ok := memeData.(map[string]interface{})
	if !ok {
		return
	}

	p.currentMeme = &Meme{
		Text:     p.getStringFromMap(memeMap, "text", ""),
		Image:    p.getStringFromMap(memeMap, "image", ""),
		Type:     p.getStringFromMap(memeMap, "type", "image"),
		Source:   p.getStringFromMap(memeMap, "source", ""),
		Category: p.getStringFromMap(memeMap, "category", "general"),
	}
}

func (p *MemePlugin) broadcastNewMeme() {
	if p.currentMeme == nil {
		return
	}
	p.hub.Broadcast("meme_update", map[string]interface{}{
		"meme": *p.currentMeme,
	})
}

func (p *MemePlugin) RefreshMeme() {
	p.selectRandomMeme()
	p.broadcastNewMeme()
	p.lastUpdate = time.Now()
}

func (p *MemePlugin) getDefaultMemes() []interface{} {
	return []interface{}{
		map[string]interface{}{"type": "image", "image": "/static/memes/test.webp", "text": "really cool", "category": "test"},
		map[string]interface{}{"type": "image", "image": "/static/memes/test2.jpg", "text": "that says a lot about our society", "category": "test"},
	}
}

func (p *MemePlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *MemePlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		return err
	}

	p.selectRandomMeme()

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})
	return nil
}

func (p *MemePlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return defaultValue
		}
		current = next
	}
	return defaultValue
}

func (p *MemePlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return defaultValue
		}
		current = next
	}
	return defaultValue
}

func (p *MemePlugin) getConfigInt(settings map[string]interface{}, key string, defaultValue int) int {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if v, ok := current[k].(float64); ok {
				return int(v)
			}
			if v, ok := current[k].(int); ok {
				return v
			}
			return defaultValue
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return defaultValue
		}
		current = next
	}
	return defaultValue
}

func (p *MemePlugin) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return defaultValue
}
