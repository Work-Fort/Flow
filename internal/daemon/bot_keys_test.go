// SPDX-License-Identifier: GPL-2.0-only
package daemon_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	daemon "github.com/Work-Fort/Flow/internal/daemon"
	"github.com/Work-Fort/Flow/internal/domain"
)

type stubBotStore struct {
	bots map[string]*domain.Bot
}

func (s *stubBotStore) CreateBot(_ context.Context, b *domain.Bot) error {
	s.bots[b.ID] = b
	return nil
}

func (s *stubBotStore) GetBotByID(_ context.Context, id string) (*domain.Bot, error) {
	if b, ok := s.bots[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("%w: bot %s", domain.ErrNotFound, id)
}

func (s *stubBotStore) GetBotByProject(_ context.Context, projectID string) (*domain.Bot, error) {
	for _, b := range s.bots {
		if b.ProjectID == projectID {
			return b, nil
		}
	}
	return nil, fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID)
}

func (s *stubBotStore) DeleteBotByProject(_ context.Context, projectID string) error {
	for id, b := range s.bots {
		if b.ProjectID == projectID {
			delete(s.bots, id)
			return nil
		}
	}
	return fmt.Errorf("%w: bot for project %s", domain.ErrNotFound, projectID)
}

func TestBotKeys_StartupSweep_RemovesOrphans(t *testing.T) {
	dir := t.TempDir()

	// Write two key files: one with a matching row, one orphan.
	knownID := "bot_known_001"
	orphanID := "bot_orphan_999"
	os.WriteFile(filepath.Join(dir, knownID), []byte("key1"), 0600)
	os.WriteFile(filepath.Join(dir, orphanID), []byte("key2"), 0600)

	store := &stubBotStore{bots: map[string]*domain.Bot{
		knownID: {ID: knownID, ProjectID: "prj_1"},
	}}

	daemon.SweepOrphanBotKeyFiles(context.Background(), dir, store)

	if _, err := os.Stat(filepath.Join(dir, knownID)); errors.Is(err, os.ErrNotExist) {
		t.Error("known bot key file was incorrectly removed")
	}
	if _, err := os.Stat(filepath.Join(dir, orphanID)); !errors.Is(err, os.ErrNotExist) {
		t.Error("orphan bot key file was not removed")
	}
}
