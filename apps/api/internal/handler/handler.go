// Package handler wires the pipeline/store layers to HTTP, using the
// standard library's method+pattern ServeMux (Go 1.22+) rather than a
// separate router dependency.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blog-pipeline-api/internal/external/fileparser"
	"blog-pipeline-api/internal/pipeline"
	"blog-pipeline-api/internal/store"
)

const maxUploadBytes = 20 << 20 // 20MB

// Handler holds the dependencies HTTP handlers need.
type Handler struct {
	store     *store.Store
	uploadDir string
}

// New creates a Handler. uploadDir is where uploaded files are saved.
func New(s *store.Store, uploadDir string) *Handler {
	return &Handler{store: s, uploadDir: uploadDir}
}

// Routes registers all endpoints on a fresh ServeMux.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /posts", h.listPosts)
	mux.HandleFunc("GET /posts/{id}", h.getPost)
	mux.HandleFunc("POST /posts", h.createKeywordPost)
	mux.HandleFunc("POST /posts/upload", h.uploadFilePost)
	return mux
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := h.store.ListPosts(r.Context())
	if err != nil {
		log.Printf("listPosts: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list posts")
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (h *Handler) getPost(w http.ResponseWriter, r *http.Request) {
	post, err := h.store.GetPost(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		log.Printf("getPost: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get post")
		return
	}
	writeJSON(w, http.StatusOK, post)
}

type createKeywordPostRequest struct {
	ContentType string  `json:"content_type"`
	Category    string  `json:"category"`
	Subtype     *string `json:"subtype"`
	Keyword     string  `json:"keyword"`
}

// createKeywordPost creates a keyword-input post at StatusResearching. It
// does not trigger trend research: internal/external/naverdatalab isn't
// implemented yet, so the post stays at researching until that step exists.
func (h *Handler) createKeywordPost(w http.ResponseWriter, r *http.Request) {
	var req createKeywordPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	contentType := pipeline.ContentType(req.ContentType)
	if !isValidContentType(contentType) {
		writeError(w, http.StatusBadRequest, "content_type must be informational or experiential")
		return
	}
	if req.Category == "" {
		writeError(w, http.StatusBadRequest, "category is required")
		return
	}
	if req.Keyword == "" {
		writeError(w, http.StatusBadRequest, "keyword is required")
		return
	}

	post, err := h.store.CreatePost(r.Context(), store.CreatePostParams{
		ContentType:  contentType,
		Category:     req.Category,
		Subtype:      req.Subtype,
		InputMethod:  pipeline.InputMethodKeyword,
		InputKeyword: &req.Keyword,
		Status:       pipeline.StatusResearching,
	})
	if err != nil {
		log.Printf("createKeywordPost: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create post")
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

// uploadFilePost creates a file-input post: saves the upload, extracts text
// via fileparser, and lands the post at StatusResearched on success or
// StatusFailedFileParsing on failure. File input skips StatusResearching
// entirely, per the state diagram in the root CLAUDE.md.
func (h *Handler) uploadFilePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form or file too large")
		return
	}

	contentType := pipeline.ContentType(r.FormValue("content_type"))
	if !isValidContentType(contentType) {
		writeError(w, http.StatusBadRequest, "content_type must be informational or experiential")
		return
	}
	category := r.FormValue("category")
	if category == "" {
		writeError(w, http.StatusBadRequest, "category is required")
		return
	}
	var subtype *string
	if v := r.FormValue("subtype"); v != "" {
		subtype = &v
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	fileType, ok := detectFileType(header.Filename)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported file type (only .pdf and .xlsx accepted)")
		return
	}

	if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
		log.Printf("uploadFilePost: mkdir upload dir: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to prepare upload storage")
		return
	}
	storagePath := filepath.Join(h.uploadDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(header.Filename)))

	dst, err := os.Create(storagePath)
	if err != nil {
		log.Printf("uploadFilePost: create file: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store upload")
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		log.Printf("uploadFilePost: copy file: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store upload")
		return
	}
	dst.Close()

	var extractedText string
	var parseErr error
	if fileType == "pdf" {
		extractedText, parseErr = fileparser.ExtractPDFText(storagePath)
	} else {
		extractedText, parseErr = fileparser.ExtractExcelText(storagePath)
	}

	var post store.Post
	if parseErr != nil {
		msg := parseErr.Error()
		post, err = h.store.CreatePost(r.Context(), store.CreatePostParams{
			ContentType:        contentType,
			Category:           category,
			Subtype:            subtype,
			InputMethod:        pipeline.InputMethodFile,
			Status:             pipeline.StatusFailedFileParsing,
			StatusErrorMessage: &msg,
		})
	} else {
		post, err = h.store.CreatePost(r.Context(), store.CreatePostParams{
			ContentType: contentType,
			Category:    category,
			Subtype:     subtype,
			InputMethod: pipeline.InputMethodFile,
			Status:      pipeline.StatusResearched,
		})
	}
	if err != nil {
		log.Printf("uploadFilePost: create post: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create post")
		return
	}

	if _, err := h.store.CreateUploadedFile(r.Context(), post.ID, header.Filename, fileType, storagePath); err != nil {
		log.Printf("uploadFilePost: create uploaded file record: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to record uploaded file")
		return
	}

	if parseErr == nil {
		if _, err := h.store.CreateResearchResult(r.Context(), post.ID, "file_upload", nil, &extractedText); err != nil {
			log.Printf("uploadFilePost: create research result: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to record research result")
			return
		}
	}

	writeJSON(w, http.StatusCreated, post)
}

func isValidContentType(c pipeline.ContentType) bool {
	return c == pipeline.ContentTypeInformational || c == pipeline.ContentTypeExperiential
}

func detectFileType(filename string) (string, bool) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		return "pdf", true
	case ".xlsx":
		return "xlsx", true
	default:
		return "", false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
