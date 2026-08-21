import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { createKeywordPost, uploadFilePost } from "../api/client";
import { CATEGORIES } from "../api/categories";

type InputMode = "keyword" | "file";

export default function NewPostPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [mode, setMode] = useState<InputMode>("keyword");
  const [categoryValue, setCategoryValue] = useState(CATEGORIES[0].value);
  const [subtypeValue, setSubtypeValue] = useState("");
  const [keyword, setKeyword] = useState("");
  const [file, setFile] = useState<File | null>(null);

  const category = CATEGORIES.find((c) => c.value === categoryValue) ?? CATEGORIES[0];

  const mutation = useMutation({
    mutationFn: async () => {
      if (mode === "keyword") {
        return createKeywordPost({
          content_type: category.contentType,
          category: category.value,
          subtype: subtypeValue || undefined,
          keyword,
        });
      }
      if (!file) {
        throw new Error("파일을 선택해 주세요.");
      }
      return uploadFilePost({
        file,
        contentType: category.contentType,
        category: category.value,
        subtype: subtypeValue || undefined,
      });
    },
    onSuccess: (post) => {
      queryClient.invalidateQueries({ queryKey: ["posts"] });
      navigate(`/posts/${post.id}`);
    },
  });

  function handleCategoryChange(value: string) {
    setCategoryValue(value);
    setSubtypeValue("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    mutation.mutate();
  }

  return (
    <div>
      <h1>새 글</h1>

      <div style={{ display: "flex", gap: "0.5rem", marginBottom: "1rem" }}>
        <button
          type="button"
          onClick={() => setMode("keyword")}
          disabled={mode === "keyword"}
        >
          키워드로 시작
        </button>
        <button type="button" onClick={() => setMode("file")} disabled={mode === "file"}>
          파일 업로드
        </button>
      </div>

      <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: "0.75rem", maxWidth: 480 }}>
        <label>
          카테고리
          <select value={categoryValue} onChange={(e) => handleCategoryChange(e.target.value)}>
            {CATEGORIES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </label>

        {category.subtypes.length > 0 && (
          <label>
            서브타입 (선택)
            <select value={subtypeValue} onChange={(e) => setSubtypeValue(e.target.value)}>
              <option value="">없음</option>
              {category.subtypes.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>
        )}

        {mode === "keyword" ? (
          <label>
            키워드
            <input
              type="text"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              placeholder="예: 2026년 청년 전세자금대출 조건"
              required
            />
          </label>
        ) : (
          <label>
            파일 (PDF/엑셀)
            <input
              type="file"
              accept=".pdf,.xlsx"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              required
            />
          </label>
        )}

        <button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "생성 중..." : "만들기"}
        </button>

        {mutation.isError && (
          <p style={{ color: "#b91c1c" }}>{(mutation.error as Error).message}</p>
        )}
      </form>
    </div>
  );
}
