// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/charmbracelet/log"

	"github.com/Work-Fort/Flow/internal/domain"
)

// combinePushPayload is the subset of Combine's PushEvent body we
// audit. Fields that aren't audited are left out — drift in those
// fields is irrelevant to Flow.
type combinePushPayload struct {
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// combineMergePayload is the subset of Combine's
// PullRequestMergedEvent body we audit.
type combineMergePayload struct {
	Repository struct {
		Name string `json:"name"`
	} `json:"repository"`
	PullRequest struct {
		Number       int64  `json:"number"`
		TargetBranch string `json:"target_branch"`
	} `json:"pull_request"`
	Sender struct {
		Username string `json:"username"`
	} `json:"sender"`
}

// HandleCombineWebhook returns the http.Handler mounted at
// POST /v1/webhooks/combine. It audits push and pull_request_merged
// events and 204s every other Combine event type.
//
// Combine's webhook discriminator is the X-SoftServe-Event header
// (combine/lead/internal/infra/webhook/webhook.go:117). Audit failures
// are logged but never block the response — the bot-vocabulary plan
// will layer real dispatch on top of this audit foundation.
//
// audit may be nil (e.g. tests / early bring-up); the handler then
// drops the event and still 204s.
//
// AuditEvent.Payload is a json.RawMessage field already declared in
// internal/domain/types.go:220 — no domain-type changes needed beyond
// the two new AuditEventType constants added alongside this handler.
func HandleCombineWebhook(audit domain.AuditEventStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := r.Header.Get("X-SoftServe-Event")
		switch event {
		case "push":
			var body combinePushPayload
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				log.Warn("combine webhook: bad push body", "err", err)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"repo":   body.Repository.Name,
				"ref":    body.Ref,
				"before": body.Before,
				"after":  body.After,
			})
			recordCombineEvent(r.Context(), audit, domain.AuditEventCombinePushReceived, payload)
		case "pull_request_merged":
			var body combineMergePayload
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				log.Warn("combine webhook: bad merge body", "err", err)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"repo":          body.Repository.Name,
				"pr_number":     body.PullRequest.Number,
				"target_branch": body.PullRequest.TargetBranch,
				"merged_by":     body.Sender.Username,
			})
			recordCombineEvent(r.Context(), audit, domain.AuditEventCombineMergeReceived, payload)
		default:
			// Other Combine event types are accepted for forward
			// compatibility but not audited at this layer.
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// recordCombineEvent persists the audit row, swallowing failures with
// a warning so a bad audit store never blocks the webhook ack. Audit
// is the secondary concern of this handler; returning the 204 is the
// primary one.
func recordCombineEvent(ctx context.Context, audit domain.AuditEventStore, ty domain.AuditEventType, payload json.RawMessage) {
	if audit == nil {
		return
	}
	if err := audit.RecordAuditEvent(ctx, &domain.AuditEvent{Type: ty, Payload: payload}); err != nil {
		log.Warn("combine webhook: audit failed", "type", ty, "err", err)
	}
}
