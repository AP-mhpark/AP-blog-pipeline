package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"blog-pipeline-api/internal/external/llm"
	"blog-pipeline-api/internal/external/naversearch"
	"blog-pipeline-api/internal/handler"
	"blog-pipeline-api/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}

	ctx := context.Background()
	s, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer s.Close()

	llmClient := llm.NewClient(os.Getenv("ANTHROPIC_API_KEY"))
	searchClient := naversearch.NewClient(os.Getenv("NAVER_CLIENT_ID"), os.Getenv("NAVER_CLIENT_SECRET"))

	h := handler.New(s, uploadDir, llmClient, searchClient)
	mux := h.Routes()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := ":8080"
	log.Printf("api server listening on %s", addr)
	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

// withCORS reflects the request's Origin back as allowed. This API has no
// auth/cookies (single-user internal tool per root CLAUDE.md), so there's
// no cross-origin credential risk in allowing any origin — and the web app
// is deployed on a different origin than the API (root CLAUDE.md section 6),
// so CORS is needed beyond just local dev.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
