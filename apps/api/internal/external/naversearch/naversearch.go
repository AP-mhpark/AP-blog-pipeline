// Package naversearch wraps the Naver blog search API
// (https://openapi.naver.com/v1/search/blog.json), used to fetch
// currently top-ranking blog titles/snippets for a keyword so drafting can
// use them as reference for title/tone. It does not fetch full post
// content — see root CLAUDE.md for why that's out of scope.
package naversearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://openapi.naver.com"

// Client queries the Naver blog search API.
type Client struct {
	clientID     string
	clientSecret string
	baseURL      string // overridable in tests to point at an httptest server
	httpClient   *http.Client
}

// NewClient creates a Client using the given Naver Open API credentials
// (shared with internal/external/naverdatalab — see NAVER_CLIENT_ID/SECRET
// in .env.example).
func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      defaultBaseURL,
		httpClient:   &http.Client{},
	}
}

// BlogResult is one search hit, with title/description HTML-cleaned.
type BlogResult struct {
	Title       string
	Link        string
	Description string
	BloggerName string
	PostDate    string
}

type blogSearchResponse struct {
	Items []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Description string `json:"description"`
		BloggerName string `json:"bloggername"`
		PostDate    string `json:"postdate"`
	} `json:"items"`
}

// SearchBlogs returns up to `display` blog search results for query,
// ordered by Naver's relevance ranking (its default sort). display is
// clamped to the API's supported range (1-100).
func (c *Client) SearchBlogs(ctx context.Context, query string, display int) ([]BlogResult, error) {
	if display < 1 {
		display = 1
	}
	if display > 100 {
		display = 100
	}

	reqURL := c.baseURL + "/v1/search/blog.json?" + url.Values{
		"query":   {query},
		"display": {strconv.Itoa(display)},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Naver-Client-Id", c.clientID)
	req.Header.Set("X-Naver-Client-Secret", c.clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search blogs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("naver search API returned status %d", resp.StatusCode)
	}

	var parsed blogSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]BlogResult, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		results = append(results, BlogResult{
			Title:       cleanText(item.Title),
			Link:        item.Link,
			Description: cleanText(item.Description),
			BloggerName: item.BloggerName,
			PostDate:    item.PostDate,
		})
	}
	return results, nil
}

// cleanText strips the <b>/</b> tags Naver wraps matched query terms in and
// unescapes HTML entities (e.g. &quot;) in title/description fields.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "<b>", "")
	s = strings.ReplaceAll(s, "</b>", "")
	return html.UnescapeString(s)
}
