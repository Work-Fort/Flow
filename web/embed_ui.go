//go:build ui

// SPDX-License-Identifier: GPL-2.0-only
package web

import "embed"

// Dist holds the Vite build output. Built via:
//
//	mise run build:web
//	go build -tags ui
//
//go:embed all:dist
var Dist embed.FS
