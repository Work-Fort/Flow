// SPDX-License-Identifier: GPL-2.0-only
package e2e_test

import (
	"flag"
	"os"
	"testing"

	"github.com/Work-Fort/Flow/tests/e2e/harness"
)

var backendFlag = flag.String("backend", "sqlite", "storage backend: sqlite | postgres")

func TestMain(m *testing.M) {
	flag.Parse()
	harness.SetDefaultBackend(*backendFlag)
	os.Exit(m.Run())
}
