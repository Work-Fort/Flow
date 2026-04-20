// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func TestInstance_ProjectBinding_RoundTrip(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	vocID := newID("voc")
	_ = s.CreateVocabulary(ctx, &domain.Vocabulary{ID: vocID, Name: "sdlc-inst"})

	prjID := newID("prj")
	_ = s.CreateProject(ctx, &domain.Project{
		ID: prjID, Name: "inst-prj", ChannelName: "#inst-prj", VocabularyID: vocID,
	})

	tplID := newID("tpl")
	_ = s.CreateTemplate(ctx, &domain.WorkflowTemplate{ID: tplID, Name: "tpl"})

	// Create instance bound to project.
	inst := &domain.WorkflowInstance{
		ID:         newID("wi"),
		TemplateID: tplID,
		TeamID:     "team-1",
		Name:       "inst-1",
		ProjectID:  prjID,
	}
	if err := s.CreateInstance(ctx, inst); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	// GetInstance returns project_id.
	got, err := s.GetInstance(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.ProjectID != prjID {
		t.Errorf("GetInstance: want project_id=%q, got %q", prjID, got.ProjectID)
	}

	// ListInstancesByProject returns just the bound instance.
	list, err := s.ListInstancesByProject(ctx, prjID)
	if err != nil {
		t.Fatalf("ListInstancesByProject: %v", err)
	}
	if len(list) != 1 || list[0].ID != inst.ID {
		t.Errorf("ListInstancesByProject: want 1 result, got %d", len(list))
	}

	// Unbound instance is not returned.
	unbound := &domain.WorkflowInstance{
		ID:         newID("wi"),
		TemplateID: tplID,
		TeamID:     "team-1",
		Name:       "inst-unbound",
	}
	_ = s.CreateInstance(ctx, unbound)
	list2, err := s.ListInstancesByProject(ctx, prjID)
	if err != nil {
		t.Fatalf("ListInstancesByProject (2): %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("expected 1 instance for project, got %d", len(list2))
	}
}
