// Package handler wires the pipeline/store layers to HTTP, using the
// standard library's method+pattern ServeMux (Go 1.22+) rather than a
// separate router dependency.
package handler

import (
	"context"
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
	"blog-pipeline-api/internal/external/llm"
	"blog-pipeline-api/internal/external/naversearch"
	"blog-pipeline-api/internal/pipeline"
	"blog-pipeline-api/internal/store"
)

const maxUploadBytes = 20 << 20 // 20MB

// draftGenerator is satisfied by *llm.Client. A narrow interface so tests
// can inject a fake instead of making real Anthropic API calls.
type draftGenerator interface {
	GenerateDraft(ctx context.Context, in llm.DraftInput) (llm.DraftOutput, error)
}

// blogSearcher is satisfied by *naversearch.Client. A narrow interface so
// tests can inject a fake instead of making real Naver API calls.
type blogSearcher interface {
	SearchBlogs(ctx context.Context, query string, display int) ([]naversearch.BlogResult, error)
}

// Handler holds the dependencies HTTP handlers need.
type Handler struct {
	store     *store.Store
	uploadDir string
	llm       draftGenerator
	searcher  blogSearcher
}

// New creates a Handler. uploadDir is where uploaded files are saved.
func New(s *store.Store, uploadDir string, llmClient draftGenerator, searcher blogSearcher) *Handler {
	return &Handler{store: s, uploadDir: uploadDir, llm: llmClient, searcher: searcher}
}

// Routes registers all endpoints on a fresh ServeMux.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /posts", h.listPosts)
	mux.HandleFunc("GET /posts/{id}", h.getPost)
	mux.HandleFunc("DELETE /posts/{id}", h.deletePost)
	mux.HandleFunc("POST /posts", h.createKeywordPost)
	mux.HandleFunc("POST /posts/upload", h.uploadFilePost)
	mux.HandleFunc("POST /posts/{id}/draft", h.draftPost)
	mux.HandleFunc("GET /posts/{id}/drafts", h.listDrafts)
	mux.HandleFunc("POST /posts/{id}/approve", h.approvePost)
	mux.HandleFunc("POST /posts/{id}/reject", h.rejectPost)
	mux.HandleFunc("POST /posts/{id}/archive", h.archivePost)
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

// deletePost removes a post and everything tied to it (upload record,
// research data, drafts, review actions — cascaded via FK constraints).
// No status restriction: this is a solo-user internal tool, and deleting a
// mistaken/test entry at any stage is a reasonable thing to want.
func (h *Handler) deletePost(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeletePost(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		log.Printf("deletePost: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete post")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// draftPost generates (or regenerates, on the revision loop) a draft for a
// post: researched/needs_revision -> drafting -> draft_ready -> pending_review.
// The Naver search reference lookup is best-effort — a failure there logs
// and continues without reference titles/snippets rather than failing the
// whole request, since it's a quality enhancement, not the core step. The
// LLM call is the load-bearing step: its failure lands the post on
// failed_drafting.
func (h *Handler) draftPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	post, err := h.store.GetPost(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		log.Printf("draftPost: get post: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get post")
		return
	}

	if err := h.store.UpdateStatus(ctx, id, pipeline.StatusDrafting, nil); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot start drafting: %v", err))
		return
	}

	research, err := h.store.GetLatestResearchResult(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			msg := "no research data available for this post"
			h.failDrafting(ctx, id, msg)
			writeError(w, http.StatusUnprocessableEntity, msg)
			return
		}
		log.Printf("draftPost: get research result: %v", err)
		h.failDrafting(ctx, id, err.Error())
		writeError(w, http.StatusInternalServerError, "failed to load research data")
		return
	}
	var sourceText string
	if research.ExtractedText != nil {
		sourceText = *research.ExtractedText
	}

	var titles, snippets []string
	if results, err := h.searcher.SearchBlogs(ctx, searchQueryFor(post), 5); err != nil {
		log.Printf("draftPost: naver search (non-fatal): %v", err)
	} else {
		for _, res := range results {
			titles = append(titles, res.Title)
			snippets = append(snippets, res.Description)
		}
	}

	var subtype string
	if post.Subtype != nil {
		subtype = *post.Subtype
	}
	draftOut, err := h.llm.GenerateDraft(ctx, llm.DraftInput{
		Category:          post.Category,
		Subtype:           subtype,
		ContentType:       string(post.ContentType),
		SourceText:        sourceText,
		ReferenceTitles:   titles,
		ReferenceSnippets: snippets,
	})
	if err != nil {
		log.Printf("draftPost: generate draft: %v", err)
		h.failDrafting(ctx, id, err.Error())
		writeError(w, http.StatusBadGateway, "failed to generate draft")
		return
	}

	version, err := h.store.NextDraftVersion(ctx, id)
	if err != nil {
		log.Printf("draftPost: next draft version: %v", err)
		h.failDrafting(ctx, id, err.Error())
		writeError(w, http.StatusInternalServerError, "failed to determine draft version")
		return
	}

	imageAltsJSON, err := json.Marshal(draftOut.ImageAlts)
	if err != nil {
		log.Printf("draftPost: marshal image alts (non-fatal): %v", err)
		imageAltsJSON = nil
	}

	draft, err := h.store.CreateDraft(ctx, id, version, draftOut.Content, &draftOut.MetaTitle, &draftOut.MetaDescription, imageAltsJSON)
	if err != nil {
		log.Printf("draftPost: create draft: %v", err)
		h.failDrafting(ctx, id, err.Error())
		writeError(w, http.StatusInternalServerError, "failed to save draft")
		return
	}

	if err := h.store.UpdateStatus(ctx, id, pipeline.StatusDraftReady, nil); err != nil {
		log.Printf("draftPost: update status draft_ready: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	if err := h.store.UpdateStatus(ctx, id, pipeline.StatusPendingReview, nil); err != nil {
		log.Printf("draftPost: update status pending_review: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update status")
		return
	}

	writeJSON(w, http.StatusCreated, draft)
}

// failDrafting best-effort transitions a post to failed_drafting with errMsg.
// Logs but doesn't surface a secondary error if the transition itself fails.
func (h *Handler) failDrafting(ctx context.Context, postID, errMsg string) {
	if err := h.store.UpdateStatus(ctx, postID, pipeline.StatusFailedDrafting, &errMsg); err != nil {
		log.Printf("draftPost: failed to record failed_drafting: %v", err)
	}
}

// searchQueryFor picks the Naver search query for a post: its explicit
// keyword for keyword_input, or category(+subtype) for file_input, which
// has no keyword field.
func searchQueryFor(post store.Post) string {
	if post.InputMethod == pipeline.InputMethodKeyword && post.InputKeyword != nil {
		return *post.InputKeyword
	}
	query := post.Category
	if post.Subtype != nil && *post.Subtype != "" {
		query += " " + *post.Subtype
	}
	return query
}

func (h *Handler) listDrafts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetPost(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		log.Printf("listDrafts: get post: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get post")
		return
	}

	drafts, err := h.store.ListDrafts(r.Context(), id)
	if err != nil {
		log.Printf("listDrafts: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list drafts")
		return
	}
	writeJSON(w, http.StatusOK, drafts)
}

// approvePost moves a post from pending_review to approved and records the
// decision in review_actions.
func (h *Handler) approvePost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	draft, ok := h.prepareReviewAction(w, r, id)
	if !ok {
		return
	}

	if err := h.store.UpdateStatus(r.Context(), id, pipeline.StatusApproved, nil); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot approve: %v", err))
		return
	}
	if _, err := h.store.CreateReviewAction(r.Context(), id, draft.ID, "approve", nil); err != nil {
		log.Printf("approvePost: create review action: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to record review action")
		return
	}

	h.respondWithPost(w, r, id)
}

type rejectRequest struct {
	FeedbackNote *string `json:"feedback_note"`
}

// rejectPost moves a post from pending_review to needs_revision and records
// the decision (with optional feedback) in review_actions. Regenerating the
// draft afterward goes through POST /posts/{id}/draft, which already
// supports needs_revision -> drafting.
func (h *Handler) rejectPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req rejectRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	draft, ok := h.prepareReviewAction(w, r, id)
	if !ok {
		return
	}

	if err := h.store.UpdateStatus(r.Context(), id, pipeline.StatusNeedsRevision, nil); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot reject: %v", err))
		return
	}
	if _, err := h.store.CreateReviewAction(r.Context(), id, draft.ID, "reject", req.FeedbackNote); err != nil {
		log.Printf("rejectPost: create review action: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to record review action")
		return
	}

	h.respondWithPost(w, r, id)
}

// archivePost moves a post from approved to archived: the user's manual
// mark that they've copied the approved content into the Naver editor.
// No review_actions row — this isn't a review decision.
func (h *Handler) archivePost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.UpdateStatus(r.Context(), id, pipeline.StatusArchived, nil); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot archive: %v", err))
		return
	}
	h.respondWithPost(w, r, id)
}

// prepareReviewAction fetches the post and its latest draft (needed for
// review_actions.draft_id), writing an error response and returning
// ok=false if either lookup fails.
func (h *Handler) prepareReviewAction(w http.ResponseWriter, r *http.Request, id string) (store.Draft, bool) {
	if _, err := h.store.GetPost(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "post not found")
			return store.Draft{}, false
		}
		log.Printf("prepareReviewAction: get post: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get post")
		return store.Draft{}, false
	}

	draft, err := h.store.GetLatestDraft(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "post has no draft to review")
			return store.Draft{}, false
		}
		log.Printf("prepareReviewAction: get latest draft: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get latest draft")
		return store.Draft{}, false
	}
	return draft, true
}

// respondWithPost re-fetches and writes the post as JSON — used after a
// status-changing action to return the post's current state.
func (h *Handler) respondWithPost(w http.ResponseWriter, r *http.Request, id string) {
	post, err := h.store.GetPost(r.Context(), id)
	if err != nil {
		log.Printf("respondWithPost: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load updated post")
		return
	}
	writeJSON(w, http.StatusOK, post)
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
