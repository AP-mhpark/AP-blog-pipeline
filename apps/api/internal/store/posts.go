package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"blog-pipeline-api/internal/pipeline"
)

// Post mirrors a row in the posts table.
type Post struct {
	ID                   string
	ContentType          pipeline.ContentType
	Category             string
	Subtype              *string
	InputMethod          pipeline.InputMethod
	InputKeyword         *string
	Status               pipeline.Status
	StatusErrorMessage   *string
	CreatedAt, UpdatedAt time.Time
}

// CreatePostParams are the fields needed to create a post. Status is set by
// the caller rather than derived here: keyword input starts at
// pipeline.StatusResearching, file input starts at pipeline.StatusResearched
// or pipeline.StatusFailedFileParsing depending on whether parsing
// succeeded — that decision belongs to the handler/pipeline layer, not the
// store.
type CreatePostParams struct {
	ContentType  pipeline.ContentType
	Category     string
	Subtype      *string
	InputMethod  pipeline.InputMethod
	InputKeyword *string
	Status       pipeline.Status
}

// CreatePost inserts a new post and returns the stored row.
func (s *Store) CreatePost(ctx context.Context, p CreatePostParams) (Post, error) {
	var post Post
	err := s.pool.QueryRow(ctx, `
		INSERT INTO posts (content_type, category, subtype, input_method, input_keyword, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, content_type, category, subtype, input_method, input_keyword,
		          status, status_error_message, created_at, updated_at
	`, p.ContentType, p.Category, p.Subtype, p.InputMethod, p.InputKeyword, p.Status).Scan(
		&post.ID, &post.ContentType, &post.Category, &post.Subtype, &post.InputMethod, &post.InputKeyword,
		&post.Status, &post.StatusErrorMessage, &post.CreatedAt, &post.UpdatedAt,
	)
	if err != nil {
		return Post{}, fmt.Errorf("create post: %w", err)
	}
	return post, nil
}

// GetPost fetches a post by ID. Returns ErrNotFound if no such post exists.
func (s *Store) GetPost(ctx context.Context, id string) (Post, error) {
	var post Post
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, content_type, category, subtype, input_method, input_keyword,
		       status, status_error_message, created_at, updated_at
		FROM posts WHERE id = $1
	`, id).Scan(
		&post.ID, &post.ContentType, &post.Category, &post.Subtype, &post.InputMethod, &post.InputKeyword,
		&post.Status, &post.StatusErrorMessage, &post.CreatedAt, &post.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Post{}, ErrNotFound
		}
		return Post{}, fmt.Errorf("get post: %w", err)
	}
	return post, nil
}

// ListPosts returns all posts, most recently created first.
func (s *Store) ListPosts(ctx context.Context) ([]Post, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, content_type, category, subtype, input_method, input_keyword,
		       status, status_error_message, created_at, updated_at
		FROM posts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var post Post
		if err := rows.Scan(
			&post.ID, &post.ContentType, &post.Category, &post.Subtype, &post.InputMethod, &post.InputKeyword,
			&post.Status, &post.StatusErrorMessage, &post.CreatedAt, &post.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	return posts, nil
}

// UpdateStatus moves a post to newStatus, validating the move against
// pipeline.Transition and recording it in status_transitions for audit,
// atomically. errMsg is stored alongside a failed_* status and cleared
// otherwise.
func (s *Store) UpdateStatus(ctx context.Context, id string, newStatus pipeline.Status, errMsg *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus pipeline.Status
	err = tx.QueryRow(ctx, `SELECT status FROM posts WHERE id = $1 FOR UPDATE`, id).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("select current status: %w", err)
	}

	if err := pipeline.Transition(currentStatus, newStatus); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE posts SET status = $1, status_error_message = $2, updated_at = now() WHERE id = $3
	`, newStatus, errMsg, id); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO status_transitions (post_id, from_status, to_status, error_message)
		VALUES ($1, $2, $3, $4)
	`, id, currentStatus, newStatus, errMsg); err != nil {
		return fmt.Errorf("insert status transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
