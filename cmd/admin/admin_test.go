// SPDX-License-Identifier: GPL-2.0-only
package admin_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/cmd/admin"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func TestSeedVocabularies_Idempotent(t *testing.T) {
	store, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// First pass — must succeed.
	if err := admin.SeedVocabularies(ctx, store); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	// Second pass — must also succeed (idempotent).
	if err := admin.SeedVocabularies(ctx, store); err != nil {
		t.Fatalf("second seed (idempotent): %v", err)
	}

	vocs, err := store.ListVocabularies(ctx)
	if err != nil {
		t.Fatalf("list vocabularies: %v", err)
	}
	if len(vocs) != 2 {
		t.Errorf("vocabulary count = %d, want 2", len(vocs))
	}

	sdlc, err := store.GetVocabularyByName(ctx, "sdlc")
	if err != nil {
		t.Fatalf("get sdlc: %v", err)
	}
	if sdlc.ReleaseEvent != "task_completed" {
		t.Errorf("sdlc release_event = %q, want task_completed", sdlc.ReleaseEvent)
	}

	bug, err := store.GetVocabularyByName(ctx, "bug-tracker")
	if err != nil {
		t.Fatalf("get bug-tracker: %v", err)
	}
	if bug.ReleaseEvent != "bug_closed" {
		t.Errorf("bug-tracker release_event = %q, want bug_closed", bug.ReleaseEvent)
	}
}
