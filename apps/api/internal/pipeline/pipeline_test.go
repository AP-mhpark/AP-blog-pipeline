package pipeline

import "testing"

func TestTransition_Legal(t *testing.T) {
	for from, tos := range transitions {
		for _, to := range tos {
			if err := Transition(from, to); err != nil {
				t.Errorf("Transition(%q, %q) = %v, want nil", from, to, err)
			}
		}
	}
}

func TestTransition_RevisionLoop(t *testing.T) {
	steps := []struct{ from, to Status }{
		{StatusPendingReview, StatusNeedsRevision},
		{StatusNeedsRevision, StatusDrafting},
	}
	for _, s := range steps {
		if err := Transition(s.from, s.to); err != nil {
			t.Errorf("Transition(%q, %q) = %v, want nil", s.from, s.to, err)
		}
	}
}

func TestTransition_Illegal(t *testing.T) {
	illegal := []struct{ from, to Status }{
		{StatusPendingReview, StatusArchived},
		{StatusApproved, StatusDrafting},
		{StatusResearching, StatusDrafting},
		{StatusDraftReady, StatusApproved},
		{StatusNeedsRevision, StatusApproved},
		{StatusArchived, StatusResearching},
		{StatusFailedResearching, StatusDrafting},
		{StatusResearched, StatusPendingReview},
	}
	for _, c := range illegal {
		if err := Transition(c.from, c.to); err == nil {
			t.Errorf("Transition(%q, %q) = nil, want error", c.from, c.to)
		}
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []Status{
		StatusResearching, StatusResearched, StatusDrafting, StatusDraftReady,
		StatusPendingReview, StatusNeedsRevision, StatusApproved, StatusArchived,
		StatusFailedFileParsing, StatusFailedResearching, StatusFailedDrafting,
	}
	for _, s := range valid {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = false, want true", s)
		}
	}
	if IsValidStatus(Status("bogus")) {
		t.Error(`IsValidStatus("bogus") = true, want false`)
	}
}
