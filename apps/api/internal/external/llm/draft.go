package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

const draftToolName = "emit_draft"

// DraftInput bundles everything the drafting prompt needs.
type DraftInput struct {
	Category          string
	Subtype           string   // empty if the post has none
	ContentType       string   // "informational" | "experiential"
	SourceText        string   // extracted PDF/Excel text (from fileparser)
	ReferenceTitles   []string // top-ranking blog titles for the target keyword
	ReferenceSnippets []string // matching descriptions/snippets
}

// DraftOutput is the structured result of a drafting call.
type DraftOutput struct {
	Content         string   `json:"content"`
	MetaTitle       string   `json:"meta_title"`
	MetaDescription string   `json:"meta_description"`
	ImageAlts       []string `json:"image_alts"`
}

// GenerateDraft asks Claude to produce a blog draft from in, using
// Anthropic tool_use (forced via tool_choice) so the response always
// arrives as structured DraftOutput rather than free text to parse.
func (c *Client) GenerateDraft(ctx context.Context, in DraftInput) (DraftOutput, error) {
	tool := anthropic.ToolParam{
		Name:        draftToolName,
		Description: param.NewOpt("생성된 블로그 초안을 구조화된 필드로 제출한다."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "완성된 블로그 본문(마크다운). 자격 요건·일정·신청 방법 등 필수 정보 요약과, 누가 특히 유리한지에 대한 서술형 분석을 포함한다.",
				},
				"meta_title": map[string]any{
					"type":        "string",
					"description": "SEO용 제목, 40자 이내",
				},
				"meta_description": map[string]any{
					"type":        "string",
					"description": "SEO용 설명, 100자 이내",
				},
				"image_alts": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "본문에 들어갈 만한 이미지 2~4개에 대한 대체텍스트 제안",
				},
			},
			Required: []string{"content", "meta_title", "meta_description"},
		},
	}

	message, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     defaultModel,
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{
			{Text: draftSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildDraftUserPrompt(in))),
		},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceParamOfTool(draftToolName),
	})
	if err != nil {
		return DraftOutput{}, fmt.Errorf("generate draft: %w", err)
	}

	for _, block := range message.Content {
		if block.Type != "tool_use" {
			continue
		}
		toolUse := block.AsToolUse()
		if toolUse.Name != draftToolName {
			continue
		}
		var out DraftOutput
		if err := json.Unmarshal(toolUse.Input, &out); err != nil {
			return DraftOutput{}, fmt.Errorf("decode draft tool input: %w", err)
		}
		return out, nil
	}
	return DraftOutput{}, fmt.Errorf("generate draft: no %s tool call in response", draftToolName)
}

const draftSystemPrompt = `당신은 한국어 생활정보 블로그 글을 작성하는 전문 작가입니다.

원칙:
- 반드시 제공된 원문 자료에 근거해서만 작성하고, 원문에 없는 사실을 지어내지 않습니다.
- 독자(신청/조사를 준비하는 사람)가 실제로 알아야 할 자격 요건, 신청 일정, 신청 방법 등 필수 정보를 빠짐없이 정리합니다.
- "누가 특히 유리한지"를 원문 근거를 들어 서술형으로 분석합니다. 개인화된 계산기가 아니라 글 안의 설명이라는 점을 유지합니다.
- 함께 제공되는 "참고 상위노출 제목/스니펫"은 표절하지 말고, 문체·어조·독자가 궁금해하는 포인트를 파악하는 용도로만 참고합니다.
- 결과는 반드시 emit_draft 도구 호출로 제출합니다.`

func buildDraftUserPrompt(in DraftInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "카테고리: %s\n", in.Category)
	if in.Subtype != "" {
		fmt.Fprintf(&b, "서브타입: %s\n", in.Subtype)
	}
	fmt.Fprintf(&b, "콘텐츠 트랙: %s\n\n", in.ContentType)

	b.WriteString("=== 원문 자료 ===\n")
	b.WriteString(in.SourceText)
	b.WriteString("\n\n")

	if len(in.ReferenceTitles) > 0 {
		b.WriteString("=== 참고: 같은 주제로 현재 상위노출된 블로그 제목/스니펫 ===\n")
		for i, title := range in.ReferenceTitles {
			fmt.Fprintf(&b, "%d. %s", i+1, title)
			if i < len(in.ReferenceSnippets) && in.ReferenceSnippets[i] != "" {
				fmt.Fprintf(&b, " — %s", in.ReferenceSnippets[i])
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}
