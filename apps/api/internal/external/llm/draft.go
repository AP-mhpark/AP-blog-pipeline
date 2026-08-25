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
	Images            []DraftImage
}

// DraftImage is an extracted image sent as vision input so Claude can judge
// relevance by actually seeing it, rather than guessing from a filename.
type DraftImage struct {
	Filename  string
	MediaType string // "image/png" | "image/jpeg"
	Data      string // base64-encoded bytes
}

// DraftOutput is the structured result of a drafting call.
type DraftOutput struct {
	Content         string   `json:"content"`
	MetaTitle       string   `json:"meta_title"`
	MetaDescription string   `json:"meta_description"`
	UsedImages      []string `json:"used_images"` // subset of DraftInput.Images actually referenced in Content
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
					"description": "완성된 블로그 본문(마크다운). 자격 요건·일정·신청 방법 등 필수 정보 요약과, 누가 특히 유리한지에 대한 서술형 분석을 포함한다. 표 형태 정보는 마크다운 표 문법으로 작성하고, 제공된 이미지 중 본문과 관련 있는 것은 본문 안에 ![대체텍스트](파일명) 형식으로 삽입한다.",
				},
				"meta_title": map[string]any{
					"type":        "string",
					"description": "SEO용 제목, 40자 이내",
				},
				"meta_description": map[string]any{
					"type":        "string",
					"description": "SEO용 설명, 100자 이내",
				},
				"used_images": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "content 안에서 실제로 ![]() 형식으로 참조한 이미지 파일명 목록. 제공된 이미지 목록에 없는 파일명은 절대 포함하지 않는다. 관련 이미지가 없으면 빈 배열.",
				},
			},
			Required: []string{"content", "meta_title", "meta_description"},
		},
	}

	var blocks []anthropic.ContentBlockParamUnion
	for _, img := range in.Images {
		blocks = append(blocks,
			anthropic.NewTextBlock(fmt.Sprintf("파일명: %s", img.Filename)),
			anthropic.NewImageBlockBase64(img.MediaType, img.Data),
		)
	}
	blocks = append(blocks, anthropic.NewTextBlock(buildDraftUserPrompt(in)))

	message, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     defaultModel,
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{
			{Text: draftSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(blocks...),
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
- 소득기준표·일정표처럼 표로 정리하는 게 나은 정보는 마크다운 표 문법으로 작성합니다. 표를 이미지로 대체하지 않습니다.
- 이미지는 새로 만들어내지 않습니다. 메시지에 원문 PDF에서 추출된 이미지가 실제로 첨부되어 있다면, 그 이미지들을 직접 보고 본문과 실제로 관련 있는 것만(예: 위치를 보여주는 지도, 실사진) 선택합니다. 장식용 아이콘·화살표·불릿 같은 이미지는 제외합니다. 삽입할 때는 각 이미지 직전에 표시된 "파일명: ..." 텍스트의 파일명을 그대로 써서 ![대체텍스트](파일명) 형식으로 본문에 넣습니다. 그 파일명이 아닌 다른 파일명을 지어내지 않고, 관련 있는 이미지가 없으면 삽입하지 않습니다.
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
		b.WriteString("\n")
	}

	if len(in.Images) > 0 {
		b.WriteString("=== 첨부 이미지 안내 ===\n")
		b.WriteString("위 메시지에 원문 PDF에서 추출된 이미지들이 실제로 첨부되어 있습니다. 각 이미지 바로 앞의 \"파일명: ...\" 텍스트가 그 이미지의 정확한 파일명입니다. 본문과 실제로 관련 있는 이미지만 그 파일명 그대로 ![대체텍스트](파일명) 형식으로 삽입하세요.\n\n")
	}

	return b.String()
}
