// SPDX-License-Identifier: GPL-2.0-only

// Package nexus provides a domain.RuntimeDriver backed by a Nexus
// daemon. The driver speaks Nexus's REST API directly via net/http;
// no Nexus client library is imported, by design — Nexus does not
// ship one, and the e2e harness independence rule (see
// feedback_e2e_harness_independence.md) keeps even hypothetical
// future client libraries out of the verification path.
//
// The driver implements the seven-method RuntimeDriver port
// verbatim. Methods map 1:1 to Nexus REST operations; see the
// per-method comments in driver.go.
//
// VolumeRef.Kind for refs this driver emits is "nexus-drive";
// RuntimeHandle.Kind is "nexus-vm". Refs/handles with a different
// Kind passed back into driver methods return ErrUnsupportedKind.
//
// Project master refresh is currently a no-op after the first call
// (idempotent create). Warming-script execution is deferred to a
// follow-up plan; see TODO(plan: warming) in driver.go.
package nexus
