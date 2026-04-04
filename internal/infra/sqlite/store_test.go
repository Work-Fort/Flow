// SPDX-License-Identifier: GPL-2.0-only
package sqlite_test

import (
	"context"
	"testing"

	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func TestStoreOpen(t *testing.T) {
	s, err := sqlite.Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
