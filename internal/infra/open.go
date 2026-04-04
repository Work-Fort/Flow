// SPDX-License-Identifier: GPL-2.0-only
package infra

import (
	"fmt"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/sqlite"
)

func Open(dsn string) (domain.Store, error) {
	s, err := sqlite.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}
	return s, nil
}
