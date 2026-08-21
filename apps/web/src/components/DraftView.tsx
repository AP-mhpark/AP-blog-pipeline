import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { extractedImageUrl, type Draft } from "../api/client";

// react-markdown renders no raw HTML by default (no rehype-raw plugin), so
// this is safe even though `content` comes straight from the LLM.
export default function DraftView({ draft }: { draft: Draft }) {
  return (
    <div style={{ border: "1px solid var(--border)", borderRadius: 8, padding: "1rem" }}>
      <div style={{ fontSize: "0.85rem", color: "#6b7280", marginBottom: "0.5rem" }}>
        버전 {draft.version} · {new Date(draft.created_at).toLocaleString()}
      </div>

      {draft.meta_title && (
        <p>
          <strong>SEO 제목:</strong> {draft.meta_title}
        </p>
      )}
      {draft.meta_description && (
        <p>
          <strong>SEO 설명:</strong> {draft.meta_description}
        </p>
      )}

      <div className="markdown-body" style={{ marginTop: "0.75rem" }}>
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
