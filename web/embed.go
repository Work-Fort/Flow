//go:build !ui

// SPDX-License-Identifier: GPL-2.0-only
package web

import "embed"

// Dist is empty when built without the "ui" tag. Use the dev proxy
// (mise run dev:web) during frontend development; serve the
// embedded build by compiling with -tags ui.
var Dist embed.FS
