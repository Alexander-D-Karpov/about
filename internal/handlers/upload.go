package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/config"
)

type UploadHandler struct {
	config *config.Config
}

type UploadResponse struct {
	Success  bool   `json:"success"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
	Error    string `json:"error,omitempty"`
}

func NewUploadHandler(config *config.Config) *UploadHandler {
	return &UploadHandler{
		config: config,
	}
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.jsonError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.jsonError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !h.isValidFileType(header.Filename) {
		h.jsonError(w, "Invalid file type", http.StatusBadRequest)
		return
	}

	if header.Size > 10<<20 {
		h.jsonError(w, "File too large (max 10MB)", http.StatusBadRequest)
		return
	}

	uploadsDir := filepath.Join(h.config.MediaPath, "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		h.jsonError(w, "Failed to create uploads directory", http.StatusInternalServerError)
		return
	}

	filename := h.sanitizeFilename(header.Filename)
	savePath := filepath.Join(uploadsDir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		h.jsonError(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		h.jsonError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	fileURL := fmt.Sprintf("/media/uploads/%s", filename)

	response := UploadResponse{
		Success:  true,
		URL:      fileURL,
		Filename: filename,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *UploadHandler) isValidFileType(filename string) bool {
	allowedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg"}
	ext := strings.ToLower(filepath.Ext(filename))

	for _, allowed := range allowedExts {
		if ext == allowed {
			return true
		}
	}
	return false
}

func (h *UploadHandler) sanitizeFilename(filename string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")

	if len(name) > 50 {
		name = name[:50]
	}

	return name + ext
}

func (h *UploadHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := UploadResponse{
		Success: false,
		Error:   message,
	}

	json.NewEncoder(w).Encode(response)
}
