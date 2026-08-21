//go:build integration

// Integration test against a real (embedded) Postgres instance. Excluded
// from the default `go test ./...` run since it downloads/boots a Postgres
// binary; run explicitly with `go test -tags=integration ./internal/store/...`.
package store_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"

	"blog-pipeline-api/internal/pipeline"
	"blog-pipeline-api/internal/store"
)

const integrationPort = 15432

func TestStoreIntegration(t *testing.T) {
	dsn := "postgres://postgres:postgres@localhost:15432/postgres?sslmode=disable"
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

	keyword := "청약 자격 요건 정리"
	post, err := s.CreatePost(ctx, store.CreatePostParams{
		ContentType:  pipeline.ContentTypeInformational,
		Category:     "생활정보_제도안내",
		InputMethod:  pipeline.InputMethodKeyword,
		InputKeyword: &keyword,
		Status:       pipeline.StatusResearching,
	})
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.ID == "" {
		t.Fatal("expected a generated ID")
	}
	if post.Status != pipeline.StatusResearching {
		t.Fatalf("got status %q, want %q", post.Status, pipeline.StatusResearching)
	}

	got, err := s.GetPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got.ID != post.ID {
		t.Fatalf("got ID %q, want %q", got.ID, post.ID)
	}

	if _, err := s.GetPost(ctx, "00000000-0000-0000-0000-000000000000"); err != store.ErrNotFound {
		t.Fatalf("GetPost(missing) = %v, want ErrNotFound", err)
	}

	if err := s.UpdateStatus(ctx, post.ID, pipeline.StatusResearched, nil); err != nil {
		t.Fatalf("UpdateStatus researching->researched: %v", err)
	}

	if err := s.UpdateStatus(ctx, post.ID, pipeline.StatusApproved, nil); err == nil {
		t.Fatal("expected researched->approved to be rejected")
	}

	posts, err := s.ListPosts(ctx)
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("got %d posts, want 1", len(posts))
	}
	if posts[0].Status != pipeline.StatusResearched {
		t.Fatalf("got status %q, want %q", posts[0].Status, pipeline.StatusResearched)
	}
}

// applyMigration executes apps/api/migrations/0001_init_schema.up.sql
// statement-by-statement (pgx's extended protocol doesn't support multiple
// statements per Exec call).
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
