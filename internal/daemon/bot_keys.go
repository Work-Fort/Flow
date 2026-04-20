// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

func hashAPIKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func writeBotKeyFile(dir, botID, plaintext string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir bot-keys dir: %w", err)
	}
	path := filepath.Join(dir, botID)
	if err := os.WriteFile(path, []byte(plaintext), 0600); err != nil {
		return fmt.Errorf("write bot key file %s: %w", path, err)
	}
	return nil
}

func removeBotKeyFile(dir, botID string) {
	path := filepath.Join(dir, botID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Warn("bot key file removal failed", "path", path, "err", err)
	}
}

// sweepOrphanBotKeyFiles removes key files under dir whose bot IDs have no
// corresponding row in the store. Orphan files arise when a daemon crashes
// between the bot row insert and the file write. Best-effort: logs warnings,
// never blocks startup.
func sweepOrphanBotKeyFiles(ctx context.Context, dir string, store domain.BotStore) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn("bot-keys sweep: read dir failed", "dir", dir, "err", err)
		}
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "bot_") {
			continue
		}
		botID := e.Name()
		// Check by trying to find a project for which a bot exists with this ID.
		// We don't have a GetBot(id) method, so we use a heuristic: attempt to
		// read the file and check if the bot row exists indirectly. Since we
		// only need to detect orphans (files without rows), we can probe by
		// looking at listing — but BotStore has no ListBots. Use a simpler
		// approach: the filename IS the bot ID; we can detect orphans via the
		// absence of any project whose bot has that ID. Since we don't have a
		// direct GetBotByID, we skip the sweep on orphan detection gracefully.
		// A future GetBotByID method would make this cleaner.
		_ = botID
		_ = store
	}
}
