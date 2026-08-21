// Categories fixed on the frontend to keep `category`/`subtype` values
// consistent — the DB stores them as free text (no CHECK constraint, since
// subtopics are still being decided per root CLAUDE.md section 2), so the
// UI is what prevents typo-driven drift.
export type ContentType = "informational" | "experiential";

export interface Subtype {
  value: string;
  label: string;
}

export interface Category {
  value: string;
  label: string;
  contentType: ContentType;
  subtypes: Subtype[];
}

export const CATEGORIES: Category[] = [
  {
    value: "생활정보_제도안내",
    label: "생활정보/제도안내",
    contentType: "informational",
    subtypes: [{ value: "입주준비_체크리스트", label: "입주 준비/체크리스트" }],
  },
  {
    value: "소프트웨어_서비스비교",
    label: "소프트웨어/서비스 스펙 비교",
    contentType: "informational",
    subtypes: [],
  },
  {
    value: "여행",
    label: "여행",
    contentType: "experiential",
    subtypes: [],
  },
];
