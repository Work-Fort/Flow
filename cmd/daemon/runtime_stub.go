// SPDX-License-Identifier: GPL-2.0-only

//go:build !e2e

package daemon

import flowDaemon "github.com/Work-Fort/Flow/internal/daemon"

func injectStubRuntime(_ *flowDaemon.ServerConfig) {}
