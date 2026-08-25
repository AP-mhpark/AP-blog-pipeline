import { useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { extractedImageUrl, type Draft } from "../api/client";

type CopiedField = "title" | "description" | "body" | null;

// react-markdown renders no raw HTML by default (no rehype-raw plugin), so
// this is safe even though `content` comes straight from the LLM.
//
// showCopyButton controls the "본문 복사" rich-text-to-clipboard button —
// callers should only enable it for approved posts (see PostDetailPage):
// copying is a paste-into-Naver convenience, not a way around the
// approve-before-anything-leaves-the-tool rule.
export default function DraftView({
  draft,
  showCopyButton = false,
}: {
  draft: Draft;
  showCopyButton?: boolean;
}) {
  const bodyRef = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState<CopiedField>(null);
  const [copyError, setCopyError] = useState<string | null>(null);

  function flashCopied(field: CopiedField) {
    setCopyError(null);
    setCopied(field);
    setTimeout(() => setCopied((c) => (c === field ? null : c)), 2000);
  }

  async function copyPlainText(field: CopiedField, text: string) {
    try {
      await navigator.clipboard.writeText(text);
      flashCopied(field);
    } catch (err) {
      setCopyError((err as Error).message);
    }
  }

  async function copyBodyRichText() {
    const el = bodyRef.current;
    if (!el) return;
    try {
      await navigator.clipboard.write([
        new ClipboardItem({
          "text/html": new Blob([el.innerHTML], { type: "text/html" }),
          "text/plain": new Blob([el.innerText], { type: "text/plain" }),
        }),
      ]);
      flashCopied("body");
    } catch (err) {
      setCopyError((err as Error).message);
    }
  }

  return (
    <div style={{ border: "1px solid var(--border)", borderRadius: 8, padding: "1rem" }}>
      <div style={{ fontSize: "0.85rem", color: "#6b7280", marginBottom: "0.5rem" }}>
        버전 {draft.version} · {new Date(draft.created_at).toLocaleString()}
      </div>

      {draft.meta_title && (
        <p>
          <strong>SEO 제목:</strong> {draft.meta_title}
          {showCopyButton && (
            <button type="button" onClick={() => copyPlainText("title", draft.meta_title!)} style={{ marginLeft: "0.5rem" }}>
              {copied === "title" ? "복사됨" : "복사"}
            </button>
          )}
        </p>
      )}
      {draft.meta_description && (
        <p>
          <strong>SEO 설명:</strong> {draft.meta_description}
          {showCopyButton && (
            <button
              type="button"
              onClick={() => copyPlainText("description", draft.meta_description!)}
              style={{ marginLeft: "0.5rem" }}
            >
              {copied === "description" ? "복사됨" : "복사"}
            </button>
          )}
        </p>
      )}

      {showCopyButton && (
        <div style={{ marginTop: "0.75rem" }}>
          <button type="button" onClick={copyBodyRichText}>
            {copied === "body" ? "본문 복사됨" : "본문 복사 (네이버 에디터용 서식 유지)"}
          </button>
          {copyError && <span style={{ color: "#b91c1c", marginLeft: "0.5rem" }}>복사 실패: {copyError}</span>}
        </div>
      )}

      <div ref={bodyRef} className="markdown-body" style={{ marginTop: "0.75rem" }}>
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          components={{
            // content references extracted images by bare filename, not a
            // full URL — resolve against the API's static upload serving.
            img: ({ src, alt }) => (
              <img src={extractedImageUrl(String(src))} alt={alt} style={{ maxWidth: "100%" }} />
            ),
          }}
        >
          {draft.content}
        </ReactMarkdown>
      </div>
    </div>
  );
}
