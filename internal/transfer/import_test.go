// SPDX-License-Identifier: GPL-2.0-only
package transfer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
	"github.com/Work-Fort/Flow/internal/transfer"
)

func openStore(t *testing.T) domain.Store {
	t.Helper()
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// minimalTemplate returns a TemplateFile with a single task step and no
// transitions — the minimum valid template structure for import tests.
func minimalTemplate(name string) *transfer.TemplateFile {
	return &transfer.TemplateFile{
		SchemaVersion: "1",
		Name:          name,
		Version:       "1.0.0",
		Steps: []transfer.StepFile{
			{Key: "start", Name: "Start", Type: "task", Position: 0},
		},
	}
}

// TestImportTemplateFromFile_DuplicateName verifies that importing a template
// with the same name as an existing template returns an error containing "already exists".
func TestImportTemplateFromFile_DuplicateName(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tf := minimalTemplate("MyWorkflow")

	// First import — should succeed.
	if _, err := transfer.ImportTemplateFromFile(ctx, store, tf, nil); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import with same name — should fail.
	_, err := transfer.ImportTemplateFromFile(ctx, store, tf, nil)
	if err == nil {
		t.Fatal("want error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should contain 'already exists', got: %v", err)
	}
}

// TestImportTemplateFromFile_InvalidGuard verifies that a transition with an
// invalid CEL guard expression causes ImportTemplateFromFile to return an error.
func TestImportTemplateFromFile_InvalidGuard(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tf := &transfer.TemplateFile{
		SchemaVersion: "1",
		Name:          "GuardedWorkflow",
		Version:       "1.0.0",
		Steps: []transfer.StepFile{
			{Key: "a", Name: "A", Type: "task", Position: 0},
			{Key: "b", Name: "B", Type: "task", Position: 1},
		},
		Transitions: []transfer.TransitionFile{
			{
				Key:   "a-to-b",
				Name:  "Advance",
				From:  "a",
				To:    "b",
				Guard: "invalid ==", // syntactically invalid CEL expression
			},
		},
	}

	_, err := transfer.ImportTemplateFromFile(ctx, store, tf, nil)
	if err == nil {
		t.Fatal("want error for invalid guard expression, got nil")
	}
	if !strings.Contains(err.Error(), "invalid guard") {
		t.Errorf("error should mention 'invalid guard', got: %v", err)
	}
}

// TestImportTemplateFromFile_UnknownRejectionStep verifies that referencing a
// non-existent step key in approval.rejection_step causes an error.
func TestImportTemplateFromFile_UnknownRejectionStep(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	tf := &transfer.TemplateFile{
		SchemaVersion: "1",
		Name:          "RejectionWorkflow",
		Version:       "1.0.0",
		Steps: []transfer.StepFile{
			{
				Key:      "gate",
				Name:     "Gate",
				Type:     "gate",
				Position: 0,
				Approval: &transfer.ApprovalFile{
					Mode:              "any",
					RequiredApprovers: 1,
					ApproverRole:      "reviewer",
					RejectionStep:     "nonexistent", // this step key does not exist
				},
			},
		},
	}

	_, err := transfer.ImportTemplateFromFile(ctx, store, tf, nil)
	if err == nil {
		t.Fatal("want error for unknown rejection step, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the unknown step key 'nonexistent', got: %v", err)
	}
}
