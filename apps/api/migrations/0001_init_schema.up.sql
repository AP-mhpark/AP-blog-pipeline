CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE posts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  content_type TEXT NOT NULL CHECK (content_type IN ('informational','experiential')),
  category TEXT NOT NULL,
  subtype TEXT,
  input_method TEXT NOT NULL CHECK (input_method IN ('keyword','file')),
  input_keyword TEXT,
  status TEXT NOT NULL CHECK (status IN (
    'researching','researched','drafting','draft_ready',
    'pending_review','needs_revision','approved','archived',
    'failed_file_parsing','failed_researching','failed_drafting'
  )),
  status_error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE uploaded_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  original_filename TEXT NOT NULL,
  file_type TEXT NOT NULL CHECK (file_type IN ('pdf','xlsx')),
  storage_path TEXT NOT NULL,
  uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE research_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('naver_datalab','file_upload')),
  raw_data JSONB,
  extracted_text TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE drafts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  version INT NOT NULL,
  content TEXT NOT NULL,
  meta_title TEXT,
  meta_description TEXT,
  image_alts JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(post_id, version)
);

CREATE TABLE review_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  draft_id UUID NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  action TEXT NOT NULL CHECK (action IN ('approve','reject')),
  feedback_note TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE status_transitions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  from_status TEXT,
  to_status TEXT NOT NULL,
  error_message TEXT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_posts_status ON posts(status);
CREATE INDEX idx_drafts_post_id ON drafts(post_id);
CREATE INDEX idx_status_transitions_post_id ON status_transitions(post_id, occurred_at);
