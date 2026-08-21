import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { getPost } from "../api/client";
import StatusBadge from "../components/StatusBadge";

// Minimal for now: metadata + status only. Draft content and review
// actions (approve/reject/archive/retry) land in a follow-up branch.
export default function PostDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: post, isLoading, error } = useQuery({
    queryKey: ["posts", id],
    queryFn: () => getPost(id!),
    enabled: Boolean(id),
  });

  return (
    <div>
      <Link to="/">← 목록으로</Link>

      {isLoading && <p>불러오는 중...</p>}
      {error && <p style={{ color: "#b91c1c" }}>글을 불러오지 못했습니다: {error.message}</p>}

      {post && (
        <div style={{ marginTop: "1rem" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
            <h1 style={{ margin: 0 }}>
              {post.category}
              {post.subtype ? ` / ${post.subtype}` : ""}
            </h1>
            <StatusBadge status={post.status} />
          </div>

          <dl style={{ marginTop: "1rem" }}>
            <dt style={{ fontWeight: 600 }}>콘텐츠 트랙</dt>
            <dd>{post.content_type === "informational" ? "정보형" : "경험형"}</dd>

            <dt style={{ fontWeight: 600, marginTop: "0.5rem" }}>입력 방식</dt>
            <dd>{post.input_method === "keyword" ? `키워드: ${post.input_keyword}` : "파일 업로드"}</dd>

            {post.status_error_message && (
              <>
                <dt style={{ fontWeight: 600, marginTop: "0.5rem", color: "#b91c1c" }}>에러</dt>
                <dd style={{ color: "#b91c1c" }}>{post.status_error_message}</dd>
              </>
            )}

            <dt style={{ fontWeight: 600, marginTop: "0.5rem" }}>생성일</dt>
            <dd>{new Date(post.created_at).toLocaleString()}</dd>
          </dl>
        </div>
      )}
    </div>
  );
}
