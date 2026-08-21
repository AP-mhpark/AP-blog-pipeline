import type { Draft } from "../api/client";

// Display-only. No markdown rendering (yet) — content is shown as
// pre-formatted text rather than pulling in a markdown dependency for this.
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
      {draft.image_alts && draft.image_alts.length > 0 && (
        <div>
          <strong>이미지 대체텍스트 제안:</strong>
          <ul>
            {draft.image_alts.map((alt, i) => (
              <li key={i}>{alt}</li>
            ))}
          </ul>
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
