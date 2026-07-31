package plugins

import (
	"html/template"
	"sync"
	"testing"
)

type recordingNotifier struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingNotifier) Notify(plugin string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, plugin)
}

func TestInvalidatePluginCacheNotifies(t *testing.T) {
	m := &Manager{renderedCache: map[string]template.HTML{}}
	n := &recordingNotifier{}
	m.SetLayoutNotifier(n)
	m.InvalidatePluginCache("music")
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.calls) != 1 || n.calls[0] != "music" {
		t.Fatalf("expected Notify(music), got %v", n.calls)
	}
}
