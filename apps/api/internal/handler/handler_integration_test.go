//go:build integration

// Integration test against a real (embedded) Postgres instance. Excluded
// from the default `go test ./...` run; run explicitly with
// `go test -tags=integration ./internal/handler/...`.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
	"github.com/razvandimescu/gopdf/pdf"

	"blog-pipeline-api/internal/external/llm"
	"blog-pipeline-api/internal/external/naversearch"
	"blog-pipeline-api/internal/handler"
	"blog-pipeline-api/internal/store"
)

// fakeLLM and fakeSearcher stand in for the real Anthropic/Naver clients so
// the orchestration (status transitions, versioning, error handling) can be
// tested without real API credentials. Their fields are mutated between
// subtests, which run sequentially (no t.Parallel).
type fakeLLM struct {
	output llm.DraftOutput
	err    error

	keyword    string
	keywordErr error
}

func (f *fakeLLM) GenerateDraft(ctx context.Context, in llm.DraftInput) (llm.DraftOutput, error) {
	return f.output, f.err
}

func (f *fakeLLM) ExtractKeyword(ctx context.Context, sourceText string) (string, error) {
	return f.keyword, f.keywordErr
}

type fakeSearcher struct {
	results []naversearch.BlogResult
	err     error
}

func (f *fakeSearcher) SearchBlogs(ctx context.Context, query string, display int) ([]naversearch.BlogResult, error) {
	return f.results, f.err
}

const integrationPort = 15433

func TestHandlerIntegration(t *testing.T) {
	dsn := "postgres://postgres:postgres@localhost:15433/postgres?sslmode=disable"
	ctx := context.Background()

	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().Port(integrationPort).RuntimePath(t.TempDir()).Logger(io.Discard),
	)
	if err := pg.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	defer func() {
		if err := pg.Stop(); err != nil {
			t.Errorf("stop embedded postgres: %v", err)
		}
	}()

	applyMigration(t, ctx, dsn)

	s, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	fakeGenerator := &fakeLLM{}
	fakeBlogSearcher := &fakeSearcher{}
	h := handler.New(s, t.TempDir(), fakeGenerator, fakeBlogSearcher)
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	t.Run("empty list", func(t *testing.T) {
		var posts []map[string]any
		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts", nil, http.StatusOK, &posts)
		if len(posts) != 0 {
			t.Fatalf("got %d posts, want 0", len(posts))
		}
	})

	var keywordPostID string
	t.Run("create keyword post", func(t *testing.T) {
		body := strings.NewReader(`{"content_type":"informational","category":"생활정보_제도안내","keyword":"청약 자격 요건"}`)
		var post map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts", body, http.StatusCreated, &post)
		if post["status"] != "researching" {
			t.Fatalf("got status %v, want researching", post["status"])
		}
		keywordPostID, _ = post["id"].(string)
		if keywordPostID == "" {
			t.Fatal("expected a generated ID")
		}
	})

	t.Run("get created post", func(t *testing.T) {
		var post map[string]any
		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts/"+keywordPostID, nil, http.StatusOK, &post)
		if post["id"] != keywordPostID {
			t.Fatalf("got id %v, want %v", post["id"], keywordPostID)
		}
	})

	t.Run("get missing post is 404", func(t *testing.T) {
		resp, err := srv.Client().Get(srv.URL + "/posts/00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("got status %d, want 404", resp.StatusCode)
		}
	})

	t.Run("upload valid pdf lands on researched", func(t *testing.T) {
		pdfBytes := makeTestPDF(t)
		resp := uploadFile(t, srv.Client(), srv.URL, "notice.pdf", pdfBytes)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("got status %d, want 201", resp.StatusCode)
		}
		var post map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&post); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if post["status"] != "researched" {
			t.Fatalf("got status %v, want researched", post["status"])
		}
	})

	t.Run("upload corrupt pdf lands on failed_file_parsing", func(t *testing.T) {
		resp := uploadFile(t, srv.Client(), srv.URL, "corrupt.pdf", []byte("not a real pdf"))
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("got status %d, want 201", resp.StatusCode)
		}
		var post map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&post); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if post["status"] != "failed_file_parsing" {
			t.Fatalf("got status %v, want failed_file_parsing", post["status"])
		}
	})

	t.Run("draft succeeds and reaches pending_review", func(t *testing.T) {
		researchedID := uploadResearchedPost(t, srv)

		fakeGenerator.err = nil
		fakeGenerator.output = llm.DraftOutput{
			Content:         "본문 초안",
			MetaTitle:       "테스트 제목",
			MetaDescription: "테스트 설명",
			UsedImages:      []string{"img1.png"},
		}
		fakeBlogSearcher.err = nil
		fakeBlogSearcher.results = []naversearch.BlogResult{
			{Title: "참고 제목", Description: "참고 스니펫"},
		}

		var draft map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+researchedID+"/draft", nil, http.StatusCreated, &draft)
		if draft["version"] != float64(1) {
			t.Fatalf("got version %v, want 1", draft["version"])
		}
		if draft["content"] != "본문 초안" {
			t.Fatalf("got content %v", draft["content"])
		}

		var post map[string]any
		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts/"+researchedID, nil, http.StatusOK, &post)
		if post["status"] != "pending_review" {
			t.Fatalf("got status %v, want pending_review", post["status"])
		}
	})

	t.Run("draft fails when llm errors, lands on failed_drafting", func(t *testing.T) {
		researchedID := uploadResearchedPost(t, srv)

		fakeGenerator.err = errors.New("anthropic: rate limited")
		fakeBlogSearcher.err = nil
		fakeBlogSearcher.results = nil

		resp, err := srv.Client().Post(srv.URL+"/posts/"+researchedID+"/draft", "application/json", nil)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("got status %d, want 502", resp.StatusCode)
		}

		var post map[string]any
		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts/"+researchedID, nil, http.StatusOK, &post)
		if post["status"] != "failed_drafting" {
			t.Fatalf("got status %v, want failed_drafting", post["status"])
		}
		if post["status_error_message"] == nil {
			t.Fatal("expected status_error_message to be set")
		}
	})

	t.Run("draft succeeds even if naver search fails (non-fatal)", func(t *testing.T) {
		researchedID := uploadResearchedPost(t, srv)

		fakeGenerator.err = nil
		fakeGenerator.output = llm.DraftOutput{
			Content:         "본문 초안 2",
			MetaTitle:       "테스트 제목 2",
			MetaDescription: "테스트 설명 2",
		}
		fakeGenerator.keyword = "테스트 키워드"
		fakeGenerator.keywordErr = nil
		fakeBlogSearcher.err = errors.New("naver: timeout")
		fakeBlogSearcher.results = nil

		var draft map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+researchedID+"/draft", nil, http.StatusCreated, &draft)
		if draft["content"] != "본문 초안 2" {
			t.Fatalf("got content %v, want draft despite search failure", draft["content"])
		}
	})

	t.Run("draft succeeds even if keyword extraction fails (non-fatal, falls back to category)", func(t *testing.T) {
		researchedID := uploadResearchedPost(t, srv)

		fakeGenerator.err = nil
		fakeGenerator.output = llm.DraftOutput{
			Content:         "본문 초안 3",
			MetaTitle:       "테스트 제목 3",
			MetaDescription: "테스트 설명 3",
		}
		fakeGenerator.keyword = ""
		fakeGenerator.keywordErr = errors.New("anthropic: rate limited")
		fakeBlogSearcher.err = nil
		fakeBlogSearcher.results = nil

		var draft map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+researchedID+"/draft", nil, http.StatusCreated, &draft)
		if draft["content"] != "본문 초안 3" {
			t.Fatalf("got content %v, want draft despite keyword extraction failure", draft["content"])
		}
	})

	t.Run("approve moves pending_review to approved", func(t *testing.T) {
		id := draftToPendingReview(t, srv, fakeGenerator, fakeBlogSearcher)

		var post map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/approve", nil, http.StatusOK, &post)
		if post["status"] != "approved" {
			t.Fatalf("got status %v, want approved", post["status"])
		}
	})

	t.Run("reject moves to needs_revision, and redraft produces version 2", func(t *testing.T) {
		id := draftToPendingReview(t, srv, fakeGenerator, fakeBlogSearcher)

		body := strings.NewReader(`{"feedback_note":"제목 다시"}`)
		var post map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/reject", body, http.StatusOK, &post)
		if post["status"] != "needs_revision" {
			t.Fatalf("got status %v, want needs_revision", post["status"])
		}

		fakeGenerator.output = llm.DraftOutput{
			Content:         "본문 초안 v2",
			MetaTitle:       "테스트 제목 v2",
			MetaDescription: "테스트 설명 v2",
		}
		var draft map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/draft", nil, http.StatusCreated, &draft)
		if draft["version"] != float64(2) {
			t.Fatalf("got version %v, want 2", draft["version"])
		}

		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts/"+id, nil, http.StatusOK, &post)
		if post["status"] != "pending_review" {
			t.Fatalf("got status %v, want pending_review after redraft", post["status"])
		}
	})

	t.Run("archive moves approved to archived", func(t *testing.T) {
		id := draftToPendingReview(t, srv, fakeGenerator, fakeBlogSearcher)
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/approve", nil, http.StatusOK, nil)

		var post map[string]any
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/archive", nil, http.StatusOK, &post)
		if post["status"] != "archived" {
			t.Fatalf("got status %v, want archived", post["status"])
		}
	})

	t.Run("approve without a draft is rejected", func(t *testing.T) {
		id := uploadResearchedPost(t, srv)

		resp, err := srv.Client().Post(srv.URL+"/posts/"+id+"/approve", "application/json", nil)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("got status %d, want 422 (no draft to review)", resp.StatusCode)
		}
	})

	t.Run("approving an already-approved post is rejected", func(t *testing.T) {
		id := draftToPendingReview(t, srv, fakeGenerator, fakeBlogSearcher)
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/approve", nil, http.StatusOK, nil)

		// Has a draft now, so this exercises pipeline.Transition rejecting
		// approved -> approved rather than the "no draft" 422 case above.
		resp, err := srv.Client().Post(srv.URL+"/posts/"+id+"/approve", "application/json", nil)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400", resp.StatusCode)
		}
	})

	t.Run("list drafts returns versions newest first", func(t *testing.T) {
		id := draftToPendingReview(t, srv, fakeGenerator, fakeBlogSearcher)
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/reject", nil, http.StatusOK, nil)

		fakeGenerator.output = llm.DraftOutput{
			Content:         "v2",
			MetaTitle:       "t2",
			MetaDescription: "d2",
		}
		doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/draft", nil, http.StatusCreated, nil)

		var drafts []map[string]any
		doJSON(t, srv.Client(), http.MethodGet, srv.URL+"/posts/"+id+"/drafts", nil, http.StatusOK, &drafts)
		if len(drafts) != 2 {
			t.Fatalf("got %d drafts, want 2", len(drafts))
		}
		if drafts[0]["version"] != float64(2) || drafts[1]["version"] != float64(1) {
			t.Fatalf("drafts not ordered newest-first: %v", drafts)
		}
	})

	t.Run("delete removes a post so get 404s afterward", func(t *testing.T) {
		id := uploadResearchedPost(t, srv)

		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/posts/"+id, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("DELETE: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("got status %d, want 204", resp.StatusCode)
		}

		getResp, err := srv.Client().Get(srv.URL + "/posts/" + id)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer getResp.Body.Close()
		if getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("got status %d, want 404 after delete", getResp.StatusCode)
		}
	})

	t.Run("delete on missing post is 404", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/posts/00000000-0000-0000-0000-000000000000", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("DELETE: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("got status %d, want 404", resp.StatusCode)
		}
	})
}

// draftToPendingReview uploads a fresh PDF and drafts it through to
// pending_review using the given fakes, returning the post ID.
func draftToPendingReview(t *testing.T, srv *httptest.Server, gen *fakeLLM, searcher *fakeSearcher) string {
	t.Helper()
	id := uploadResearchedPost(t, srv)

	gen.err = nil
	gen.output = llm.DraftOutput{
		Content:         "본문 초안",
		MetaTitle:       "테스트 제목",
		MetaDescription: "테스트 설명",
	}
	gen.keyword = "테스트 키워드"
	gen.keywordErr = nil
	searcher.err = nil
	searcher.results = nil

	doJSON(t, srv.Client(), http.MethodPost, srv.URL+"/posts/"+id+"/draft", nil, http.StatusCreated, nil)
	return id
}

// uploadResearchedPost uploads a fresh valid PDF and returns the resulting
// post's ID (status researched).
func uploadResearchedPost(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	resp := uploadFile(t, srv.Client(), srv.URL, "notice.pdf", makeTestPDF(t))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: got status %d, want 201", resp.StatusCode)
	}
	var post map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&post); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if post["status"] != "researched" {
		t.Fatalf("got status %v, want researched", post["status"])
	}
	id, _ := post["id"].(string)
	if id == "" {
		t.Fatal("expected a generated ID")
	}
	return id
}

func uploadFile(t *testing.T, client *http.Client, baseURL, filename string, content []byte) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("content_type", "informational"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := w.WriteField("category", "생활정보_제도안내"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/posts/upload", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func makeTestPDF(t *testing.T) []byte {
	t.Helper()
	c := pdf.NewCreator()
	page := c.NewPage(595, 842)
	page.SetFont("Helvetica", 12)
	// ASCII only: gopdf's creator supports the standard 14 fonts, which have
	// no Korean glyphs — non-ASCII text here would corrupt on round-trip.
	page.DrawText(72, 750, "housing subscription notice")
	data, err := c.Build()
	if err != nil {
		t.Fatalf("build test pdf: %v", err)
	}
	return data
}

func doJSON(t *testing.T, client *http.Client, method, url string, body io.Reader, wantStatus int, out any) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("got status %d, want %d", resp.StatusCode, wantStatus)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func applyMigration(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	path := filepath.Join("..", "..", "migrations", "0001_init_schema.up.sql")
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for migration: %v", err)
	}
	defer conn.Close(ctx)

	for _, stmt := range strings.Split(string(sqlBytes), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("apply migration statement %q: %v", stmt, err)
		}
	}
}
