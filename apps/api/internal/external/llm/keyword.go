package llm

import (
	"context"
	"fmt"
	"strings"
)

const extractKeywordSystemPrompt = `주어진 원문을 읽고, 네이버 블로그 검색에 쓸 핵심 키워드 구문을 딱 한 줄로만 출력한다.
설명·따옴표·마침표 없이 검색어 자체만 출력한다. 예: 성남금토 A-4블록 신혼희망타운 행복주택 청약`

// ExtractKeyword asks Claude for a single search-style keyword phrase
// summarizing sourceText, for use as a Naver search query when a post has
// no explicit keyword (file_input). Uses the smaller/cheaper model since
// this is a low-stakes auxiliary call, not the main drafting step.
func (c *Client) ExtractKeyword(ctx context.Context, sourceText string) (string, error) {
	out, err := c.generateText(ctx, smallModel, extractKeywordSystemPrompt, sourceText, 64)
	if err != nil {
		return "", fmt.Errorf("extract keyword: %w", err)
	}
	keyword := strings.TrimSpace(out)
	if keyword == "" {
		return "", fmt.Errorf("extract keyword: empty result")
	}
	return keyword, nil
}
