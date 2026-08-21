package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// UploadedFile mirrors a row in the uploaded_files table.
type UploadedFile struct {
	ID               string    `json:"id"`
	PostID           string    `json:"post_id"`
	OriginalFilename string    `json:"original_filename"`
	FileType         string    `json:"file_type"` // "pdf" | "xlsx"
	StoragePath      string    `json:"storage_path"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

// CreateUploadedFile records an uploaded PDF/Excel file for a post.
func (s *Store) CreateUploadedFile(ctx context.Context, postID, originalFilename, fileType, storagePath string) (UploadedFile, error) {
	var f UploadedFile
	err := s.pool.QueryRow(ctx, `
		INSERT INTO uploaded_files (post_id, original_filename, file_type, storage_path)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, post_id::text, original_filename, file_type, storage_path, uploaded_at
	`, postID, originalFilename, fileType, storagePath).Scan(
		&f.ID, &f.PostID, &f.OriginalFilename, &f.FileType, &f.StoragePath, &f.UploadedAt,
	)
	if err != nil {
		return UploadedFile{}, fmt.Errorf("create uploaded file: %w", err)
	}
	return f, nil
}

// ResearchResult mirrors a row in the research_results table.
type ResearchResult struct {
	ID            string          `json:"id"`
	PostID        string          `json:"post_id"`
	Source        string          `json:"source"` // "naver_datalab" | "file_upload"
	RawData       json.RawMessage `json:"raw_data"`
	ExtractedText *string         `json:"extracted_text"`
	CreatedAt     time.Time       `json:"created_at"`
}

// CreateResearchResult records trend-research or extracted-file data for a post.
func (s *Store) CreateResearchResult(ctx context.Context, postID, source string, rawData json.RawMessage, extractedText *string) (ResearchResult, error) {
	var r ResearchResult
	err := s.pool.QueryRow(ctx, `
		INSERT INTO research_results (post_id, source, raw_data, extracted_text)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, post_id::text, source, raw_data, extracted_text, created_at
	`, postID, source, rawData, extractedText).Scan(
		&r.ID, &r.PostID, &r.Source, &r.RawData, &r.ExtractedText, &r.CreatedAt,
	)
	if err != nil {
		return ResearchResult{}, fmt.Errorf("create research result: %w", err)
	}
	return r, nil
}

// Draft mirrors a row in the drafts table.
type Draft struct {
	ID              string          `json:"id"`
	PostID          string          `json:"post_id"`
	Version         int             `json:"version"`
	Content         string          `json:"content"`
	MetaTitle       *string         `json:"meta_title"`
	MetaDescription *string         `json:"meta_description"`
	ImageAlts       json.RawMessage `json:"image_alts"`
	CreatedAt       time.Time       `json:"created_at"`
}

// CreateDraft records a generated draft (with its SEO metadata) for a post.
func (s *Store) CreateDraft(ctx context.Context, postID string, version int, content string, metaTitle, metaDescription *string, imageAlts json.RawMessage) (Draft, error) {
	var d Draft
	err := s.pool.QueryRow(ctx, `
		INSERT INTO drafts (post_id, version, content, meta_title, meta_description, image_alts)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, post_id::text, version, content, meta_title, meta_description, image_alts, created_at
	`, postID, version, content, metaTitle, metaDescription, imageAlts).Scan(
		&d.ID, &d.PostID, &d.Version, &d.Content, &d.MetaTitle, &d.MetaDescription, &d.ImageAlts, &d.CreatedAt,
	)
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	return d, nil
}

// ReviewAction mirrors a row in the review_actions table.
type ReviewAction struct {
	ID           string    `json:"id"`
	PostID       string    `json:"post_id"`
	DraftID      string    `json:"draft_id"`
	Action       string    `json:"action"` // "approve" | "reject"
	FeedbackNote *string   `json:"feedback_note"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateReviewAction records an approve/reject decision on a draft.
func (s *Store) CreateReviewAction(ctx context.Context, postID, draftID, action string, feedbackNote *string) (ReviewAction, error) {
	var r ReviewAction
	err := s.pool.QueryRow(ctx, `
		INSERT INTO review_actions (post_id, draft_id, action, feedback_note)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, post_id::text, draft_id::text, action, feedback_note, created_at
	`, postID, draftID, action, feedbackNote).Scan(
		&r.ID, &r.PostID, &r.DraftID, &r.Action, &r.FeedbackNote, &r.CreatedAt,
	)
	if err != nil {
		return ReviewAction{}, fmt.Errorf("create review action: %w", err)
	}
	return r, nil
}
