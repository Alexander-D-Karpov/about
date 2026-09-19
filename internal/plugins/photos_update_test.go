package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
)

// photosBackend fakes the photos API: a folders list plus per-folder photos,
// with a switch to start failing one folder's photo fetch mid-life.
type photosBackend struct {
	mu       sync.Mutex
	failFold map[int]bool
	inFlight int32
	maxPar   int32
}

func (b *photosBackend) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/folders", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(PhotosAPIFoldersResponse{Folders: []PhotosFolder{
			{ID: 1, Name: "one", Path: "one", PhotoCount: 3},
			{ID: 2, Name: "two", Path: "two", PhotoCount: 5},
			{ID: 3, Name: "three", Path: "three", PhotoCount: 4},
		}})
	})
	mux.HandleFunc("/api/photos", func(w http.ResponseWriter, r *http.Request) {
		// Hold every request open for a window so that parallel fetches are all
		// in flight at once; then record the peak. A serialized implementation
		// only ever has one open at a time, so maxPar stays 1.
		atomic.AddInt32(&b.inFlight, 1)
		time.Sleep(120 * time.Millisecond)
		peak := atomic.LoadInt32(&b.inFlight)
		for {
			old := atomic.LoadInt32(&b.maxPar)
			if peak <= old || atomic.CompareAndSwapInt32(&b.maxPar, old, peak) {
				break
			}
		}
		defer atomic.AddInt32(&b.inFlight, -1)

		var id int
		fmt.Sscanf(r.URL.Query().Get("folder_id"), "%d", &id)

		b.mu.Lock()
		fail := b.failFold[id]
		b.mu.Unlock()
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(PhotosAPIPhotosResponse{
			Total: 4,
			Photos: []PhotosPhoto{
				{ID: id*100 + 1, FolderID: id, Thumbnails: PhotosThumbnails{Small: fmt.Sprintf("/thumb/small/%d", id*100+1)}},
			},
		})
	})
	return mux
}

func newPhotosPluginForTest(t *testing.T, apiURL string) *PhotosPlugin {
	t.Helper()
	dir := t.TempDir()
	st := storage.New(dir)
	t.Cleanup(st.Wait)
	if err := st.Load(); err != nil {
		t.Fatalf("storage load: %v", err)
	}
	if err := st.SetPluginConfig("photos", &storage.PluginConfig{
		Enabled:  true,
		Settings: map[string]interface{}{"apiUrl": apiURL},
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	return NewPhotosPlugin(st, nil)
}

func photosByID(data *PhotosData) map[int]PhotosFolder {
	out := map[int]PhotosFolder{}
	for _, f := range data.Folders {
		out[f.ID] = f
	}
	return out
}

// A folder whose photo fetch fails on a refresh keeps the thumbnails it already
// had, instead of blanking to a placeholder — the reported bug was every folder
// but the first showing an empty card.
func TestPhotosPreserveThumbnailsOnFolderFailure(t *testing.T) {
	be := &photosBackend{failFold: map[int]bool{}}
	srv := httptest.NewServer(be.handler())
	defer srv.Close()

	p := newPhotosPluginForTest(t, srv.URL)

	if err := p.UpdateData(context.Background()); err != nil {
		t.Fatalf("first update: %v", err)
	}
	first := photosByID(p.data)
	for _, id := range []int{1, 2, 3} {
		if len(first[id].Photos) == 0 {
			t.Fatalf("folder %d had no photos after a healthy update", id)
		}
	}
	if be.maxPar < 2 {
		t.Fatalf("per-folder fetches never overlapped (maxPar=%d); they are serialized", be.maxPar)
	}

	// Folder 2 now fails; force a refresh past the throttle.
	be.mu.Lock()
	be.failFold[2] = true
	be.mu.Unlock()
	p.lastUpdate = p.lastUpdate.Add(-10 * 60 * 1e9) // 10 min earlier

	if err := p.UpdateData(context.Background()); err != nil {
		t.Fatalf("second update: %v", err)
	}
	second := photosByID(p.data)
	if len(second[2].Photos) == 0 {
		t.Fatal("folder 2 lost its thumbnails when its fetch failed; should have kept the cached ones")
	}
	if got := second[2].Photos[0].Thumbnails.Small; !strings.Contains(got, "/thumb/small/201") {
		t.Fatalf("folder 2 kept unexpected photo %q", got)
	}
	// Healthy folders still fine.
	for _, id := range []int{1, 3} {
		if len(second[id].Photos) == 0 {
			t.Fatalf("folder %d lost photos on refresh", id)
		}
	}
}
