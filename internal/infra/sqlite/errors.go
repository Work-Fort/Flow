// SPDX-License-Identifier: GPL-2.0-only
package sqlite

import "strings"

// isUniqueViolation returns true if the error is a SQLite unique constraint
// violation. modernc.org/sqlite wraps errors as strings.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isFKViolation returns true if the error is a SQLite foreign key constraint
// violation.
func isFKViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
