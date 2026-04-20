// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func intPtr(n int) *int { return &n }

func TestProject_RetentionDays_RoundTrip(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	vocID := newID("voc")
	_ = s.CreateVocabulary(ctx, &domain.Vocabulary{ID: vocID, Name: "sdlc"})

	// Create project with nil retention (permanent).
	p := &domain.Project{
		ID:           newID("prj"),
		Name:         "ret-test",
		ChannelName:  "#ret-test",
		VocabularyID: vocID,
	}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	got, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.RetentionDays != nil {
		t.Errorf("new project: want nil retention, got %v", got.RetentionDays)
	}

	// PATCH: set retention_days = 30.
	got.RetentionDays = intPtr(30)
	if err := s.UpdateProject(ctx, got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	got2, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject after update: %v", err)
	}
	if got2.RetentionDays == nil || *got2.RetentionDays != 30 {
		t.Errorf("after set 30: want retention=30, got %v", got2.RetentionDays)
	}

	// PATCH: clear retention (set to nil = permanent).
	got2.RetentionDays = nil
	if err := s.UpdateProject(ctx, got2); err != nil {
		t.Fatalf("UpdateProject clear: %v", err)
	}
	got3, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProject after clear: %v", err)
	}
	if got3.RetentionDays != nil {
		t.Errorf("after clear: want nil retention, got %v", got3.RetentionDays)
	}
}
