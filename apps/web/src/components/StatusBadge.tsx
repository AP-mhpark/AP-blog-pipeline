import type { Status } from "../api/client";

const LABELS: Record<Status, string> = {
  researching: "조사 중",
  researched: "조사 완료",
  drafting: "초안 생성 중",
  draft_ready: "초안 준비됨",
  pending_review: "검토 대기",
  needs_revision: "반려됨",
  approved: "승인됨",
  archived: "보관됨",
  failed_file_parsing: "파일 파싱 실패",
  failed_researching: "조사 실패",
  failed_drafting: "초안 생성 실패",
};

const COLORS: Record<Status, { bg: string; fg: string }> = {
  researching: { bg: "#e0ecff", fg: "#1d4ed8" },
  researched: { bg: "#e0ecff", fg: "#1d4ed8" },
  drafting: { bg: "#e0ecff", fg: "#1d4ed8" },
  draft_ready: { bg: "#e0ecff", fg: "#1d4ed8" },
  pending_review: { bg: "#fef3c7", fg: "#b45309" },
  needs_revision: { bg: "#fef3c7", fg: "#b45309" },
  approved: { bg: "#dcfce7", fg: "#15803d" },
  archived: { bg: "#e5e7eb", fg: "#374151" },
  failed_file_parsing: { bg: "#fee2e2", fg: "#b91c1c" },
  failed_researching: { bg: "#fee2e2", fg: "#b91c1c" },
  failed_drafting: { bg: "#fee2e2", fg: "#b91c1c" },
};

export default function StatusBadge({ status }: { status: Status }) {
  const color = COLORS[status];
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 10px",
        borderRadius: "999px",
        fontSize: "0.8rem",
        fontWeight: 600,
        backgroundColor: color.bg,
        color: color.fg,
        whiteSpace: "nowrap",
      }}
    >
      {LABELS[status]}
    </span>
  );
}
