// Typed bindings for the apps/api REST API (see apps/api/CLAUDE.md).
// Field names mirror the backend's JSON tags (snake_case).
import type { ContentType } from "./categories";

export const API_BASE_URL: string =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

export type InputMethod = "keyword" | "file";

export type Status =
  | "researching"
  | "researched"
  | "drafting"
  | "draft_ready"
  | "pending_review"
  | "needs_revision"
  | "approved"
  | "archived"
  | "failed_file_parsing"
  | "failed_researching"
  | "failed_drafting";

export interface Post {
  id: string;
  content_type: ContentType;
  category: string;
  subtype: string | null;
  input_method: InputMethod;
  input_keyword: string | null;
  status: Status;
  status_error_message: string | null;
  created_at: string;
  updated_at: string;
}

export interface Draft {
  id: string;
  post_id: string;
  version: number;
  content: string;
  meta_title: string | null;
  meta_description: string | null;
  image_alts: string[] | null;
  created_at: string;
}

class ApiError extends Error {}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE_URL}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(
      typeof body?.error === "string" ? body.error : `request failed: ${res.status}`,
    );
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export function listPosts(): Promise<Post[]> {
  return request<Post[]>("/posts");
}

export function getPost(id: string): Promise<Post> {
  return request<Post>(`/posts/${id}`);
}

export function deletePost(id: string): Promise<void> {
  return request<void>(`/posts/${id}`, { method: "DELETE" });
}

export function listDrafts(postId: string): Promise<Draft[]> {
  return request<Draft[]>(`/posts/${postId}/drafts`);
}

export interface CreateKeywordPostInput {
  content_type: ContentType;
  category: string;
  subtype?: string;
  keyword: string;
}

export function createKeywordPost(input: CreateKeywordPostInput): Promise<Post> {
  return request<Post>("/posts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export interface UploadFilePostInput {
  file: File;
  contentType: ContentType;
  category: string;
  subtype?: string;
}

export function uploadFilePost(input: UploadFilePostInput): Promise<Post> {
  const form = new FormData();
  form.set("file", input.file);
  form.set("content_type", input.contentType);
  form.set("category", input.category);
  if (input.subtype) {
    form.set("subtype", input.subtype);
  }
  return request<Post>("/posts/upload", { method: "POST", body: form });
}

export function draftPost(postId: string): Promise<Draft> {
  return request<Draft>(`/posts/${postId}/draft`, { method: "POST" });
}

export function approvePost(postId: string): Promise<Post> {
  return request<Post>(`/posts/${postId}/approve`, { method: "POST" });
}

export function rejectPost(postId: string, feedbackNote?: string): Promise<Post> {
  return request<Post>(`/posts/${postId}/reject`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ feedback_note: feedbackNote }),
  });
}

export function archivePost(postId: string): Promise<Post> {
  return request<Post>(`/posts/${postId}/archive`, { method: "POST" });
}
