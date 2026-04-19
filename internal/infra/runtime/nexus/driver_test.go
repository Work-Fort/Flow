// SPDX-License-Identifier: GPL-2.0-only
package nexus_test

import (
	"testing"

	"github.com/Work-Fort/Flow/internal/domain"
	"github.com/Work-Fort/Flow/internal/infra/runtime/nexus"
)

func TestDriver_SatisfiesRuntimeDriverInterface(t *testing.T) {
	var _ domain.RuntimeDriver = nexus.New(nexus.Config{
		BaseURL: "http://example.invalid",
	})
}
