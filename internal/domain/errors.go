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

	// ErrPoolExhausted is returned from Scheduler.AcquireAgent when Hive has
	// no free agent after all retries. The caller should surface this to
	// the workflow engine, which will retry later or mark the step blocked.
	ErrPoolExhausted = errors.New("agent pool exhausted")

	// ErrWorkflowMismatch is returned when Flow tries to release or renew
	// a lease with a workflow ID that does not match the one currently
	// held in Hive. This is almost always a bug in the caller.
	ErrWorkflowMismatch = errors.New("workflow id does not match current claim")

	// ErrRuntimeUnavailable is returned from RuntimeDriver operations when
	// the underlying runtime (Nexus, k8s, …) is unreachable or rejected
	// the call. Distinct from ErrNotFound so callers can retry transient
	// infrastructure outages without muddying not-found semantics.
	ErrRuntimeUnavailable = errors.New("runtime driver unavailable")

	// ErrValidation is returned when an infra adapter receives an
	// HTTP 400 (or equivalent) from a downstream service — the request
	// was syntactically/semantically rejected. Used by the Nexus
	// runtime driver and any future HTTP-backed adapter.
	ErrValidation = errors.New("validation failed")

	// ErrInvalidState is returned when an infra adapter receives an
	// HTTP 409 (or equivalent) from a downstream service — the
	// requested operation is incompatible with the resource's current
	// state (e.g., trying to clone a drive whose source is attached).
	ErrInvalidState = errors.New("invalid state")

	ErrEventNotInVocabulary = errors.New("event not in vocabulary")
	ErrProjectHasBot        = errors.New("project still has a bound bot")
	ErrBotKeyMissing        = errors.New("bot Passport key file missing")
)
