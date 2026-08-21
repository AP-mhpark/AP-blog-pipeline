import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { listPosts } from "../api/client";
import StatusBadge from "../components/StatusBadge";

export default function PostListPage() {
  const { data: posts, isLoading, error } = useQuery({
    queryKey: ["posts"],
    queryFn: listPosts,
  });

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1>포스트</h1>
        <Link to="/posts/new">
          <button type="button">새 글</button>
        </Link>
      </div>

      {isLoading && <p>불러오는 중...</p>}
      {error && <p style={{ color: "#b91c1c" }}>목록을 불러오지 못했습니다: {error.message}</p>}

      {posts && posts.length === 0 && <p>아직 글이 없습니다.</p>}

      {posts && posts.length > 0 && (
        <table style={{ width: "100%", borderCollapse: "collapse", marginTop: "1rem" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid #e5e7eb" }}>
              <th style={{ padding: "0.5rem" }}>카테고리</th>
              <th style={{ padding: "0.5rem" }}>입력</th>
              <th style={{ padding: "0.5rem" }}>상태</th>
              <th style={{ padding: "0.5rem" }}>생성일</th>
            </tr>
          </thead>
          <tbody>
            {posts.map((post) => (
              <tr key={post.id} style={{ borderBottom: "1px solid #f3f4f6" }}>
                <td style={{ padding: "0.5rem" }}>
                  <Link to={`/posts/${post.id}`}>
                    {post.category}
                    {post.subtype ? ` / ${post.subtype}` : ""}
                  </Link>
                  {post.input_keyword && (
                    <div style={{ fontSize: "0.85rem", color: "#6b7280" }}>{post.input_keyword}</div>
                  )}
                </td>
                <td style={{ padding: "0.5rem" }}>{post.input_method === "keyword" ? "키워드" : "파일"}</td>
                <td style={{ padding: "0.5rem" }}>
                  <StatusBadge status={post.status} />
                </td>
                <td style={{ padding: "0.5rem", color: "#6b7280" }}>
                  {new Date(post.created_at).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
