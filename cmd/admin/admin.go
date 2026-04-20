// SPDX-License-Identifier: GPL-2.0-only
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra"
	"github.com/Work-Fort/Flow/internal/transfer"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Admin commands"}
	cmd.AddCommand(newSeedCmd())
	cmd.AddCommand(newSeedVocabulariesCmd())
	return cmd
}

// SeedVocabularies inserts the built-in SDLC and bug-tracker vocabularies.
// Idempotent: if a vocabulary with the same name already exists the insert is
// skipped silently (domain.ErrAlreadyExists).
func SeedVocabularies(ctx context.Context, store domain.VocabularyStore) error {
	seeds := builtinVocabularies()
	for _, v := range seeds {
		if err := store.CreateVocabulary(ctx, v); err != nil {
			if errors.Is(err, domain.ErrAlreadyExists) {
				log.Debug("vocabulary already loaded", "name", v.Name)
				continue
			}
			return fmt.Errorf("seed vocabulary %q: %w", v.Name, err)
		}
		log.Debug("vocabulary seeded", "name", v.Name)
	}
	return nil
}

func builtinVocabularies() []*domain.Vocabulary {
	return []*domain.Vocabulary{sdlcVocabulary(), bugTrackerVocabulary()}
}

func sdlcVocabulary() *domain.Vocabulary {
	return &domain.Vocabulary{
		ID:           "voc_builtin_sdlc",
		Name:         "sdlc",
		Description:  "Canonical software-delivery lifecycle vocabulary.",
		ReleaseEvent: "task_completed",
		Events: []domain.VocabularyEvent{
			{ID: "ve_builtin_sdlc_assigned", VocabularyID: "voc_builtin_sdlc", EventType: "task_assigned",
				MessageTemplate: `Task assigned: {{.WorkItem.Title}} → {{.AgentName}} ({{.Role}})`,
				MetadataKeys:    []string{"agent_id", "role", "workflow_id"}},
			{ID: "ve_builtin_sdlc_branch", VocabularyID: "voc_builtin_sdlc", EventType: "branch_created",
				MessageTemplate: `Branch created: {{index .Payload "branch"}}`,
				MetadataKeys:    []string{"branch", "commit_sha"}},
			{ID: "ve_builtin_sdlc_commit", VocabularyID: "voc_builtin_sdlc", EventType: "commit_landed",
				MessageTemplate: `Commit on {{index .Payload "branch"}}: {{index .Payload "commit_sha"}} by {{index .Payload "author"}}`,
				MetadataKeys:    []string{"branch", "commit_sha", "author"}},
			{ID: "ve_builtin_sdlc_pr", VocabularyID: "voc_builtin_sdlc", EventType: "pr_opened",
				MessageTemplate: `PR opened: #{{index .Payload "pr_number"}} on {{index .Payload "branch"}}`,
				MetadataKeys:    []string{"pr_number", "branch"}},
			{ID: "ve_builtin_sdlc_review", VocabularyID: "voc_builtin_sdlc", EventType: "review_requested",
				MessageTemplate: `Review requested: {{.WorkItem.Title}} (gate {{index .Payload "gate_step_id"}})`,
				MetadataKeys:    []string{"agent_id", "gate_step_id"}},
			{ID: "ve_builtin_sdlc_tests", VocabularyID: "voc_builtin_sdlc", EventType: "tests_passing",
				MessageTemplate: `Tests passing: {{index .Payload "repo"}} run {{index .Payload "run_id"}}`,
				MetadataKeys:    []string{"repo", "run_id"}},
			{ID: "ve_builtin_sdlc_merged", VocabularyID: "voc_builtin_sdlc", EventType: "merged",
				MessageTemplate: `Merged PR #{{index .Payload "pr_number"}} → {{index .Payload "target_branch"}} by {{index .Payload "merged_by"}}`,
				MetadataKeys:    []string{"pr_number", "merged_by", "target_branch"}},
			{ID: "ve_builtin_sdlc_deployed", VocabularyID: "voc_builtin_sdlc", EventType: "deployed",
				MessageTemplate: `Deployed {{index .Payload "commit_sha"}} to {{index .Payload "env"}}`,
				MetadataKeys:    []string{"commit_sha", "env"}},
			{ID: "ve_builtin_sdlc_completed", VocabularyID: "voc_builtin_sdlc", EventType: "task_completed",
				MessageTemplate: `Task completed: {{.WorkItem.Title}} → {{.AgentName}}`,
				MetadataKeys:    []string{"agent_id", "role", "workflow_id"}},
		},
	}
}

func bugTrackerVocabulary() *domain.Vocabulary {
	return &domain.Vocabulary{
		ID:           "voc_builtin_bugtracker",
		Name:         "bug-tracker",
		Description:  "Bug-tracker workflow vocabulary — covers the full lifecycle: file, triage, assign, fix-proposed, verify, close, reopen.",
		ReleaseEvent: "bug_closed",
		Events: []domain.VocabularyEvent{
			{ID: "ve_builtin_bug_filed", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_filed",
				MessageTemplate: `Bug filed: {{.WorkItem.Title}} (priority {{.WorkItem.Priority}})`,
				MetadataKeys:    []string{"reporter"}},
			{ID: "ve_builtin_bug_triaged", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_triaged",
				MessageTemplate: `Triaged: {{.WorkItem.Title}} severity {{index .Payload "severity"}}`,
				MetadataKeys:    []string{"severity"}},
			{ID: "ve_builtin_bug_assigned", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_assigned",
				MessageTemplate: `Assigned bug {{.WorkItem.Title}} → {{.AgentName}}`,
				MetadataKeys:    []string{"agent_id"}},
			{ID: "ve_builtin_bug_fix_proposed", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_fix_proposed",
				MessageTemplate: `Fix proposed for {{.WorkItem.Title}} by {{.AgentName}} (commit {{index .Payload "commit_sha"}})`,
				MetadataKeys:    []string{"agent_id", "commit_sha", "branch"}},
			{ID: "ve_builtin_bug_verified", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_verified",
				MessageTemplate: `Verified fix for {{.WorkItem.Title}} by {{.AgentName}}`,
				MetadataKeys:    []string{"agent_id", "verified_in"}},
			{ID: "ve_builtin_bug_closed", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_closed",
				MessageTemplate: `Closed: {{.WorkItem.Title}} (resolution {{index .Payload "resolution"}})`,
				MetadataKeys:    []string{"agent_id", "resolution"}},
			{ID: "ve_builtin_bug_reopened", VocabularyID: "voc_builtin_bugtracker", EventType: "bug_reopened",
				MessageTemplate: `Reopened: {{.WorkItem.Title}} (reason: {{index .Payload "reason"}})`,
				MetadataKeys:    []string{"reason"}},
		},
	}
}

func newSeedVocabulariesCmd() *cobra.Command {
	var db string
	cmd := &cobra.Command{
		Use:   "seed-vocabularies",
		Short: "Seed built-in SDLC and bug-tracker vocabularies",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := infra.Open(db)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer store.Close()
			return SeedVocabularies(context.Background(), store)
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Database path (required)")
	cmd.MarkFlagRequired("db")
	return cmd
}

func newSeedCmd() *cobra.Command {
	var db, file string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load a workflow template from a JSON file",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := infra.Open(db)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer store.Close()
			t, err := transfer.ImportTemplate(context.Background(), store, file, nil)
			if err != nil {
				return fmt.Errorf("import template: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(t)
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Database path (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to workflow JSON template (required)")
	cmd.MarkFlagRequired("db")
	cmd.MarkFlagRequired("file")
	return cmd
}
