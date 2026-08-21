import { extractedImageUrl, type Draft } from "../api/client";

// Display-only. No markdown rendering (yet) — content is shown as
// pre-formatted text rather than pulling in a markdown dependency for this,
// so ![alt](filename) references inside content show as literal text.
// used_images is rendered separately as an actual image gallery below so
// the images are at least visible somewhere.
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
      {draft.used_images && draft.used_images.length > 0 && (
        <div>
          <strong>원문에서 추출해 사용한 이미지:</strong>
          <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", marginTop: "0.5rem" }}>
            {draft.used_images.map((filename) => (
              <img
                key={filename}
                src={extractedImageUrl(filename)}
                alt={filename}
                style={{ maxWidth: 200, maxHeight: 200, border: "1px solid var(--border)", borderRadius: 4 }}
              />
            ))}
          </div>
        </div>
      )}

      <pre
        style={{
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          fontFamily: "inherit",
          marginTop: "0.75rem",
        }}
      >
        {draft.content}
      </pre>
    </div>
  );
}
