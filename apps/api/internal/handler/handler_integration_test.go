//go:build integration

// Integration test against a real (embedded) Postgres instance. Excluded
// from the default `go test ./...` run; run explicitly with
// `go test -tags=integration ./internal/handler/...`.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
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

	"blog-pipeline-api/internal/handler"
	"blog-pipeline-api/internal/store"
)

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

	h := handler.New(s, t.TempDir())
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
