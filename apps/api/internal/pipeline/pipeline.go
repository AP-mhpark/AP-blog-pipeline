// Package pipeline encodes the content pipeline state machine described in
// the root CLAUDE.md (파이프라인 상태 머신). It owns the domain types and the
// legal-transition rules; persistence and HTTP wiring live in internal/store
// and internal/handler.
package pipeline

import "fmt"

// Status is a pipeline stage as recorded in posts.status. Values must match
// the CHECK constraint in apps/api/migrations/0001_init_schema.up.sql.
type Status string

const (
	StatusResearching       Status = "researching"
	StatusResearched        Status = "researched"
	StatusDrafting          Status = "drafting"
	StatusDraftReady        Status = "draft_ready"
	StatusPendingReview     Status = "pending_review"
	StatusNeedsRevision     Status = "needs_revision"
	StatusApproved          Status = "approved"
	StatusArchived          Status = "archived"
	StatusFailedFileParsing Status = "failed_file_parsing"
	StatusFailedResearching Status = "failed_researching"
	StatusFailedDrafting    Status = "failed_drafting"
)

// InputMethod is how a post's source material was provided.
type InputMethod string

const (
	InputMethodKeyword InputMethod = "keyword"
	InputMethodFile    InputMethod = "file"
)

// ContentType is the content track a post belongs to.
type ContentType string

const (
	ContentTypeInformational ContentType = "informational"
	ContentTypeExperiential  ContentType = "experiential"
)

// transitions enumerates the legal next statuses for each status, mirroring
// the state diagram in the root CLAUDE.md.
//
// failed_file_parsing is the one failed_* status allowed to loop back to
// itself: file parsing is synchronous, so there's no separate "in progress"
// status for it the way researching/drafting have for their (external API)
// steps. A retry either lands on researched or fails again in the same
// call, so the self-transition represents "retried, failed again". The
// other failed_* statuses instead retry through their in-progress status
// (researching/drafting), which already has a transition back to the
// matching failed_* status if the retry fails too.
var transitions = map[Status][]Status{
	StatusResearching:       {StatusResearched, StatusFailedResearching},
	StatusFailedResearching: {StatusResearching},
	StatusResearched:        {StatusDrafting},
	StatusFailedFileParsing: {StatusResearched, StatusFailedFileParsing},
	StatusDrafting:          {StatusDraftReady, StatusFailedDrafting},
	StatusFailedDrafting:    {StatusDrafting},
	StatusDraftReady:        {StatusPendingReview},
	StatusPendingReview:     {StatusApproved, StatusNeedsRevision},
	StatusNeedsRevision:     {StatusDrafting},
	StatusApproved:          {StatusArchived},
	// StatusArchived is terminal: no outgoing transitions.
}

// Transition reports whether moving from `from` to `to` is a legal pipeline
// transition. It returns nil if legal, or a descriptive error if not.
func Transition(from, to Status) error {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return nil
		}
	}
	return fmt.Errorf("illegal transition from %q to %q", from, to)
}

// IsValidStatus reports whether s is one of the known pipeline statuses.
func IsValidStatus(s Status) bool {
	if _, ok := transitions[s]; ok {
		return true
	}
	return s == StatusArchived
}
