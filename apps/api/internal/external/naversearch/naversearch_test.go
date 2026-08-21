package naversearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c := NewClient("test-id", "test-secret")
	c.baseURL = server.URL
	return c
}

func TestSearchBlogs_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"items": [
				{
					"title": "2026년 <b>청약</b> 자격 &quot;총정리&quot;",
					"link": "https://blog.example.com/1",
					"description": "무주택 <b>세대주</b> 기준 정리",
					"bloggername": "블로그A",
					"postdate": "20260810"
				}
			]
		}`))
	})

	results, err := c.SearchBlogs(context.Background(), "청약 자격", 10)
	if err != nil {
		t.Fatalf("SearchBlogs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	got := results[0]
	if got.Title != `2026년 청약 자격 "총정리"` {
		t.Errorf("title not cleaned, got %q", got.Title)
	}
	if got.Description != "무주택 세대주 기준 정리" {
		t.Errorf("description not cleaned, got %q", got.Description)
	}
	if got.Link != "https://blog.example.com/1" {
		t.Errorf("got link %q", got.Link)
	}
}

func TestSearchBlogs_SendsAuthHeaders(t *testing.T) {
	var gotID, gotSecret string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Naver-Client-Id")
		gotSecret = r.Header.Get("X-Naver-Client-Secret")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": []}`))
	})

	if _, err := c.SearchBlogs(context.Background(), "query", 10); err != nil {
		t.Fatalf("SearchBlogs: %v", err)
	}
	if gotID != "test-id" {
		t.Errorf("got client id header %q, want test-id", gotID)
	}
	if gotSecret != "test-secret" {
		t.Errorf("got client secret header %q, want test-secret", gotSecret)
	}
}

func TestSearchBlogs_NonOKStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.SearchBlogs(context.Background(), "query", 10)
	if err == nil {
		t.Fatal("expected an error for non-200 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention status code, got: %v", err)
	}
}

func TestSearchBlogs_Timeout(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
		w.Write([]byte(`{"items": []}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := c.SearchBlogs(ctx, "query", 10)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
}
