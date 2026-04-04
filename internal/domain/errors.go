// SPDX-License-Identifier: GPL-2.0-only
package domain

import "errors"

var (
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrHasDependencies      = errors.New("has dependencies")
	ErrInvalidGuard         = errors.New("invalid guard expression")
	ErrGuardDenied          = errors.New("transition guard denied")
	ErrInvalidTransition    = errors.New("invalid transition")
	ErrNotAtGateStep        = errors.New("work item is not at a gate step")
	ErrGateRequiresApproval = errors.New("gate step requires approval")
	ErrPermissionDenied     = errors.New("permission denied")
)
