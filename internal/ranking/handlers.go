package ranking

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/gorilla/mux"
)

type Handler struct {
	store        *Store
	config       *config.Config
	templates    map[string]*template.Template
	router       *mux.Router
	ws           *WSHub
	recentURLs   map[string]time.Time
	recentURLsMu sync.Mutex
}

func NewHandler(store *Store, cfg *config.Config, templateFS embed.FS) *Handler {
	h := &Handler{
		store:      store,
		config:     cfg,
		templates:  make(map[string]*template.Template),
		ws:         NewWSHub(),
		recentURLs: make(map[string]time.Time),
	}

	tmplNames := []string{"list", "view", "admin_list", "admin_edit"}
	for _, name := range tmplNames {
		t, err := template.New(name+".html").Funcs(template.FuncMap{
			"json": func(v interface{}) template.JS {
				b, _ := json.Marshal(v)
				return template.JS(b)
			},
			"add": func(a, b int) int { return a + b },
		}).ParseFS(templateFS, "templates/ranking/"+name+".html")
		if err != nil {
			log.Printf("[Ranking] Failed to parse template %s: %v", name, err)
			continue
		}
		h.templates[name] = t
	}

	h.router = mux.NewRouter()
	h.setupRoutes()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

func (h *Handler) setupRoutes() {
	h.router.HandleFunc("/ranking", h.handleList).Methods("GET")
	h.router.HandleFunc("/ranking/{slug}/ws", h.handleWS)
	h.router.HandleFunc("/ranking/{slug}", h.handleView).Methods("GET")

	admin := h.router.PathPrefix("/ranking/admin").Subrouter()
	admin.Use(h.basicAuth)
	admin.HandleFunc("", h.handleAdminList).Methods("GET")
	admin.HandleFunc("/", h.handleAdminList).Methods("GET")
	admin.HandleFunc("/create", h.handleCreate).Methods("POST")
	admin.HandleFunc("/{slug}", h.handleAdminEdit).Methods("GET")
	admin.HandleFunc("/{slug}/data", h.handleGetData).Methods("GET")
	admin.HandleFunc("/{slug}/save", h.handleSave).Methods("POST")
	admin.HandleFunc("/{slug}/publish", h.handlePublish).Methods("POST")
	admin.HandleFunc("/{slug}/delete", h.handleDelete).Methods("POST")
	admin.HandleFunc("/{slug}/upload", h.handleUpload).Methods("POST")
	admin.HandleFunc("/{slug}/tier", h.handleAddTier).Methods("POST")
	admin.HandleFunc("/{slug}/entry", h.handleAddEntry).Methods("POST")
	admin.HandleFunc("/{slug}/entry/{entryId}/delete", h.handleDeleteEntry).Methods("POST")
	admin.HandleFunc("/{slug}/upload-url", h.handleUploadURL).Methods("POST")
	admin.HandleFunc("/{slug}/regen-thumbs", h.handleRegenThumbs).Methods("POST")
	admin.HandleFunc("/{slug}/clean-thumbs", h.handleCleanThumbs).Methods("POST")
}

func (h *Handler) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != h.config.AdminUser || pass != h.config.AdminPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="Ranking Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	tierlists := h.store.GetAllPublished()
	h.render(w, "list", map[string]interface{}{"Tierlists": tierlists, "Title": "Tier Rankings"})
}

func (h *Handler) handleView(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil || !tl.Published {
		http.NotFound(w, r)
		return
	}

	tierEntries := make(map[int][]Entry)
	for _, t := range tl.Tiers {
		tierEntries[t.ID] = []Entry{}
	}
	var unsorted []Entry
	for _, e := range tl.Entries {
		if e.TierID != nil {
			tierEntries[*e.TierID] = append(tierEntries[*e.TierID], e)
		} else {
			unsorted = append(unsorted, e)
		}
	}

	h.render(w, "view", map[string]interface{}{
		"Tierlist":    tl,
		"Title":       tl.Title,
		"TierEntries": tierEntries,
		"Unsorted":    unsorted,
	})
}

func (h *Handler) handleAdminList(w http.ResponseWriter, r *http.Request) {
	tierlists := h.store.GetAll()
	h.render(w, "admin_list", map[string]interface{}{"Tierlists": tierlists, "Title": "Manage Tierlists"})
}

func (h *Handler) handleAdminEdit(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, "admin_edit", map[string]interface{}{"Tierlist": tl, "Title": "Edit: " + tl.Title})
}

func (h *Handler) handleGetData(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tl)
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	title := r.FormValue("title")
	if title == "" {
		title = "New Tierlist"
	}
	description := r.FormValue("description")

	tl, err := h.store.Create(title, description)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/ranking/admin/"+tl.Slug, http.StatusSeeOther)
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]

	var req SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json: "+err.Error(), 400)
		return
	}

	if err := h.store.Save(slug, req); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	tl := h.store.GetBySlug(slug)
	if tl != nil {
		if data, err := json.Marshal(tl); err == nil {
			h.ws.Broadcast(slug, data, nil)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *Handler) handlePublish(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	if err := h.store.SetPublished(slug, !tl.Published); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "published": !tl.Published})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	if err := h.store.Delete(slug); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "parse error", 400)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "no file", 400)
		return
	}
	defer file.Close()

	tierIDStr := r.FormValue("tier_id")
	var tierID *int
	if tierIDStr != "" && tierIDStr != "null" && tierIDStr != "0" {
		id, err := strconv.Atoi(tierIDStr)
		if err == nil {
			tierID = &id
		}
	}

	dir := filepath.Join(h.config.MediaPath, "ranking", slug)
	thumbDir := filepath.Join(dir, "thumbs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		jsonError(w, "mkdir failed", 500)
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true}
	if !allowed[ext] {
		jsonError(w, "invalid file type", 400)
		return
	}

	ts := time.Now().Format("20060102150405")
	safeName := sanitize(strings.TrimSuffix(header.Filename, ext))
	filename := fmt.Sprintf("%s_%s%s", safeName, ts, ext)
	savePath := filepath.Join(dir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		jsonError(w, "create failed", 500)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		jsonError(w, "copy failed", 500)
		return
	}
	out.Close()

	imagePath := fmt.Sprintf("/media/ranking/%s/%s", slug, filename)
	thumbPath := ""

	if ext != ".svg" {
		thumbName, err := generateThumbnail(savePath, thumbDir)
		if err != nil {
			log.Printf("[Ranking] Thumbnail generation failed: %v", err)
		} else {
			thumbPath = fmt.Sprintf("/media/ranking/%s/thumbs/%s", slug, thumbName)
		}
	}

	entry := Entry{
		TierlistID: tl.ID,
		TierID:     tierID,
		Name:       "",
		ImagePath:  imagePath,
		ThumbPath:  thumbPath,
		Position:   0,
	}

	saved, err := h.store.AddEntry(tl.ID, slug, entry)
	if err != nil {
		jsonError(w, "db error: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (h *Handler) handleAddEntry(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	var req struct {
		Name   string `json:"name"`
		TierID *int   `json:"tier_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", 400)
		return
	}

	entry := Entry{
		TierlistID: tl.ID,
		TierID:     req.TierID,
		Name:       req.Name,
		Position:   0,
	}

	saved, err := h.store.AddEntry(tl.ID, slug, entry)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (h *Handler) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	idStr := mux.Vars(r)["entryId"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	if err := h.store.DeleteEntry(slug, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *Handler) handleAddTier(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", 400)
		return
	}
	if req.Name == "" {
		req.Name = "New Tier"
	}
	if req.Color == "" {
		req.Color = "#CCCCCC"
	}

	tier := Tier{TierlistID: tl.ID, Name: req.Name, Color: req.Color, Position: 0}
	saved, err := h.store.AddTier(tl.ID, slug, tier)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	t, ok := h.templates[name]
	if !ok {
		http.Error(w, "template not found: "+name, 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Printf("[Ranking] Template error %s: %v", name, err)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func sanitize(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		name = "entry"
	}
	return name
}

func (h *Handler) deduplicateURL(slug, resolvedURL string) bool {
	h.recentURLsMu.Lock()
	defer h.recentURLsMu.Unlock()

	key := slug + "|" + resolvedURL
	if t, ok := h.recentURLs[key]; ok && time.Since(t) < 30*time.Second {
		return true
	}
	h.recentURLs[key] = time.Now()

	for k, t := range h.recentURLs {
		if time.Since(t) > 60*time.Second {
			delete(h.recentURLs, k)
		}
	}
	return false
}

func (h *Handler) handleUploadURL(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	var req struct {
		URL         string `json:"url"`
		FallbackURL string `json:"fallback_url"`
		TierID      *int   `json:"tier_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", 400)
		return
	}
	if req.URL == "" {
		jsonError(w, "no url", 400)
		return
	}

	imageURL := resolveImageURL(req.URL)

	if h.deduplicateURL(slug, imageURL) {
		jsonError(w, "duplicate upload (same URL already processed)", 409)
		return
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return fmt.Errorf("too many redirects")
			}
			r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TierlistBot/1.0)")
			return nil
		},
	}

	body, ct, err := h.fetchImage(imageURL, client)
	if err != nil && req.FallbackURL != "" {
		fallbackResolved := resolveImageURL(req.FallbackURL)
		body, ct, err = h.fetchImage(fallbackResolved, client)
	}
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	defer body.Close()

	extMap := map[string]string{
		"image/jpeg":    ".jpg",
		"image/png":     ".png",
		"image/gif":     ".gif",
		"image/webp":    ".webp",
		"image/svg+xml": ".svg",
	}
	ext := ".jpg"
	for mime, e := range extMap {
		if strings.Contains(ct, mime) {
			ext = e
			break
		}
	}

	dir := filepath.Join(h.config.MediaPath, "ranking", slug)
	thumbDir := filepath.Join(dir, "thumbs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		jsonError(w, "mkdir failed", 500)
		return
	}

	ts := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("ext_%s%s", ts, ext)
	savePath := filepath.Join(dir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		jsonError(w, "create failed", 500)
		return
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		jsonError(w, "copy failed", 500)
		return
	}
	out.Close()

	imagePath := fmt.Sprintf("/media/ranking/%s/%s", slug, filename)
	thumbPath := ""
	if ext != ".svg" {
		thumbName, err := generateThumbnail(savePath, thumbDir)
		if err != nil {
			log.Printf("[Ranking] Thumbnail generation failed for URL upload: %v", err)
		} else {
			thumbPath = fmt.Sprintf("/media/ranking/%s/thumbs/%s", slug, thumbName)
		}
	}

	entry := Entry{
		TierlistID: tl.ID,
		TierID:     req.TierID,
		Name:       "",
		ImagePath:  imagePath,
		ThumbPath:  thumbPath,
		Position:   0,
	}

	saved, err := h.store.AddEntry(tl.ID, slug, entry)
	if err != nil {
		jsonError(w, "db error: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (h *Handler) fetchImage(imageURL string, client *http.Client) (io.ReadCloser, string, error) {
	fetchReq, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("invalid url")
	}
	fetchReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TierlistBot/1.0)")
	fetchReq.Header.Set("Accept", "image/*,*/*")

	resp, err := client.Do(fetchReq)
	if err != nil {
		return nil, "", fmt.Errorf("fetch failed: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("fetch returned %d for %s", resp.StatusCode, imageURL)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "image/") {
		return resp.Body, ct, nil
	}

	resp.Body.Close()

	ogImage := tryExtractOGImage(imageURL, client)
	if ogImage == "" {
		return nil, "", fmt.Errorf("url did not return an image (got %s)", ct)
	}

	return h.fetchImage(ogImage, client)
}

func tryExtractOGImage(pageURL string, client *http.Client) string {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TierlistBot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body := make([]byte, 64*1024)
	n, _ := io.ReadAtLeast(resp.Body, body, 1)
	if n == 0 {
		return ""
	}
	html := string(body[:n])

	patterns := []string{
		`property="og:image"`,
		`name="og:image"`,
		`property="twitter:image"`,
		`name="twitter:image"`,
	}

	lower := strings.ToLower(html)
	for _, pat := range patterns {
		idx := strings.Index(lower, pat)
		if idx == -1 {
			continue
		}
		region := html[max(0, idx-200):min(len(html), idx+300)]
		contentIdx := strings.Index(strings.ToLower(region), `content="`)
		if contentIdx == -1 {
			continue
		}
		start := contentIdx + 9
		end := strings.Index(region[start:], `"`)
		if end == -1 {
			continue
		}
		u := strings.TrimSpace(region[start : start+end])
		if strings.HasPrefix(u, "http") {
			return u
		}
	}

	return ""
}

func resolveImageURL(rawURL string) string {
	if strings.Contains(rawURL, "google.") && strings.Contains(rawURL, "/imgres") {
		if idx := strings.Index(rawURL, "imgurl="); idx != -1 {
			rest := rawURL[idx+7:]
			if end := strings.IndexByte(rest, '&'); end != -1 {
				rest = rest[:end]
			}
			if decoded, err := url.QueryUnescape(rest); err == nil && decoded != "" {
				return decoded
			}
		}
	}

	if strings.Contains(rawURL, "commons.wikimedia.org/wiki/File:") {
		parts := strings.SplitN(rawURL, "File:", 2)
		if len(parts) == 2 {
			filename := strings.SplitN(parts[1], "?", 2)[0]
			filename = strings.ReplaceAll(filename, " ", "_")
			return "https://commons.wikimedia.org/wiki/Special:FilePath/" + filename
		}
	}

	if strings.Contains(rawURL, "upload.wikimedia.org") {
		return rawURL
	}

	if strings.Contains(rawURL, "wikipedia.org") && strings.Contains(rawURL, "/wiki/File:") {
		parts := strings.SplitN(rawURL, "File:", 2)
		if len(parts) == 2 {
			filename := strings.SplitN(parts[1], "?", 2)[0]
			filename = strings.ReplaceAll(filename, " ", "_")
			return "https://commons.wikimedia.org/wiki/Special:FilePath/" + filename
		}
	}

	if strings.Contains(rawURL, "imgur.com") && !strings.Contains(rawURL, "i.imgur.com") {
		rawURL = strings.Replace(rawURL, "imgur.com", "i.imgur.com", 1)
		if !strings.Contains(rawURL, ".") || strings.HasSuffix(rawURL, ".com") {
			rawURL += ".jpg"
		}
	}

	return rawURL
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	h.ws.ServeWS(w, r, slug)
}

func (h *Handler) handleRegenThumbs(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	dir := filepath.Join(h.config.MediaPath, "ranking", slug)
	thumbDir := filepath.Join(dir, "thumbs")

	regenerated := 0
	failed := 0
	for _, e := range tl.Entries {
		if e.ImagePath == "" {
			continue
		}
		rel := strings.TrimPrefix(e.ImagePath, "/media/")
		srcPath := filepath.Join(h.config.MediaPath, rel)
		if strings.HasSuffix(strings.ToLower(srcPath), ".svg") {
			continue
		}
		if _, err := os.Stat(srcPath); err != nil {
			failed++
			continue
		}
		thumbName, err := generateThumbnail(srcPath, thumbDir)
		if err != nil {
			log.Printf("[Ranking] regen failed for %s: %v", srcPath, err)
			failed++
			continue
		}
		newThumb := fmt.Sprintf("/media/ranking/%s/thumbs/%s", slug, thumbName)
		if newThumb != e.ThumbPath {
			_, _ = h.store.db.Exec(`UPDATE tier_entries SET thumb_path=$1 WHERE id=$2`, newThumb, e.ID)
		}
		regenerated++
	}

	if err := h.store.reloadOne(slug); err != nil {
		log.Printf("[Ranking] reload after regen failed: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"regenerated": regenerated, "failed": failed})
}

func (h *Handler) handleCleanThumbs(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	tl := h.store.GetBySlug(slug)
	if tl == nil {
		jsonError(w, "not found", 404)
		return
	}

	keep := make(map[string]bool)
	for _, e := range tl.Entries {
		if e.ThumbPath == "" {
			continue
		}
		keep[filepath.Base(e.ThumbPath)] = true
	}

	thumbDir := filepath.Join(h.config.MediaPath, "ranking", slug, "thumbs")
	files, err := os.ReadDir(thumbDir)
	if err != nil {
		jsonError(w, "read thumbs dir: "+err.Error(), 500)
		return
	}

	removed := 0
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !keep[f.Name()] {
			if err := os.Remove(filepath.Join(thumbDir, f.Name())); err != nil {
				log.Printf("[Ranking] failed to remove orphan thumb %s: %v", f.Name(), err)
				continue
			}
			removed++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"removed": removed, "kept": len(keep)})
}
