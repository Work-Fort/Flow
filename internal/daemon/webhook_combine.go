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
// events and dispatches to the project bot's vocabulary when a project
// matching the repo name exists.
//
// Combine's webhook discriminator is the X-SoftServe-Event header
// (combine/lead/internal/infra/webhook/webhook.go:117). Audit failures
// are logged but never block the response.
//
// audit may be nil (e.g. tests / early bring-up); the handler then
// drops the event and still 204s.
func HandleCombineWebhook(audit domain.AuditEventStore, projects domain.ProjectStore, dispatcher domain.BotDispatcher) http.Handler {
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
			if dispatcher != nil && projects != nil {
				branch := body.Ref
				if idx := len("refs/heads/"); len(branch) > idx {
					branch = branch[idx:]
				}
				dispatchCombineEvent(r.Context(), projects, dispatcher, body.Repository.Name, "commit_landed",
					domain.VocabularyContext{
						Payload: map[string]any{
							"branch":     branch,
							"commit_sha": body.After,
							"author":     "",
						},
					})
			}
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
			if dispatcher != nil && projects != nil {
				dispatchCombineEvent(r.Context(), projects, dispatcher, body.Repository.Name, "merged",
					domain.VocabularyContext{
						Payload: map[string]any{
							"pr_number":     body.PullRequest.Number,
							"merged_by":     body.Sender.Username,
							"target_branch": body.PullRequest.TargetBranch,
						},
					})
			}
		default:
			// Other Combine event types are accepted for forward
			// compatibility but not audited at this layer.
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// dispatchCombineEvent looks up a project by the Combine repo name and
// fires a vocabulary dispatch if the project exists. If no project matches
// or the vocabulary doesn't define the event, the call is a no-op (logged
// at debug). Audit recording is not affected — dispatch is additive.
func dispatchCombineEvent(ctx context.Context, projects domain.ProjectStore, dispatcher domain.BotDispatcher, repoName, eventType string, vocCtx domain.VocabularyContext) {
	p, err := projects.GetProjectByName(ctx, repoName)
	if err != nil {
		log.Debug("combine: no project for repo", "repo", repoName, "err", err)
		return
	}
	if err := dispatcher.Dispatch(ctx, p.ID, eventType, vocCtx); err != nil {
		log.Debug("combine: dispatch skipped", "repo", repoName, "event", eventType, "err", err)
	}
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
