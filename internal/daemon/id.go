// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// NewID returns a prefixed short ID: e.g. "tpl_a1b2c3d4".
func NewID(prefix string) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	return fmt.Sprintf("%s_%s", prefix, id[:8])
}
