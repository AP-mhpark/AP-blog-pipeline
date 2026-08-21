import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import {
  approvePost,
  archivePost,
  draftPost,
  getPost,
  listDrafts,
  rejectPost,
} from "../api/client";
import StatusBadge from "../components/StatusBadge";
import DraftView from "../components/DraftView";

export default function PostDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const [feedbackNote, setFeedbackNote] = useState("");

  const {
    data: post,
    isLoading: postLoading,
    error: postError,
  } = useQuery({
    queryKey: ["posts", id],
    queryFn: () => getPost(id!),
    enabled: Boolean(id),
  });

  const { data: drafts } = useQuery({
    queryKey: ["posts", id, "drafts"],
    queryFn: () => listDrafts(id!),
    enabled: Boolean(id),
  });

  function invalidateAll() {
    queryClient.invalidateQueries({ queryKey: ["posts"] });
    queryClient.invalidateQueries({ queryKey: ["posts", id] });
    queryClient.invalidateQueries({ queryKey: ["posts", id, "drafts"] });
  }

  const draftMutation = useMutation({
    mutationFn: () => draftPost(id!),
    onSuccess: invalidateAll,
  });
  const approveMutation = useMutation({
    mutationFn: () => approvePost(id!),
    onSuccess: invalidateAll,
  });
  const rejectMutation = useMutation({
    mutationFn: () => rejectPost(id!, feedbackNote || undefined),
    onSuccess: () => {
      setFeedbackNote("");
      invalidateAll();
    },
  });
  const archiveMutation = useMutation({
    mutationFn: () => archivePost(id!),
    onSuccess: invalidateAll,
  });

  const latestDraft = drafts?.[0];
  const olderDrafts = drafts?.slice(1) ?? [];

  return (
    <div>
      <Link to="/">← 목록으로</Link>

      {postLoading && <p>불러오는 중...</p>}
      {postError && <p style={{ color: "#b91c1c" }}>글을 불러오지 못했습니다: {postError.message}</p>}

      {post && (
        <div style={{ marginTop: "1rem" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
            <h1 style={{ margin: 0 }}>
              {post.category}
              {post.subtype ? ` / ${post.subtype}` : ""}
            </h1>
            <StatusBadge status={post.status} />
          </div>

          <dl>
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
          </dl>

          <div style={{ marginTop: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
            {(post.status === "researched" || post.status === "needs_revision" || post.status === "failed_drafting") && (
              <div>
                <button type="button" onClick={() => draftMutation.mutate()} disabled={draftMutation.isPending}>
                  {draftMutation.isPending
                    ? "생성 중..."
                    : post.status === "researched"
                      ? "초안 생성"
                      : "다시 생성"}
                </button>
                {draftMutation.isError && (
                  <p style={{ color: "#b91c1c" }}>{(draftMutation.error as Error).message}</p>
                )}
              </div>
            )}

            {(post.status === "drafting" || post.status === "draft_ready") && <p>처리 중...</p>}

            {post.status === "pending_review" && (
              <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", maxWidth: 480 }}>
                <button type="button" onClick={() => approveMutation.mutate()} disabled={approveMutation.isPending}>
                  {approveMutation.isPending ? "처리 중..." : "승인"}
                </button>
                {approveMutation.isError && (
                  <p style={{ color: "#b91c1c" }}>{(approveMutation.error as Error).message}</p>
                )}

                <textarea
                  placeholder="반려 사유 (선택)"
                  value={feedbackNote}
                  onChange={(e) => setFeedbackNote(e.target.value)}
                  rows={3}
                />
                <button type="button" onClick={() => rejectMutation.mutate()} disabled={rejectMutation.isPending}>
                  {rejectMutation.isPending ? "처리 중..." : "반려"}
                </button>
                {rejectMutation.isError && (
                  <p style={{ color: "#b91c1c" }}>{(rejectMutation.error as Error).message}</p>
                )}
              </div>
            )}

            {post.status === "approved" && (
              <div>
                <p style={{ color: "#6b7280" }}>
                  쇼핑 커넥트 링크 삽입과 발행은 네이버 블로그 에디터에서 직접 진행하세요. 아래 내용을 복사해
                  붙여넣은 뒤, 업로드가 끝나면 보관 처리해주세요.
                </p>
                <button type="button" onClick={() => archiveMutation.mutate()} disabled={archiveMutation.isPending}>
                  {archiveMutation.isPending ? "처리 중..." : "보관 처리 (수동 업로드 완료)"}
                </button>
                {archiveMutation.isError && (
                  <p style={{ color: "#b91c1c" }}>{(archiveMutation.error as Error).message}</p>
                )}
              </div>
            )}
          </div>

          {latestDraft && (
            <div style={{ marginTop: "1.5rem" }}>
              <h2>초안</h2>
              <DraftView draft={latestDraft} />
            </div>
          )}

          {olderDrafts.length > 0 && (
            <details style={{ marginTop: "1rem" }}>
              <summary>이전 버전 ({olderDrafts.length})</summary>
              <div style={{ display: "flex", flexDirection: "column", gap: "1rem", marginTop: "0.75rem" }}>
                {olderDrafts.map((d) => (
                  <DraftView key={d.id} draft={d} />
                ))}
              </div>
            </details>
          )}
        </div>
      )}
    </div>
  );
}
